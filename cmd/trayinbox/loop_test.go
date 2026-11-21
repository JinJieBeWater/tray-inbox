package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
	"github.com/jinjiebewater/tray-inbox/internal/inbox"
)

type fakeAdapter struct {
	name    string
	events  []agent.Event
	emitted chan struct{}
	jumps   []agent.ID
	opens   int
}

func (f *fakeAdapter) Orchestrator() string { return f.name }

func (f *fakeAdapter) Run(ctx context.Context, emit func(agent.Event)) error {
	for _, ev := range f.events {
		emit(ev)
	}
	close(f.emitted)
	<-ctx.Done()
	return ctx.Err()
}

func (f *fakeAdapter) Jump(_ context.Context, id agent.ID) error {
	f.jumps = append(f.jumps, id)
	return nil
}

func (f *fakeAdapter) Open(_ context.Context) error {
	f.opens++
	return nil
}

func snapshot(orch, native string) agent.Snapshot {
	id := agent.ID{Orchestrator: orch, Source: "local", Native: native}
	return agent.Snapshot{Orchestrator: orch, Source: "local", Agents: []agent.Agent{{ID: id, Info: agent.Info{Title: native}, State: agent.Blocked}}}
}

func TestRunPublishesOnlyChangedViews(t *testing.T) {
	hc := &fakeAdapter{name: "hc", emitted: make(chan struct{}), events: []agent.Event{snapshot("hc", "t1"), snapshot("hc", "t1")}}
	t3 := &fakeAdapter{name: "t3", emitted: make(chan struct{}), events: []agent.Event{snapshot("t3", "x"), snapshot("t3", "x")}}
	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

	var mu sync.Mutex
	var views []inbox.View
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		run(ctx, []agent.Adapter{hc, t3}, func() time.Time { return at }, func(v inbox.View) {
			mu.Lock()
			views = append(views, v)
			mu.Unlock()
		})
		close(done)
	}()
	for _, f := range []*fakeAdapter{hc, t3} {
		select {
		case <-f.emitted:
		case <-done:
			t.Fatal("run returned before the adapters emitted")
		}
	}
	cancel()
	<-done

	if len(views) != 3 {
		t.Fatalf("published %d views, want 3 (initial, hc, t3): %+v", len(views), views)
	}
	if len(views[0].Inbox) != 0 {
		t.Fatalf("first view = %+v, want empty", views[0])
	}
	last := views[2]
	if len(last.Inbox) != 2 || last.Counts.Blocked != 2 {
		t.Fatalf("last view = %+v, want both agents blocked", last)
	}
}

func TestJumpGoesToTheAdapterThatNamedTheAgent(t *testing.T) {
	hc, t3 := &fakeAdapter{name: "hc"}, &fakeAdapter{name: "t3"}
	id := agent.ID{Orchestrator: "t3", Source: "local", Native: "x"}
	if err := jump(context.Background(), []agent.Adapter{hc, t3}, id); err != nil {
		t.Fatal(err)
	}
	if len(hc.jumps) != 0 || len(t3.jumps) != 1 || t3.jumps[0] != id {
		t.Fatalf("hc got %v, t3 got %v", hc.jumps, t3.jumps)
	}
	if err := jump(context.Background(), []agent.Adapter{hc, t3}, agent.ID{Orchestrator: "gone"}); err == nil {
		t.Fatal("jumped to an orchestrator nobody runs")
	}
}

func TestOpenAppRoutesToSoleAdapter(t *testing.T) {
	t3 := &fakeAdapter{name: "t3"}
	if err := openApp(context.Background(), []agent.Adapter{t3}); err != nil {
		t.Fatalf("openApp = %v", err)
	}
	if t3.opens != 1 {
		t.Fatalf("opens = %d, want 1", t3.opens)
	}
	if err := openApp(context.Background(), nil); err == nil {
		t.Fatal("openApp succeeded with no adapters")
	}
	if err := openApp(context.Background(), []agent.Adapter{t3, &fakeAdapter{name: "hc"}}); err == nil {
		t.Fatal("openApp succeeded with multiple adapters")
	}
}
