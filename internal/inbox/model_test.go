package inbox

import (
	"reflect"
	"testing"
	"time"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
)

var t0 = time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)

func mins(n int) time.Time { return t0.Add(time.Duration(n) * time.Minute) }

func id(native string) agent.ID {
	return agent.ID{Orchestrator: "t3", Source: "local", Native: native}
}

type mod func(*agent.Agent)

func since(t time.Time) mod      { return func(a *agent.Agent) { a.Since = t } }
func parent(p string) mod        { return func(a *agent.Agent) { a.Parent = p } }
func why(w agent.BlockedWhy) mod { return func(a *agent.Agent) { a.BlockedWhy = w } }
func withID(i agent.ID) mod      { return func(a *agent.Agent) { a.ID = i; a.Title = i.Native } }

func ag(native string, s agent.State, mods ...mod) agent.Agent {
	a := agent.Agent{ID: id(native), Info: agent.Info{Title: native}, State: s}
	for _, m := range mods {
		m(&a)
	}
	return a
}

type rmod func(*Row)

func rwhy(w agent.BlockedWhy) rmod { return func(r *Row) { r.BlockedWhy = w } }
func kids(n int) rmod              { return func(r *Row) { r.WorkingChildren = n } }
func rid(i agent.ID) rmod          { return func(r *Row) { r.ID = i; r.Title = i.Native } }

func row(native string, s agent.State, at time.Time, mods ...rmod) Row {
	r := Row{ID: id(native), Info: agent.Info{Title: native}, State: s, At: at}
	for _, m := range mods {
		m(&r)
	}
	return r
}

type step struct {
	ev  agent.Event
	now time.Time
}

func up(a agent.Agent, now time.Time) step { return step{agent.Upsert{Agent: a}, now} }

func run(steps []step) View {
	m := New()
	for _, s := range steps {
		m.Apply(s.ev, s.now)
	}
	return m.View()
}

type rowsCase struct {
	name    string
	steps   []step
	inbox   []Row
	running []Row
}

func checkRows(t *testing.T, cases []rowsCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := run(c.steps)
			want := c.inbox
			if want == nil {
				want = []Row{}
			}
			if !reflect.DeepEqual(v.Inbox, want) {
				t.Errorf("inbox\n got %+v\nwant %+v", v.Inbox, want)
			}
			want = c.running
			if want == nil {
				want = []Row{}
			}
			if !reflect.DeepEqual(v.Running, want) {
				t.Errorf("running\n got %+v\nwant %+v", v.Running, want)
			}
		})
	}
}

func TestTimes(t *testing.T) {
	checkRows(t, []rowsCase{
		{
			name:  "first sight not working uses Since",
			steps: []step{up(ag("a", agent.Idle, since(mins(-5))), t0)},
			inbox: []Row{row("a", agent.Idle, mins(-5))},
		},
		{
			name:  "first sight not working clamps a future Since to now",
			steps: []step{up(ag("a", agent.Done, since(mins(5))), t0)},
			inbox: []Row{row("a", agent.Done, t0)},
		},
		{
			name:  "first sight not working with zero Since uses now",
			steps: []step{up(ag("a", agent.Blocked), t0)},
			inbox: []Row{row("a", agent.Blocked, t0)},
		},
		{
			name:    "first sight working uses Since as started",
			steps:   []step{up(ag("a", agent.Working, since(mins(-5))), t0)},
			running: []Row{row("a", agent.Working, mins(-5))},
		},
		{
			name:    "first sight working clamps a future Since to now",
			steps:   []step{up(ag("a", agent.Working, since(mins(5))), t0)},
			running: []Row{row("a", agent.Working, t0)},
		},
		{
			name:    "first sight working with zero Since uses now",
			steps:   []step{up(ag("a", agent.Working), t0)},
			running: []Row{row("a", agent.Working, t0)},
		},
		{
			name: "entering blocked sets now, not Since",
			steps: []step{
				up(ag("a", agent.Idle, since(mins(-10))), t0),
				up(ag("a", agent.Blocked, since(mins(-3))), mins(1)),
			},
			inbox: []Row{row("a", agent.Blocked, mins(1))},
		},
		{
			name: "entering error sets now",
			steps: []step{
				up(ag("a", agent.Blocked), t0),
				up(ag("a", agent.Error), mins(2)),
			},
			inbox: []Row{row("a", agent.Error, mins(2))},
		},
		{
			name: "done to idle keeps the returned time",
			steps: []step{
				up(ag("a", agent.Done), t0),
				up(ag("a", agent.Idle, since(mins(5))), mins(5)),
			},
			inbox: []Row{row("a", agent.Idle, t0)},
		},
		{
			name: "done again after idle sets now",
			steps: []step{
				up(ag("a", agent.Done), t0),
				up(ag("a", agent.Idle), mins(5)),
				up(ag("a", agent.Done), mins(10)),
			},
			inbox: []Row{row("a", agent.Done, mins(10))},
		},
		{
			name: "idle to idle keeps the returned time",
			steps: []step{
				up(ag("a", agent.Idle, since(mins(-10))), t0),
				up(ag("a", agent.Idle, since(mins(-1))), mins(5)),
			},
			inbox: []Row{row("a", agent.Idle, mins(-10))},
		},
		{
			name: "blocked with a new reason keeps the returned time",
			steps: []step{
				up(ag("a", agent.Blocked, why(agent.WhyPermission)), t0),
				up(ag("a", agent.Blocked, why(agent.WhyQuestion)), mins(5)),
			},
			inbox: []Row{row("a", agent.Blocked, t0, rwhy(agent.WhyQuestion))},
		},
		{
			name: "leaving working to idle sets now",
			steps: []step{
				up(ag("a", agent.Idle, since(mins(-10))), t0),
				up(ag("a", agent.Working), mins(2)),
				up(ag("a", agent.Idle, since(mins(-10))), mins(7)),
			},
			inbox: []Row{row("a", agent.Idle, mins(7))},
		},
		{
			name: "entering working sets started to now, not Since",
			steps: []step{
				up(ag("a", agent.Idle), t0),
				up(ag("a", agent.Working, since(mins(-100))), mins(3)),
			},
			running: []Row{row("a", agent.Working, mins(3))},
		},
		{
			name: "staying working keeps the started time",
			steps: []step{
				up(ag("a", agent.Working), t0),
				up(ag("a", agent.Working, since(mins(4))), mins(5)),
			},
			running: []Row{row("a", agent.Working, t0)},
		},
	})
}

func TestSort(t *testing.T) {
	x := agent.ID{Orchestrator: "x", Source: "a", Native: "a"}
	y := agent.ID{Orchestrator: "t3", Source: "z", Native: "a"}
	z := agent.ID{Orchestrator: "t3", Source: "y", Native: "b"}
	w := agent.ID{Orchestrator: "t3", Source: "y", Native: "a"}
	checkRows(t, []rowsCase{
		{
			name: "inbox newest returned first",
			steps: []step{
				up(ag("a", agent.Idle, since(mins(-30))), t0),
				up(ag("b", agent.Done, since(mins(-10))), t0),
				up(ag("c", agent.Blocked, since(mins(-20))), t0),
			},
			inbox: []Row{
				row("b", agent.Done, mins(-10)),
				row("c", agent.Blocked, mins(-20)),
				row("a", agent.Idle, mins(-30)),
			},
		},
		{
			name: "running newest started first",
			steps: []step{
				up(ag("a", agent.Working, since(mins(-30))), t0),
				up(ag("b", agent.Working, since(mins(-10))), t0),
				up(ag("c", agent.Working, since(mins(-20))), t0),
			},
			running: []Row{
				row("b", agent.Working, mins(-10)),
				row("c", agent.Working, mins(-20)),
				row("a", agent.Working, mins(-30)),
			},
		},
		{
			name: "ties broken by orchestrator, source, native",
			steps: []step{
				up(ag("", agent.Idle, withID(x)), t0),
				up(ag("", agent.Idle, withID(y)), t0),
				up(ag("", agent.Idle, withID(z)), t0),
				up(ag("", agent.Idle, withID(w)), t0),
			},
			inbox: []Row{
				row("", agent.Idle, t0, rid(w)),
				row("", agent.Idle, t0, rid(z)),
				row("", agent.Idle, t0, rid(y)),
				row("", agent.Idle, t0, rid(x)),
			},
		},
	})
}

func TestEmptyView(t *testing.T) {
	got := New().View()
	want := View{Disconnected: []Disconnect{}, Inbox: []Row{}, Running: []Row{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestFold(t *testing.T) {
	other := agent.ID{Orchestrator: "t3", Source: "remote", Native: "c"}
	checkRows(t, []rowsCase{
		{
			name: "blocked child shows the parent blocked with the child's reason and time",
			steps: []step{
				up(ag("p", agent.Idle, since(mins(-30))), t0),
				up(ag("c", agent.Blocked, parent("p"), why(agent.WhyPermission), since(mins(-4))), t0),
			},
			inbox: []Row{row("p", agent.Blocked, mins(-4), rwhy(agent.WhyPermission))},
		},
		{
			name: "error child shows the parent blocked with WhyOther",
			steps: []step{
				up(ag("p", agent.Idle, since(mins(-30))), t0),
				up(ag("c", agent.Error, parent("p"), why(agent.WhyQuestion), since(mins(-4))), t0),
			},
			inbox: []Row{row("p", agent.Blocked, mins(-4))},
		},
		{
			name: "several blocked children: the most recent one decides",
			steps: []step{
				up(ag("p", agent.Idle, since(mins(-30))), t0),
				up(ag("c1", agent.Blocked, parent("p"), why(agent.WhyPermission), since(mins(-9))), t0),
				up(ag("c2", agent.Blocked, parent("p"), why(agent.WhyQuestion), since(mins(-2))), t0),
				up(ag("c3", agent.Error, parent("p"), since(mins(-5))), t0),
			},
			inbox: []Row{row("p", agent.Blocked, mins(-2), rwhy(agent.WhyQuestion))},
		},
		{
			name: "parent's own blocked beats a working child",
			steps: []step{
				up(ag("p", agent.Blocked, why(agent.WhyQuestion), since(mins(-5))), t0),
				up(ag("c", agent.Working, parent("p"), since(mins(-1))), t0),
			},
			inbox: []Row{row("p", agent.Blocked, mins(-5), rwhy(agent.WhyQuestion), kids(1))},
		},
		{
			name: "parent's own error beats a blocked child",
			steps: []step{
				up(ag("p", agent.Error, since(mins(-8))), t0),
				up(ag("c", agent.Blocked, parent("p"), why(agent.WhyPermission), since(mins(-1))), t0),
			},
			inbox: []Row{row("p", agent.Error, mins(-8))},
		},
		{
			name: "parent done beats a working child",
			steps: []step{
				up(ag("p", agent.Done, since(mins(-6))), t0),
				up(ag("c", agent.Working, parent("p"), since(mins(-1))), t0),
			},
			inbox: []Row{row("p", agent.Done, mins(-6), kids(1))},
		},
		{
			name: "a done child does not alert",
			steps: []step{
				up(ag("p", agent.Idle, since(mins(-10))), t0),
				up(ag("c", agent.Done, parent("p"), since(mins(-1))), t0),
			},
			inbox: []Row{row("p", agent.Idle, mins(-10))},
		},
		{
			name: "working children make an idle parent working from the earliest child start",
			steps: []step{
				up(ag("p", agent.Idle, since(mins(-30))), t0),
				up(ag("c1", agent.Working, parent("p"), since(mins(-3))), t0),
				up(ag("c2", agent.Working, parent("p"), since(mins(-8))), t0),
			},
			running: []Row{row("p", agent.Working, mins(-8), kids(2))},
		},
		{
			name: "a working parent uses its own start",
			steps: []step{
				up(ag("p", agent.Working, since(mins(-2))), t0),
				up(ag("c", agent.Working, parent("p"), since(mins(-8))), t0),
			},
			running: []Row{row("p", agent.Working, mins(-2), kids(1))},
		},
		{
			name: "otherwise the parent's own state and time",
			steps: []step{
				up(ag("p", agent.Idle, since(mins(-10))), t0),
				up(ag("c", agent.Idle, parent("p"), since(mins(-1))), t0),
			},
			inbox: []Row{row("p", agent.Idle, mins(-10))},
		},
		{
			name: "grandchildren fold into the topmost parent and count at any depth",
			steps: []step{
				up(ag("p", agent.Idle, since(mins(-30))), t0),
				up(ag("c", agent.Working, parent("p"), since(mins(-6))), t0),
				up(ag("g", agent.Working, parent("c"), since(mins(-3))), t0),
				up(ag("gg", agent.Blocked, parent("g"), why(agent.WhyQuestion), since(mins(-1))), t0),
			},
			inbox: []Row{row("p", agent.Blocked, mins(-1), rwhy(agent.WhyQuestion), kids(2))},
		},
		{
			name: "a child whose parent is absent is a top row and gathers its own children",
			steps: []step{
				up(ag("c", agent.Idle, parent("gone"), since(mins(-10))), t0),
				up(ag("g", agent.Blocked, parent("c"), since(mins(-2))), t0),
			},
			inbox: []Row{row("c", agent.Blocked, mins(-2))},
		},
		{
			name: "a parent in another source does not fold",
			steps: []step{
				up(ag("p", agent.Idle, since(mins(-10))), t0),
				up(ag("", agent.Blocked, withID(other), parent("p"), since(mins(-2))), t0),
			},
			inbox: []Row{
				row("", agent.Blocked, mins(-2), rid(other)),
				row("p", agent.Idle, mins(-10)),
			},
		},
		{
			name: "agents in a parent cycle are top rows and folding terminates",
			steps: []step{
				up(ag("a", agent.Idle, parent("b"), since(mins(-1))), t0),
				up(ag("b", agent.Idle, parent("a"), since(mins(-2))), t0),
				up(ag("c", agent.Blocked, parent("a"), since(mins(-3))), t0),
				up(ag("s", agent.Idle, parent("s"), since(mins(-4))), t0),
			},
			inbox: []Row{
				row("b", agent.Idle, mins(-2)),
				row("a", agent.Blocked, mins(-3)),
				row("s", agent.Idle, mins(-4)),
			},
		},
	})
}

func TestFoldFieldsComeFromTop(t *testing.T) {
	p := ag("p", agent.Idle, since(mins(-10)))
	p.Project, p.Where, p.Kind = "perch", "~/perch", "claude"
	c := ag("c", agent.Blocked, parent("p"), since(mins(-1)))
	c.Project, c.Where, c.Kind = "other", "~/other", "codex"
	v := run([]step{up(p, t0), up(c, t0)})
	want := []Row{{
		ID: id("p"), Info: agent.Info{Project: "perch", Title: "p", Where: "~/perch", Kind: "claude"},
		State: agent.Blocked, At: mins(-1),
	}}
	if !reflect.DeepEqual(v.Inbox, want) {
		t.Fatalf("got %+v, want %+v", v.Inbox, want)
	}
}

func remote(native string) agent.ID {
	return agent.ID{Orchestrator: "t3", Source: "remote", Native: native}
}

func snap(now time.Time, agents ...agent.Agent) step {
	return step{agent.Snapshot{Orchestrator: "t3", Source: "local", Agents: agents}, now}
}

func down(orch, src string, r agent.Reason, now time.Time) step {
	return step{agent.SourceStatus{Orchestrator: orch, Source: src, Reason: r}, now}
}

func TestSources(t *testing.T) {
	cases := []struct {
		name  string
		steps []step
		want  View
	}{
		{
			name: "snapshot replaces its source's agents and leaves other sources",
			steps: []step{
				up(ag("a", agent.Idle), t0),
				up(ag("b", agent.Idle), t0),
				up(ag("", agent.Idle, withID(remote("r"))), t0),
				snap(t0, ag("b", agent.Idle), ag("c", agent.Idle)),
			},
			want: View{
				Disconnected: []Disconnect{},
				Inbox: []Row{
					row("b", agent.Idle, t0),
					row("c", agent.Idle, t0),
					row("", agent.Idle, t0, rid(remote("r"))),
				},
				Running: []Row{},
				Counts:  Counts{Idle: 3},
			},
		},
		{
			name: "an agent kept across snapshots keeps its times",
			steps: []step{
				snap(t0, ag("a", agent.Done)),
				snap(mins(5), ag("a", agent.Done, since(mins(4)))),
			},
			want: View{
				Disconnected: []Disconnect{},
				Inbox:        []Row{row("a", agent.Done, t0)},
				Running:      []Row{},
				Counts:       Counts{Done: 1},
			},
		},
		{
			name: "an agent that leaves a snapshot forgets its times",
			steps: []step{
				snap(t0, ag("a", agent.Done)),
				snap(mins(5)),
				snap(mins(10), ag("a", agent.Idle)),
			},
			want: View{
				Disconnected: []Disconnect{},
				Inbox:        []Row{row("a", agent.Idle, mins(10))},
				Running:      []Row{},
				Counts:       Counts{Idle: 1},
			},
		},
		{
			name: "snapshot marks its source connected",
			steps: []step{
				down("t3", "local", agent.ReasonReconnecting, t0),
				down("t3", "remote", agent.ReasonAppNotRunning, t0),
				snap(mins(1)),
			},
			want: View{
				Disconnected: []Disconnect{{Orchestrator: "t3", Reason: agent.ReasonAppNotRunning}},
				Inbox:        []Row{},
				Running:      []Row{},
			},
		},
		{
			name: "down removes its source's agents and adds a line",
			steps: []step{
				up(ag("a", agent.Blocked), t0),
				up(ag("", agent.Working, withID(remote("r"))), t0),
				down("t3", "local", agent.ReasonAppNotRunning, mins(1)),
			},
			want: View{
				Disconnected: []Disconnect{{Orchestrator: "t3", Reason: agent.ReasonAppNotRunning}},
				Inbox:        []Row{},
				Running:      []Row{row("", agent.Working, t0, rid(remote("r")))},
				Counts:       Counts{Working: 1},
			},
		},
		{
			name: "a second down updates the line",
			steps: []step{
				down("t3", "local", agent.ReasonReconnecting, t0),
				down("t3", "local", agent.ReasonAppNotRunning, mins(1)),
			},
			want: View{
				Disconnected: []Disconnect{{Orchestrator: "t3", Reason: agent.ReasonAppNotRunning}},
				Inbox:        []Row{},
				Running:      []Row{},
			},
		},
		{
			name: "up removes the line",
			steps: []step{
				down("t3", "local", agent.ReasonReconnecting, t0),
				{agent.SourceStatus{Orchestrator: "t3", Source: "local", Up: true}, mins(1)},
			},
			want: View{Disconnected: []Disconnect{}, Inbox: []Row{}, Running: []Row{}},
		},
		{
			name: "down forgets times, and an upsert to a down source still applies",
			steps: []step{
				up(ag("a", agent.Done), t0),
				down("t3", "local", agent.ReasonReconnecting, mins(1)),
				up(ag("a", agent.Idle), mins(5)),
			},
			want: View{
				Disconnected: []Disconnect{{Orchestrator: "t3", Reason: agent.ReasonReconnecting}},
				Inbox:        []Row{row("a", agent.Idle, mins(5))},
				Running:      []Row{},
				Counts:       Counts{Idle: 1},
			},
		},
		{
			name: "disconnect lines sort by orchestrator, then source",
			steps: []step{
				down("x", "a", agent.ReasonNoCommand, t0),
				down("t3", "z", agent.ReasonNoCommand, t0),
				down("t3", "y", agent.ReasonReconnecting, t0),
			},
			want: View{
				Disconnected: []Disconnect{
					{Orchestrator: "t3", Reason: agent.ReasonReconnecting},
					{Orchestrator: "t3", Reason: agent.ReasonNoCommand},
					{Orchestrator: "x", Reason: agent.ReasonNoCommand},
				},
				Inbox:   []Row{},
				Running: []Row{},
			},
		},
		{
			name: "remove deletes one agent",
			steps: []step{
				up(ag("a", agent.Idle), t0),
				up(ag("b", agent.Idle), t0),
				{agent.Remove{ID: id("a")}, mins(1)},
			},
			want: View{Disconnected: []Disconnect{}, Inbox: []Row{row("b", agent.Idle, t0)}, Running: []Row{}, Counts: Counts{Idle: 1}},
		},
		{
			name: "removing a parent makes its child a top row",
			steps: []step{
				up(ag("p", agent.Idle), t0),
				up(ag("c", agent.Blocked, parent("p")), t0),
				{agent.Remove{ID: id("p")}, mins(1)},
			},
			want: View{
				Disconnected: []Disconnect{},
				Inbox:        []Row{row("c", agent.Blocked, t0)},
				Running:      []Row{},
				Counts:       Counts{Blocked: 1},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := run(c.steps); !reflect.DeepEqual(got, c.want) {
				t.Errorf("\n got %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestCountsFollowFoldedRows(t *testing.T) {
	v := run([]step{
		up(ag("p1", agent.Idle), t0),
		up(ag("p1c", agent.Blocked, parent("p1")), t0),
		up(ag("p2", agent.Blocked), t0),
		up(ag("p2c", agent.Working, parent("p2")), t0),
		up(ag("d", agent.Done), t0),
		up(ag("dc", agent.Done, parent("d")), t0),
		up(ag("e", agent.Error), t0),
		up(ag("ec", agent.Error, parent("e")), t0),
		up(ag("w1", agent.Working), t0),
		up(ag("w2", agent.Idle), t0),
		up(ag("w2c", agent.Working, parent("w2")), t0),
		up(ag("i", agent.Idle), t0),
	})
	want := Counts{Error: 1, Blocked: 2, Done: 1, Idle: 1, Working: 2}
	if v.Counts != want {
		t.Fatalf("got %+v, want %+v", v.Counts, want)
	}
}
