package t3

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
)

const testEnv = "9c1f5e2a-4b7d-4f10-8e3a-6d5c4b3a2f10"

var (
	testNow   = mustTime("2026-11-20T10:05:00Z")
	testKinds = map[string]string{"claude-work": "claudeAgent", "codex": "codex"}
)

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func tid(n int) string { return fmt.Sprintf("8f2a6c10-3b7d-4e91-a5c4-%012d", n) }

func native(n int) string { return testEnv + "/" + tid(n) }

func decode(t *testing.T, b []byte) item {
	t.Helper()
	it, err := decodeItem(b)
	if err != nil {
		t.Fatal(err)
	}
	return it
}

// fixtureThread returns thread n of the snapshot fixture as raw JSON fields, changed by kv pairs.
func fixtureThread(t *testing.T, n int, kv ...any) map[string]any {
	t.Helper()
	var snap struct {
		Snapshot struct {
			Threads []map[string]any `json:"threads"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(readTestdata(t, "shell-snapshot.json"), &snap); err != nil {
		t.Fatal(err)
	}
	for _, th := range snap.Snapshot.Threads {
		if th["id"] == tid(n) {
			for i := 0; i < len(kv); i += 2 {
				th[kv[i].(string)] = kv[i+1]
			}
			return th
		}
	}
	t.Fatalf("no thread %d in fixture", n)
	return nil
}

func encode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func threadUpdated(t *testing.T, seq int, location string, th map[string]any) item {
	t.Helper()
	return decode(t, encode(t, map[string]any{"kind": "thread.updated", "sequence": seq, "location": location, "thread": th}))
}

func iso(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

func stopped(t *testing.T, n int, at time.Time, kv ...any) map[string]any {
	t.Helper()
	kv = append([]any{
		"status", "completed", "activeRunId", nil, "activityRunStatus", nil, "activityRunStartedAt", nil,
		"latestRunCompletedAt", iso(at), "updatedAt", iso(at),
	}, kv...)
	return fixtureThread(t, n, kv...)
}

func newFixtureTracker(t *testing.T) *tracker {
	t.Helper()
	tr := newTracker(testEnv, testKinds)
	tr.apply(decode(t, readTestdata(t, "shell-snapshot.json")), testNow)
	return tr
}

func id(n int) agent.ID { return agent.ID{Orchestrator: "t3", Source: "local", Native: native(n)} }

// A blocked thread's time is when its request came, not when its run started.
func fixtureAgents() []agent.Agent {
	a := func(n int, title, project, where string, kind agent.Kind, st agent.State, why agent.BlockedWhy, since string) agent.Agent {
		return agent.Agent{ID: id(n), Info: agent.Info{Project: project, Title: title, Where: where, Kind: kind}, State: st, BlockedWhy: why, Since: mustTime(since)}
	}
	child := a(10, "Review the parser changes", "perch", "/Users/dev/perch", "codex", agent.Blocked, agent.WhyPermission, "2026-11-20T10:03:00Z")
	child.Parent = native(1)
	return []agent.Agent{
		a(1, "Refactor the parser", "perch", "feat/parser", "claude", agent.Working, "", "2026-11-20T09:58:00Z"),
		a(2, "Write release notes", "perch", "/Users/dev/worktrees/notes", "codex", agent.Done, "", "2026-11-20T10:02:00Z"),
		a(3, "Fix the login bug", "perch", "/Users/dev/perch", "codex", agent.Idle, "", "2026-11-20T09:30:00Z"),
		a(4, "Migrate the database", "perch", "/Users/dev/perch", "codex", agent.Blocked, agent.WhyPermission, "2026-11-20T10:03:00Z"),
		a(5, "Pick a name", "perch", "/Users/dev/perch", "codex", agent.Blocked, agent.WhyQuestion, "2026-11-20T10:03:00Z"),
		a(6, "Deploy preview", "perch", "/Users/dev/perch", "codex", agent.Error, "", "2026-11-20T10:02:00Z"),
		a(7, "Pinned notes", "perch", "/Users/dev/perch", "codex", agent.Idle, "", "2026-11-20T10:02:00Z"),
		child,
		a(11, "Fix the login bug fork", "t3code", "/Users/dev/t3code", "codex", agent.Idle, "", "2026-11-20T09:50:00Z"),
		a(13, "Plan the auth rewrite", "t3code", "/Users/dev/t3code", "codex", agent.Blocked, agent.WhyPermission, "2026-11-20T10:02:00Z"),
		a(14, "Shared with a guest", "t3code", "/Users/dev/t3code", "codex", agent.Blocked, agent.WhyOther, "2026-11-20T10:02:00Z"),
	}
}

func fixtureAgent(n int) agent.Agent {
	for _, a := range fixtureAgents() {
		if a.ID.Native == native(n) {
			return a
		}
	}
	panic(n)
}

func expectEvents(t *testing.T, got []agent.Event, want ...agent.Event) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events\n %+v\nwant\n %+v", got, want)
	}
}

func TestSnapshotMapsThreads(t *testing.T) {
	tr := newTracker(testEnv, testKinds)
	got := tr.apply(decode(t, readTestdata(t, "shell-snapshot.json")), testNow)
	expectEvents(t, got, agent.Snapshot{Orchestrator: "t3", Source: "local", Agents: fixtureAgents()})
}

func TestEventsBeforeSnapshotAreIgnored(t *testing.T) {
	tr := newTracker(testEnv, testKinds)
	expectEvents(t, tr.apply(threadUpdated(t, 121, "active", fixtureThread(t, 3)), testNow))
	expectEvents(t, tr.apply(decode(t, []byte(`{"kind":"synchronized"}`)), testNow))
}

func TestPatchSnapshotKeepsThreads(t *testing.T) {
	tr := newFixtureTracker(t)
	expectEvents(t, tr.apply(decode(t, readTestdata(t, "shell-patch.json")), testNow))
	expectEvents(t, tr.apply(decode(t, []byte(`{"kind":"thread.removed","sequence":122,"location":"active","threadId":"`+tid(3)+`"}`)), testNow),
		agent.Remove{ID: id(3)})
}

func TestRepeatedSnapshotEmitsOnlyOnChange(t *testing.T) {
	tr := newFixtureTracker(t)
	expectEvents(t, tr.apply(decode(t, readTestdata(t, "shell-snapshot.json")), testNow))

	var snap map[string]any
	json.Unmarshal(readTestdata(t, "shell-snapshot.json"), &snap)
	inner := snap["snapshot"].(map[string]any)
	inner["threads"] = inner["threads"].([]any)[:1]
	expectEvents(t, tr.apply(decode(t, encode(t, snap)), testNow),
		agent.Snapshot{Orchestrator: "t3", Source: "local", Agents: fixtureAgents()[:1]})
}

func TestVisitAfterCompletionIsIdle(t *testing.T) {
	tr := newFixtureTracker(t)
	got := tr.apply(threadUpdated(t, 121, "active", fixtureThread(t, 2, "lastVisitedAt", "2026-11-20T10:04:00.000Z")), testNow)
	want := fixtureAgent(2)
	want.State = agent.Idle
	expectEvents(t, got, agent.Upsert{Agent: want})
}

func TestNeverVisitedCompletionIsIdle(t *testing.T) {
	tr := newFixtureTracker(t)
	got := tr.apply(threadUpdated(t, 121, "active", fixtureThread(t, 2, "lastVisitedAt", nil)), testNow)
	want := fixtureAgent(2)
	want.State = agent.Idle
	expectEvents(t, got, agent.Upsert{Agent: want})
}

func TestInterruptedRunIsNotDone(t *testing.T) {
	tr := newFixtureTracker(t)
	got := tr.apply(threadUpdated(t, 121, "active", fixtureThread(t, 2, "status", "interrupted")), testNow)
	want := fixtureAgent(2)
	want.State = agent.Idle
	expectEvents(t, got, agent.Upsert{Agent: want})
}

func TestUnchangedUpdateEmitsNothing(t *testing.T) {
	tr := newFixtureTracker(t)
	expectEvents(t, tr.apply(threadUpdated(t, 121, "active", fixtureThread(t, 3)), testNow))
}

func TestStaleSequenceIsIgnored(t *testing.T) {
	tr := newFixtureTracker(t)
	expectEvents(t, tr.apply(threadUpdated(t, 120, "active", fixtureThread(t, 3, "settledOverride", "settled")), testNow))
}

func TestExcludedThreadsAreRemoved(t *testing.T) {
	cases := map[string]item{}
	cases["settled"] = threadUpdated(t, 121, "active", fixtureThread(t, 3, "settledOverride", "settled"))
	cases["archived"] = threadUpdated(t, 121, "archive", fixtureThread(t, 3, "archivedAt", "2026-11-20T10:04:00.000Z"))
	cases["deleted"] = threadUpdated(t, 121, "active", fixtureThread(t, 3, "deletedAt", "2026-11-20T10:04:00.000Z"))
	cases["snoozed"] = threadUpdated(t, 121, "active", fixtureThread(t, 3, "snoozedAt", "2026-11-20T10:04:00.000Z", "snoozedUntil", "2026-11-20T12:00:00.000Z"))
	cases["removed"] = decode(t, []byte(`{"kind":"thread.removed","sequence":121,"location":"active","threadId":"`+tid(3)+`"}`))
	for name, it := range cases {
		t.Run(name, func(t *testing.T) {
			tr := newFixtureTracker(t)
			expectEvents(t, tr.apply(it, testNow), agent.Remove{ID: id(3)})
		})
	}
}

func TestSnoozeEndsEarlyOnApproval(t *testing.T) {
	tr := newFixtureTracker(t)
	got := tr.apply(threadUpdated(t, 121, "active", fixtureThread(t, 8,
		"pendingRuntimeRequest", map[string]any{"id": "req-1", "kind": "command", "createdAt": "2026-11-20T10:04:00.000Z"})), testNow)
	want := agent.Agent{ID: id(8), Info: agent.Info{Project: "perch", Title: "Snoozed until tomorrow", Where: "/Users/dev/perch", Kind: "codex"},
		State: agent.Blocked, BlockedWhy: agent.WhyPermission, Since: mustTime("2026-11-20T10:04:00Z")}
	expectEvents(t, got, agent.Upsert{Agent: want})
}

func TestSnoozeKeepsAnOldQuestion(t *testing.T) {
	tr := newFixtureTracker(t)
	expectEvents(t, tr.apply(threadUpdated(t, 121, "active", fixtureThread(t, 8,
		"pendingRuntimeRequest", map[string]any{"id": "req-1", "kind": "user_input", "createdAt": "2026-11-20T10:00:00.000Z"})), testNow))
}

func TestSnoozeWakesAtItsDeadline(t *testing.T) {
	tr := newFixtureTracker(t)
	wake := mustTime("2026-11-21T09:00:00Z")
	if d, ok := tr.deadline(); !ok || !d.Equal(wake) {
		t.Fatalf("deadline = %v %v, want %v", d, ok, wake)
	}
	want := agent.Agent{ID: id(8), Info: agent.Info{Project: "perch", Title: "Snoozed until tomorrow", Where: "/Users/dev/perch", Kind: "codex"},
		State: agent.Done, Since: mustTime("2026-11-20T10:02:00Z")}
	expectEvents(t, tr.expire(wake), agent.Upsert{Agent: want})
	if _, ok := tr.deadline(); ok {
		t.Fatal("deadline still set after the snooze ended")
	}
}

func TestProjectUpdateRenamesItsThreads(t *testing.T) {
	tr := newFixtureTracker(t)
	var snap struct {
		Snapshot struct {
			Projects []map[string]any `json:"projects"`
		} `json:"snapshot"`
	}
	json.Unmarshal(readTestdata(t, "shell-snapshot.json"), &snap)
	p := snap.Snapshot.Projects[1]
	p["title"] = "t3"
	got := tr.apply(decode(t, encode(t, map[string]any{"kind": "project.updated", "sequence": 121, "project": p})), testNow)
	var want []agent.Event
	for _, n := range []int{11, 13, 14} {
		a := fixtureAgent(n)
		a.Project = "t3"
		want = append(want, agent.Upsert{Agent: a})
	}
	expectEvents(t, got, want...)

	got = tr.apply(decode(t, []byte(`{"kind":"project.removed","sequence":122,"projectId":"5b0d3c1e-7a4f-4c1b-9d2e-1f6a8b3c4d02"}`)), testNow)
	want = nil
	for _, n := range []int{11, 13, 14} {
		a := fixtureAgent(n)
		a.Project, a.Where = "", ""
		want = append(want, agent.Upsert{Agent: a})
	}
	expectEvents(t, got, want...)
}

func TestWorkingHoldsThroughTheGrace(t *testing.T) {
	tr := newFixtureTracker(t)
	held := fixtureAgent(1)
	held.Since = testNow
	expectEvents(t, tr.apply(threadUpdated(t, 121, "active", stopped(t, 1, testNow)), testNow), agent.Upsert{Agent: held})
	d, ok := tr.deadline()
	if want := testNow.Add(2 * time.Second); !ok || !d.Equal(want) {
		t.Fatalf("deadline = %v %v, want %v", d, ok, want)
	}
	expectEvents(t, tr.expire(testNow.Add(time.Second)))

	want := fixtureAgent(1)
	want.State, want.Since = agent.Done, testNow
	expectEvents(t, tr.expire(testNow.Add(2*time.Second)), agent.Upsert{Agent: want})
	if d, _ := tr.deadline(); d.Before(testNow.Add(time.Hour)) {
		t.Fatalf("grace deadline still armed: %v", d)
	}
}

func TestRestartWithinTheGraceStaysWorking(t *testing.T) {
	tr := newFixtureTracker(t)
	held := fixtureAgent(1)
	held.Since = testNow
	expectEvents(t, tr.apply(threadUpdated(t, 121, "active", stopped(t, 1, testNow)), testNow), agent.Upsert{Agent: held})
	later := testNow.Add(time.Second)
	restarted := fixtureThread(t, 1, "latestRunId", "run-1b", "activeRunId", "run-1b", "latestRunRequestedAt", iso(later), "updatedAt", iso(later))
	got := tr.apply(threadUpdated(t, 122, "active", restarted), later)
	want := fixtureAgent(1)
	want.Since = later
	expectEvents(t, got, agent.Upsert{Agent: want})
	if d, _ := tr.deadline(); d.Before(testNow.Add(time.Hour)) {
		t.Fatalf("grace deadline still armed: %v", d)
	}
}

func TestGraceIsSkipped(t *testing.T) {
	cases := map[string]struct {
		thread map[string]any
		state  agent.State
		why    agent.BlockedWhy
		since  time.Time
	}{
		"blocked": {stopped(t, 1, testNow, "pendingRuntimeRequest", map[string]any{"id": "r", "kind": "permission", "createdAt": iso(testNow)}), agent.Blocked, agent.WhyPermission, testNow},
		"error":   {stopped(t, 1, testNow, "status", "failed"), agent.Error, "", testNow},
		"stale":   {stopped(t, 1, testNow.Add(-time.Minute)), agent.Done, "", testNow.Add(-time.Minute)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			tr := newFixtureTracker(t)
			want := fixtureAgent(1)
			want.State, want.BlockedWhy, want.Since = c.state, c.why, c.since
			expectEvents(t, tr.apply(threadUpdated(t, 121, "active", c.thread), testNow), agent.Upsert{Agent: want})
		})
	}
}

func TestMovedThreadSkipsTheGrace(t *testing.T) {
	tr := newFixtureTracker(t)
	want := fixtureAgent(1)
	want.State, want.Since = agent.Done, testNow
	expectEvents(t, tr.apply(threadUpdated(t, 121, "active", stopped(t, 1, testNow, "pinnedAt", iso(testNow))), testNow), agent.Upsert{Agent: want})
}

func TestBackgroundWorkKeepsWorking(t *testing.T) {
	tr := newFixtureTracker(t)
	th := stopped(t, 3, testNow.Add(-10*time.Minute), "pendingBackgroundTasks", []any{map[string]any{"taskId": "t1", "kind": "subagent"}})
	want := fixtureAgent(3)
	want.State, want.Since = agent.Working, testNow.Add(-10*time.Minute)
	expectEvents(t, tr.apply(threadUpdated(t, 121, "active", th), testNow), agent.Upsert{Agent: want})
}

// T3 lists a sub-agent or side chat only under its parent, so it goes when the parent does.
func TestChildGoesWithItsParent(t *testing.T) {
	cases := map[string]item{}
	cases["settled"] = threadUpdated(t, 121, "active", fixtureThread(t, 1, "settledOverride", "settled"))
	cases["archived"] = threadUpdated(t, 121, "archive", fixtureThread(t, 1, "archivedAt", "2026-11-20T10:04:00.000Z"))
	cases["removed"] = decode(t, []byte(`{"kind":"thread.removed","sequence":121,"location":"active","threadId":"`+tid(1)+`"}`))
	for name, it := range cases {
		t.Run(name, func(t *testing.T) {
			tr := newFixtureTracker(t)
			got := tr.apply(it, testNow)
			slices.SortFunc(got, func(a, b agent.Event) int {
				return strings.Compare(a.(agent.Remove).ID.Native, b.(agent.Remove).ID.Native)
			})
			expectEvents(t, got, agent.Remove{ID: id(1)}, agent.Remove{ID: id(10)})
		})
	}
}
