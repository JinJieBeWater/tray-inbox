package t3

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
)

const fakeT3Env = "TRAYINBOX_FAKE_T3"

func TestMain(m *testing.M) {
	if dir := os.Getenv(fakeT3Env); dir != "" {
		os.Exit(fakeT3(dir, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// fakeT3 acts as `t3`: it logs its arguments and prints dir/issue.json.
func fakeT3(dir string, args []string) int {
	if f, err := os.OpenFile(filepath.Join(dir, "calls.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		f.WriteString(strings.Join(args, " ") + "\n")
		f.Close()
	}
	b, err := os.ReadFile(filepath.Join(dir, "issue.json"))
	if err != nil {
		return 1
	}
	os.Stdout.Write(b)
	return 0
}

type fakeStore struct {
	mu     sync.Mutex
	token  string
	sets   []string
	broken bool // Set fails and keeps nothing
}

func (s *fakeStore) Get() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token == "" {
		return "", ErrNoToken
	}
	return s.token, nil
}

func (s *fakeStore) Set(v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken {
		return errors.New("keychain refused")
	}
	s.token = v
	s.sets = append(s.sets, v)
	return nil
}

func (s *fakeStore) setCalls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sets...)
}

type fakeServer struct {
	*httptest.Server
	mu       sync.Mutex
	accept   map[string]bool
	conns    chan *serverConn
	rejected chan string
}

func startServer(t *testing.T, accept ...string) *fakeServer {
	t.Helper()
	s := &fakeServer{accept: map[string]bool{}, conns: make(chan *serverConn, 10), rejected: make(chan string, 10)}
	for _, tok := range accept {
		s.accept[tok] = true
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" || r.URL.Query().Get("orchestrationProtocol") != "2" {
			w.WriteHeader(http.StatusUpgradeRequired)
			return
		}
		tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		s.mu.Lock()
		ok := s.accept[tok]
		s.mu.Unlock()
		if !ok {
			s.rejected <- tok
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"_tag":"EnvironmentAuthInvalidError","code":"auth_invalid","reason":"invalid_credential","traceId":"t1"}`))
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		c.SetReadLimit(-1)
		s.conns <- &serverConn{c: c}
	}))
	t.Cleanup(s.Close)
	return s
}

type serverConn struct{ c *websocket.Conn }

func (c *serverConn) read(t *testing.T) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, b, err := c.c.Read(ctx)
	if err != nil {
		t.Fatalf("server read: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("server read %s: %v", b, err)
	}
	return m
}

func (c *serverConn) send(t *testing.T, msg string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.c.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
		t.Fatalf("server write: %v", err)
	}
}

func chunk(values ...[]byte) string {
	return `{"_tag":"Chunk","requestId":"1","values":[` + string(joinJSON(values)) + `]}`
}

func joinJSON(values [][]byte) []byte {
	var out []byte
	for i, v := range values {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, compact(v)...)
	}
	return out
}

func compact(b []byte) []byte {
	var m json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		panic(err)
	}
	out, _ := json.Marshal(m)
	return out
}

func (c *serverConn) expectAck(t *testing.T) {
	t.Helper()
	if got, want := c.read(t), map[string]any{"_tag": "Ack", "requestId": "1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("client sent %v, want %v", got, want)
	}
}

func (c *serverConn) sendConfig(t *testing.T) {
	t.Helper()
	c.send(t, `{"_tag":"Exit","requestId":"2","exit":{"_tag":"Success","value":`+string(compact(readTestdata(t, "server-config.json")))+`}}`)
}

// handshake checks the two requests the adapter opens with and answers server.getConfig.
func (c *serverConn) handshake(t *testing.T) {
	t.Helper()
	want := map[string]map[string]any{
		"orchestration.subscribeShell": {"_tag": "Request", "id": "1", "tag": "orchestration.subscribeShell", "payload": map[string]any{"requestCompletionMarker": true}, "headers": []any{}},
		"server.getConfig":             {"_tag": "Request", "id": "2", "tag": "server.getConfig", "payload": map[string]any{}, "headers": []any{}},
	}
	for range 2 {
		got := c.read(t)
		tag, _ := got["tag"].(string)
		if w, ok := want[tag]; !ok || !reflect.DeepEqual(got, w) {
			t.Fatalf("client sent %v", got)
		}
		delete(want, tag)
	}
	c.sendConfig(t)
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) set(t time.Time) {
	c.mu.Lock()
	c.now = t
	c.mu.Unlock()
}

type afterCall struct {
	d  time.Duration
	ch chan time.Time
}

type harness struct {
	userData, t3Dir string
	store           *fakeStore
	server          *fakeServer
	clock           *fakeClock
	afters          chan afterCall
	emitted         chan agent.Event
	adapter         *Adapter
	cancel          context.CancelFunc
	done            chan error
}

type setup struct {
	token   string   // in the store at start
	accept  []string // tokens the server takes
	noT3    bool
	noFile  bool
	refused bool // server-runtime.json names a port nobody listens on
}

func newHarness(t *testing.T, s setup) *harness {
	t.Helper()
	h := &harness{
		userData: t.TempDir(), t3Dir: t.TempDir(),
		store:   &fakeStore{token: s.token},
		server:  startServer(t, s.accept...),
		clock:   &fakeClock{now: testNow},
		afters:  make(chan afterCall, 100),
		emitted: make(chan agent.Event, 100),
	}
	t.Setenv(fakeT3Env, h.t3Dir)
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	if err := os.WriteFile(filepath.Join(h.t3Dir, "issue.json"), readTestdata(t, "session-issue.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	switch {
	case s.refused:
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := ln.Addr().String()
		ln.Close()
		h.writeRuntime(t, "http://"+addr)
	case !s.noFile:
		h.writeRuntime(t, h.server.URL)
	}
	cfg := Config{
		UserData: h.userData,
		Tokens:   h.store,
		OpenURL:  func(string) error { return errors.New("not in this test") },
		Now:      h.clock.Now,
		After: func(d time.Duration) <-chan time.Time {
			c := afterCall{d, make(chan time.Time, 1)}
			h.afters <- c
			return c.ch
		},
	}
	if !s.noT3 {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cfg.T3 = exe
	}
	h.adapter = New(cfg)
	return h
}

// writeRuntime replaces server-runtime.json in one rename, as a reader never sees it half written.
func (h *harness) writeRuntime(t *testing.T, origin string) {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"version": 1, "pid": 4242, "host": "127.0.0.1", "port": 0, "origin": origin, "startedAt": iso(testNow), "serverVersion": "0.0.0-nightly", "managedPolicy": nil, "ownerId": "o1"})
	tmp := filepath.Join(h.userData, ".server-runtime.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(h.userData, "server-runtime.json")); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) run(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.done = make(chan error, 1)
	go func() { h.done <- h.adapter.Run(ctx, func(ev agent.Event) { h.emitted <- ev }) }()
	t.Cleanup(func() { h.stop(t) })
}

func (h *harness) stop(t *testing.T) {
	t.Helper()
	if h.cancel == nil {
		return
	}
	h.cancel()
	h.cancel = nil
	select {
	case err := <-h.done:
		if err != nil {
			t.Errorf("Run returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func (h *harness) expect(t *testing.T, want ...agent.Event) {
	t.Helper()
	for _, w := range want {
		select {
		case got := <-h.emitted:
			if !reflect.DeepEqual(got, w) {
				t.Fatalf("emitted\n %+v\nwant\n %+v", got, w)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("nothing emitted, want %+v", w)
		}
	}
}

// afterFor waits for an After of d, passing over the snooze deadline the fixture arms.
func (h *harness) afterFor(t *testing.T, d time.Duration) afterCall {
	t.Helper()
	for {
		select {
		case c := <-h.afters:
			if c.d == d {
				return c
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("no After(%v)", d)
		}
	}
}

func (h *harness) accept(t *testing.T) *serverConn {
	t.Helper()
	select {
	case c := <-h.server.conns:
		t.Cleanup(func() { c.c.CloseNow() })
		return c
	case <-time.After(10 * time.Second):
		t.Fatal("adapter did not connect")
		return nil
	}
}

func (h *harness) rejected(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-h.server.rejected:
		if got != want {
			t.Fatalf("server rejected %q, want %q", got, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("server did not reject %q", want)
	}
}

func (h *harness) calls(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(h.t3Dir, "calls.log"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func fixtureSnapshot() agent.Snapshot {
	return agent.Snapshot{Orchestrator: "t3", Source: "local", Agents: fixtureAgents()}
}

func down(r agent.Reason) agent.SourceStatus {
	return agent.SourceStatus{Orchestrator: "t3", Source: "local", Reason: r}
}

const issueCall = "auth session issue --scope orchestration:read --ttl 30d --label trayinbox --json"

func TestSubscribeEmitsTheSnapshot(t *testing.T) {
	h := newHarness(t, setup{token: "stored-token", accept: []string{"stored-token"}})
	h.run(t)
	c := h.accept(t)
	c.handshake(t)
	c.send(t, chunk(readTestdata(t, "shell-snapshot.json")))
	c.expectAck(t)
	h.expect(t, fixtureSnapshot())
	if got := h.calls(t); got != nil {
		t.Fatalf("t3 ran %v with a stored token", got)
	}
}

func TestEveryChunkIsAcked(t *testing.T) {
	h := newHarness(t, setup{token: "stored-token", accept: []string{"stored-token"}})
	h.run(t)
	c := h.accept(t)
	c.handshake(t)
	c.send(t, `[`+chunk(readTestdata(t, "shell-snapshot.json"))+`,`+chunk([]byte(`{"kind":"synchronized"}`))+`]`)
	c.expectAck(t)
	c.expectAck(t)
	h.expect(t, fixtureSnapshot())
	for i := range 10 {
		th := fixtureThread(t, 3, "title", "Fix the login bug "+string(rune('a'+i)))
		c.send(t, chunk(encode(t, map[string]any{"kind": "thread.updated", "sequence": 121 + i, "location": "active", "thread": th})))
		c.expectAck(t)
		want := fixtureAgent(3)
		want.Title = th["title"].(string)
		h.expect(t, agent.Upsert{Agent: want})
	}
}

func TestShellBeforeConfigWaitsForIt(t *testing.T) {
	h := newHarness(t, setup{token: "stored-token", accept: []string{"stored-token"}})
	h.run(t)
	c := h.accept(t)
	c.read(t)
	c.read(t)
	c.send(t, chunk(readTestdata(t, "shell-snapshot.json")))
	c.expectAck(t)
	c.sendConfig(t)
	h.expect(t, fixtureSnapshot())
}

func TestGraceEndsOnTheInjectedTimer(t *testing.T) {
	h := newHarness(t, setup{token: "stored-token", accept: []string{"stored-token"}})
	h.run(t)
	c := h.accept(t)
	c.handshake(t)
	c.send(t, chunk(readTestdata(t, "shell-snapshot.json")))
	c.expectAck(t)
	h.expect(t, fixtureSnapshot())

	c.send(t, chunk(encode(t, map[string]any{"kind": "thread.updated", "sequence": 121, "location": "active", "thread": stopped(t, 1, testNow)})))
	c.expectAck(t)
	held := fixtureAgent(1)
	held.Since = testNow
	h.expect(t, agent.Upsert{Agent: held})
	timer := h.afterFor(t, workingExitGrace)
	h.clock.set(testNow.Add(workingExitGrace))
	timer.ch <- testNow.Add(workingExitGrace)
	done := held
	done.State = agent.Done
	h.expect(t, agent.Upsert{Agent: done})
}

func TestMissingRuntimeFileWaitsForIt(t *testing.T) {
	h := newHarness(t, setup{token: "stored-token", accept: []string{"stored-token"}, noFile: true})
	h.run(t)
	h.expect(t, down(agent.ReasonAppNotRunning))
	h.writeRuntime(t, h.server.URL)
	c := h.accept(t)
	c.handshake(t)
	c.send(t, chunk(readTestdata(t, "shell-snapshot.json")))
	c.expectAck(t)
	h.expect(t, fixtureSnapshot())
}

func TestRefusedConnectionBacksOff(t *testing.T) {
	h := newHarness(t, setup{token: "stored-token", accept: []string{"stored-token"}, refused: true})
	h.run(t)
	h.expect(t, down(agent.ReasonAppNotRunning))
	h.afterFor(t, time.Second).ch <- testNow
	h.afterFor(t, 2*time.Second)
	h.writeRuntime(t, h.server.URL)
	c := h.accept(t)
	c.handshake(t)
	c.send(t, chunk(readTestdata(t, "shell-snapshot.json")))
	c.expectAck(t)
	h.expect(t, fixtureSnapshot())
}

func TestDropReconnects(t *testing.T) {
	h := newHarness(t, setup{token: "stored-token", accept: []string{"stored-token"}})
	h.run(t)
	for range 2 {
		c := h.accept(t)
		c.handshake(t)
		c.send(t, chunk(readTestdata(t, "shell-snapshot.json")))
		c.expectAck(t)
		h.expect(t, fixtureSnapshot())
		c.c.CloseNow()
		h.expect(t, down(agent.ReasonReconnecting))
		h.afterFor(t, time.Second).ch <- testNow
	}
}

func TestRejectedTokenIsReissuedOnce(t *testing.T) {
	h := newHarness(t, setup{token: "stale-token", accept: []string{"fresh-token"}})
	h.run(t)
	h.rejected(t, "stale-token")
	c := h.accept(t)
	c.handshake(t)
	c.send(t, chunk(readTestdata(t, "shell-snapshot.json")))
	c.expectAck(t)
	h.expect(t, fixtureSnapshot())
	if got := h.calls(t); !reflect.DeepEqual(got, []string{issueCall}) {
		t.Fatalf("t3 calls %q", got)
	}
	if got := h.store.setCalls(); !reflect.DeepEqual(got, []string{"fresh-token"}) {
		t.Fatalf("stored %q", got)
	}
}

func TestRejectedFreshTokenIsNotReissued(t *testing.T) {
	h := newHarness(t, setup{token: "stale-token"})
	h.run(t)
	h.rejected(t, "stale-token")
	h.rejected(t, "fresh-token")
	h.expect(t, down(agent.ReasonReconnecting))
	h.afterFor(t, time.Second).ch <- testNow
	h.rejected(t, "fresh-token")
	h.afterFor(t, 2*time.Second)
	if got := h.calls(t); !reflect.DeepEqual(got, []string{issueCall}) {
		t.Fatalf("t3 calls %q", got)
	}
}

func TestMissingTokenIsIssued(t *testing.T) {
	h := newHarness(t, setup{accept: []string{"fresh-token"}})
	h.run(t)
	c := h.accept(t)
	c.handshake(t)
	c.send(t, chunk(readTestdata(t, "shell-snapshot.json")))
	c.expectAck(t)
	h.expect(t, fixtureSnapshot())
	if got := h.calls(t); !reflect.DeepEqual(got, []string{issueCall}) {
		t.Fatalf("t3 calls %q", got)
	}
}

// Issuing writes T3's database: a keychain that keeps nothing must not make every reconnect issue again.
func TestIssuedTokenOutlivesABrokenKeychain(t *testing.T) {
	h := newHarness(t, setup{accept: []string{"fresh-token"}})
	h.store.broken = true
	h.run(t)
	for range 2 {
		c := h.accept(t)
		c.handshake(t)
		c.send(t, chunk(readTestdata(t, "shell-snapshot.json")))
		c.expectAck(t)
		h.expect(t, fixtureSnapshot())
		c.c.CloseNow()
		h.expect(t, down(agent.ReasonReconnecting))
		h.afterFor(t, time.Second).ch <- testNow
	}
	if got := h.calls(t); !reflect.DeepEqual(got, []string{issueCall}) {
		t.Fatalf("t3 calls %q, want one issue", got)
	}
}

func TestNoTokenAndNoCommand(t *testing.T) {
	h := newHarness(t, setup{noT3: true})
	h.run(t)
	h.expect(t, down(agent.ReasonNoCommand))
	select {
	case <-h.server.conns:
		t.Fatal("connected without a token")
	case tok := <-h.server.rejected:
		t.Fatalf("dialed with %q", tok)
	default:
	}
}

func TestJumpOpensTheDeepLink(t *testing.T) {
	var opened []string
	openErr := errors.New("no handler for t3code:")
	a := New(Config{OpenURL: func(u string) error { opened = append(opened, u); return openErr }})
	if err := a.Jump(context.Background(), id(2)); !errors.Is(err, openErr) {
		t.Fatalf("Jump = %v, want the OpenURL error", err)
	}
	if want := []string{"t3code://threads/" + testEnv + "/" + tid(2)}; !reflect.DeepEqual(opened, want) {
		t.Fatalf("opened %q, want %q", opened, want)
	}
	if err := a.Jump(context.Background(), agent.ID{Orchestrator: "hc", Source: "local", Native: "x/y"}); err == nil {
		t.Fatal("Jump took an hc agent")
	}
	if len(opened) != 1 {
		t.Fatalf("opened %q", opened)
	}
}
