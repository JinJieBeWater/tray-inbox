package t3

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
)

type Config struct {
	UserData string // T3's state directory, ~/.t3/userdata when empty; only read
	T3       string // path to t3; empty when it was not found
	Tokens   TokenStore
	OpenURL  func(string) error
	Now      func() time.Time
	After    func(time.Duration) <-chan time.Time
}

type Adapter struct{ cfg Config }

var _ agent.Adapter = (*Adapter)(nil)

func New(cfg Config) *Adapter {
	if cfg.UserData == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cfg.UserData = filepath.Join(home, ".t3", "userdata")
		}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.After == nil {
		cfg.After = time.After
	}
	return &Adapter{cfg: cfg}
}

const (
	minBackoff   = time.Second
	maxBackoff   = 30 * time.Second
	ioTimeout    = 10 * time.Second
	issueTimeout = 30 * time.Second
	runtimeFile  = "server-runtime.json"

	shellRequest  = "1"
	configRequest = "2"
	subscribeMsg  = `{"_tag":"Request","id":"1","tag":"orchestration.subscribeShell","payload":{"requestCompletionMarker":true},"headers":[]}`
	configMsg     = `{"_tag":"Request","id":"2","tag":"server.getConfig","payload":{},"headers":[]}`
	ackMsg        = `{"_tag":"Ack","requestId":"1"}`
)

type retry int

const (
	retryOnChange retry = iota // when server-runtime.json changes
	retryLater                 // after the backoff, or sooner when server-runtime.json changes
)

// runner is one Run's state; only Run's goroutine touches it.
type runner struct {
	*Adapter
	emit     func(agent.Event)
	isDown   bool
	reason   agent.Reason
	unproven bool   // the token was issued by this run and the server has not accepted it yet
	token    string // the last token read or issued, so a keychain that fails cannot make every session issue
}

func (r *runner) send(ev agent.Event) {
	if _, ok := ev.(agent.Snapshot); ok {
		r.isDown = false
	}
	r.emit(ev)
}

func (r *runner) down(reason agent.Reason) {
	if r.isDown && r.reason == reason {
		return
	}
	r.isDown, r.reason = true, reason
	r.emit(agent.SourceStatus{Orchestrator: orchestrator, Source: localSource, Reason: reason})
}

func (a *Adapter) Run(ctx context.Context, emit func(agent.Event)) error {
	// The watcher exists before the first read, so a server that starts in between is not missed.
	var dir *fsnotify.Watcher
	if w, err := fsnotify.NewWatcher(); err == nil {
		if w.Add(a.cfg.UserData) == nil {
			dir = w
			defer dir.Close()
		} else {
			w.Close()
		}
	}
	r := &runner{Adapter: a, emit: emit}
	backoff := minBackoff
	for {
		up, reason, next := r.session(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if up {
			backoff = minBackoff
		}
		r.down(reason)
		var later <-chan time.Time
		if dir == nil || next == retryLater {
			later = a.cfg.After(backoff)
			backoff = min(2*backoff, maxBackoff)
		}
		if !waitRuntime(ctx, dir, later) {
			return nil
		}
	}
}

// session connects once and streams until the connection ends. up reports whether it emitted a snapshot.
func (r *runner) session(ctx context.Context) (up bool, reason agent.Reason, next retry) {
	origin, err := r.origin()
	if errors.Is(err, fs.ErrNotExist) {
		return false, agent.ReasonAppNotRunning, retryOnChange
	}
	if err != nil {
		return false, agent.ReasonAppNotRunning, retryLater
	}
	if r.token == "" {
		r.token, _ = r.cfg.Tokens.Get()
	}
	token := r.token
	if token == "" {
		if token, reason = r.issue(ctx); reason != "" {
			return false, reason, retryLater
		}
	}
	conn, err := r.dial(ctx, origin, token)
	if errors.Is(err, errRejected) && !r.unproven {
		if token, reason = r.issue(ctx); reason != "" {
			return false, reason, retryLater
		}
		conn, err = r.dial(ctx, origin, token)
	}
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return false, agent.ReasonAppNotRunning, retryLater
	case err != nil:
		return false, agent.ReasonReconnecting, retryLater
	}
	r.unproven = false
	return r.stream(ctx, conn)
}

func (a *Adapter) Orchestrator() string { return orchestrator }

func (a *Adapter) Jump(ctx context.Context, id agent.ID) error {
	if id.Orchestrator != orchestrator || id.Source != localSource {
		return fmt.Errorf("t3: not a local T3 agent: %v", id)
	}
	env, thread, ok := strings.Cut(id.Native, "/")
	if !ok || env == "" || thread == "" {
		return fmt.Errorf("t3: malformed id %q", id.Native)
	}
	if a.cfg.OpenURL == nil {
		return errors.New("t3: cannot open links")
	}
	link := "t3code://threads/" + url.PathEscape(env) + "/" + url.PathEscape(thread)
	if err := a.cfg.OpenURL(link); err != nil {
		return fmt.Errorf("t3: open %s: %w", link, err)
	}
	return nil
}
