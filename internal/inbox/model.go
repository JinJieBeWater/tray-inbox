package inbox

import (
	"cmp"
	"slices"
	"time"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
)

type tracked struct {
	agent.Agent
	returned time.Time
	started  time.Time
}

type source struct{ orchestrator, source string }

type Model struct {
	agents map[agent.ID]*tracked
	down   map[source]Disconnect
}

func New() *Model {
	return &Model{agents: map[agent.ID]*tracked{}, down: map[source]Disconnect{}}
}

func (m *Model) Apply(ev agent.Event, now time.Time) {
	switch ev := ev.(type) {
	case agent.Snapshot:
		src := source{ev.Orchestrator, ev.Source}
		keep := map[agent.ID]bool{}
		for _, a := range ev.Agents {
			keep[a.ID] = true
		}
		m.drop(src, keep)
		for _, a := range ev.Agents {
			m.upsert(a, now)
		}
		delete(m.down, src)
	case agent.Upsert:
		m.upsert(ev.Agent, now)
	case agent.Remove:
		delete(m.agents, ev.ID)
	case agent.SourceStatus:
		src := source{ev.Orchestrator, ev.Source}
		if ev.Up {
			delete(m.down, src)
			return
		}
		m.drop(src, nil)
		m.down[src] = Disconnect{Orchestrator: ev.Orchestrator, Reason: ev.Reason}
	}
}

func (m *Model) drop(src source, keep map[agent.ID]bool) {
	for id := range m.agents {
		if id.Orchestrator == src.orchestrator && id.Source == src.source && !keep[id] {
			delete(m.agents, id)
		}
	}
}

func (m *Model) upsert(a agent.Agent, now time.Time) {
	t, ok := m.agents[a.ID]
	if !ok {
		first := now
		if !a.Since.IsZero() && a.Since.Before(now) {
			first = a.Since
		}
		t = &tracked{Agent: a}
		if a.State == agent.Working {
			t.started = first
		} else {
			t.returned = first
		}
		m.agents[a.ID] = t
		return
	}
	prev := t.State
	t.Agent = a
	if a.State == prev {
		return
	}
	if alerts(a.State) || prev == agent.Working {
		t.returned = now
	}
	if a.State == agent.Working {
		t.started = now
	}
}

func alerts(s agent.State) bool {
	return s == agent.Blocked || s == agent.Error || s == agent.Done
}

func (m *Model) View() View {
	v := View{Disconnected: []Disconnect{}, Inbox: []Row{}, Running: []Row{}}
	descendants := map[*tracked][]*tracked{}
	for _, t := range m.agents {
		top := m.top(t)
		if top == t {
			if _, ok := descendants[t]; !ok {
				descendants[t] = nil
			}
		} else {
			descendants[top] = append(descendants[top], t)
		}
	}
	for top, ds := range descendants {
		r := fold(top, ds)
		switch r.State {
		case agent.Working:
			v.Running = append(v.Running, r)
			continue
		case agent.Error:
			v.Counts.Error++
		case agent.Blocked:
			v.Counts.Blocked++
		case agent.Done:
			v.Counts.Done++
		case agent.Idle:
			v.Counts.Idle++
		}
		v.Inbox = append(v.Inbox, r)
	}
	v.Counts.Working = len(v.Running)
	slices.SortFunc(v.Inbox, byAt)
	slices.SortFunc(v.Running, byAt)

	srcs := make([]source, 0, len(m.down))
	for s := range m.down {
		srcs = append(srcs, s)
	}
	slices.SortFunc(srcs, func(a, b source) int {
		return cmp.Or(
			cmp.Compare(a.orchestrator, b.orchestrator),
			cmp.Compare(a.source, b.source),
		)
	})
	for _, s := range srcs {
		v.Disconnected = append(v.Disconnected, m.down[s])
	}
	return v
}

// top follows parent links within the source to the topmost present ancestor.
// When the walk revisits an agent, that agent is where a cycle closes and is the top.
func (m *Model) top(t *tracked) *tracked {
	seen := map[*tracked]bool{t: true}
	cur := t
	for cur.Parent != "" {
		p, ok := m.agents[agent.ID{Orchestrator: t.ID.Orchestrator, Source: t.ID.Source, Native: cur.Parent}]
		if !ok {
			break
		}
		if seen[p] {
			return p
		}
		seen[p] = true
		cur = p
	}
	return cur
}

func fold(top *tracked, ds []*tracked) Row {
	slices.SortFunc(ds, func(a, b *tracked) int { return compareID(a.ID, b.ID) })
	r := Row{
		ID:    top.ID,
		Info:  top.Info,
		State: top.State,
		At:    top.returned,
	}
	var blocker, firstWorking *tracked
	for _, d := range ds {
		switch d.State {
		case agent.Blocked, agent.Error:
			if blocker == nil || d.returned.After(blocker.returned) {
				blocker = d
			}
		case agent.Working:
			r.WorkingChildren++
			if firstWorking == nil || d.started.Before(firstWorking.started) {
				firstWorking = d
			}
		}
	}
	switch {
	case top.State == agent.Blocked || top.State == agent.Error:
		r.BlockedWhy = top.BlockedWhy
	case blocker != nil:
		r.State, r.At = agent.Blocked, blocker.returned
		if blocker.State == agent.Blocked {
			r.BlockedWhy = blocker.BlockedWhy
		}
	case top.State == agent.Done:
	case top.State == agent.Working:
		r.At = top.started
	case firstWorking != nil:
		r.State, r.At = agent.Working, firstWorking.started
	}
	return r
}

func byAt(a, b Row) int {
	if c := b.At.Compare(a.At); c != 0 {
		return c
	}
	return compareID(a.ID, b.ID)
}

func compareID(a, b agent.ID) int {
	return cmp.Or(
		cmp.Compare(a.Orchestrator, b.Orchestrator),
		cmp.Compare(a.Source, b.Source),
		cmp.Compare(a.Native, b.Native),
	)
}
