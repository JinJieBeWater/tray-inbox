package t3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
)

func (a *Adapter) dial(ctx context.Context, origin, token string) (*websocket.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, ioTimeout)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, strings.TrimSuffix(origin, "/")+"/ws?orchestrationProtocol=2", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + token}},
	})
	if err != nil {
		if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			return nil, errRejected
		}
		return nil, err
	}
	conn.SetReadLimit(-1)
	return conn, nil
}

// rpcMessage is an Effect RPC message from the server (effect/src/rpc/RpcMessage.ts) in its JSON serialization.
type rpcMessage struct {
	Tag       string            `json:"_tag"`
	RequestID requestID         `json:"requestId"`
	Values    []json.RawMessage `json:"values"`
	Exit      json.RawMessage   `json:"exit"`
}

// requestID echoes the id the request was sent with; Effect allows a string or a number.
type requestID string

func (id *requestID) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*id = requestID(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*id = requestID(n.String())
	return nil
}

// decodeFrame reads one WebSocket frame, which holds one message or an array of them.
func decodeFrame(b []byte) ([]rpcMessage, error) {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '[' {
		var ms []rpcMessage
		return ms, json.Unmarshal(b, &ms)
	}
	var m rpcMessage
	return []rpcMessage{m}, json.Unmarshal(b, &m)
}

func decodeConfig(exit json.RawMessage) (env string, kinds map[string]string, err error) {
	var e struct {
		Tag   string `json:"_tag"`
		Value struct {
			Environment struct {
				EnvironmentID string `json:"environmentId"`
			} `json:"environment"`
			Providers []struct {
				InstanceID string `json:"instanceId"`
				Driver     string `json:"driver"`
			} `json:"providers"`
		} `json:"value"`
	}
	if err := json.Unmarshal(exit, &e); err != nil {
		return "", nil, err
	}
	if e.Tag != "Success" || e.Value.Environment.EnvironmentID == "" {
		return "", nil, errors.New("t3: server.getConfig failed")
	}
	kinds = map[string]string{}
	for _, p := range e.Value.Providers {
		kinds[p.InstanceID] = p.Driver
	}
	return e.Value.Environment.EnvironmentID, kinds, nil
}

// stream reads the shell subscription until the connection ends or ctx ends.
func (r *runner) stream(ctx context.Context, conn *websocket.Conn) (up bool, reason agent.Reason, next retry) {
	sctx, cancel := context.WithCancel(ctx)
	frames := make(chan []byte)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		defer close(frames)
		for {
			_, b, err := conn.Read(sctx)
			if err != nil {
				return
			}
			select {
			case frames <- b:
			case <-sctx.Done():
				return
			}
		}
	}()
	defer func() {
		cancel()
		conn.CloseNow()
		<-readDone
	}()

	write := func(msg string) error {
		wctx, cancel := context.WithTimeout(sctx, ioTimeout)
		defer cancel()
		return conn.Write(wctx, websocket.MessageText, []byte(msg))
	}
	if write(subscribeMsg) != nil || write(configMsg) != nil {
		return false, agent.ReasonReconnecting, retryLater
	}

	var (
		tr    *tracker
		held  []item // shell items that arrived before the environment id
		armed time.Time
		timer <-chan time.Time
	)
	out := func(evs []agent.Event) {
		for _, ev := range evs {
			if _, ok := ev.(agent.Snapshot); ok {
				up = true
			}
			r.send(ev)
		}
	}
	for {
		if tr != nil {
			if d, ok := tr.deadline(); !ok {
				armed, timer = time.Time{}, nil
			} else if !d.Equal(armed) {
				armed, timer = d, r.cfg.After(max(d.Sub(r.cfg.Now()), 0))
			}
		}
		select {
		case <-ctx.Done():
			return up, agent.ReasonReconnecting, retryLater
		case <-timer:
			armed, timer = time.Time{}, nil
			out(tr.expire(r.cfg.Now()))
		case b, ok := <-frames:
			if !ok {
				return up, agent.ReasonReconnecting, retryLater
			}
			msgs, err := decodeFrame(b)
			if err != nil {
				return up, agent.ReasonReconnecting, retryLater
			}
			for _, m := range msgs {
				switch m.Tag {
				case "Chunk":
					if m.RequestID != shellRequest {
						continue
					}
					for _, v := range m.Values {
						it, err := decodeItem(v)
						if err != nil {
							return up, agent.ReasonReconnecting, retryLater
						}
						if tr == nil {
							held = append(held, it)
						} else {
							out(tr.apply(it, r.cfg.Now()))
						}
					}
					// The server stops the stream after 8 chunks without an Ack (apps/server/src/terminal/OutputProtocol.ts).
					if write(ackMsg) != nil {
						return up, agent.ReasonReconnecting, retryLater
					}
				case "Exit":
					if m.RequestID != configRequest {
						return up, agent.ReasonReconnecting, retryLater
					}
					env, kinds, err := decodeConfig(m.Exit)
					if err != nil {
						return up, agent.ReasonReconnecting, retryLater
					}
					tr = newTracker(env, kinds)
					for _, it := range held {
						out(tr.apply(it, r.cfg.Now()))
					}
					held = nil
				case "Defect":
					return up, agent.ReasonReconnecting, retryLater
				}
			}
		}
	}
}
