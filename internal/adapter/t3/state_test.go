package t3

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

func fixtureAgents() []agent.Agent {
	a := func(n int, title, project, where string, kind agent.Kind, st agent.State, why agent.BlockedWhy, since, sortAt string) agent.Agent {
		return agent.Agent{
			ID:         id(n),
			Info:       agent.Info{Project: project, Title: title, Where: where, Kind: kind},
			State:      st,
			BlockedWhy: why,
			Since:      mustTime(since),
			SortAt:     mustTime(sortAt),
		}
	}
	return []agent.Agent{
		a(1, "Refactor the parser", "perch", "feat/parser", "claude", agent.Working, "", "2026-11-20T09:58:01Z", "2026-11-20T09:58:00Z"),
		a(2, "Write release notes", "perch", "/Users/dev/worktrees/notes", "codex", agent.Done, "", "2026-11-20T10:02:00Z", "2026-11-20T10:02:00Z"),
		a(3, "Fix the login bug", "perch", "/Users/dev/perch", "codex", agent.Idle, "", "2026-11-20T09:58:01Z", "2026-11-20T09:58:00Z"),
		a(4, "Migrate the database", "perch", "/Users/dev/perch", "codex", agent.Blocked, agent.WhyPermission, "2026-11-20T10:03:00Z", "2026-11-20T09:58:00Z"),
		a(5, "Pick a name", "perch", "/Users/dev/perch", "codex", agent.Blocked, agent.WhyQuestion, "2026-11-20T10:03:00Z", "2026-11-20T09:58:00Z"),
		a(6, "Deploy preview", "perch", "/Users/dev/perch", "codex", agent.Error, "", "2026-11-20T10:02:00Z", "2026-11-20T10:02:00Z"),
		a(7, "Pinned notes", "perch", "/Users/dev/perch", "codex", agent.Idle, "", "2026-11-20T10:02:00Z", "2026-11-20T10:02:00Z"),
		a(11, "Fix the login bug fork", "t3code", "/Users/dev/t3code", "codex", agent.Idle, "", "2026-11-20T09:58:00Z", "2026-11-20T09:00:00Z"),
		a(13, "Plan the auth rewrite", "t3code", "/Users/dev/t3code", "codex", agent.Done, "", "2026-11-20T10:02:00Z", "2026-11-20T10:02:00Z"),
		a(14, "Shared with a guest", "t3code", "/Users/dev/t3code", "codex", agent.Blocked, agent.WhyRequest, "2026-11-20T10:02:00Z", "2026-11-20T10:02:00Z"),
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
		State: agent.Blocked, BlockedWhy: agent.WhyPermission, Since: mustTime("2026-11-20T10:04:00Z"), SortAt: mustTime("2026-11-20T10:02:00Z")}
	expectEvents(t, got, agent.Upsert{Agent: want})
}

func TestSnoozeKeepsAnOldQuestion(t *testing.T) {
	tr := newFixtureTracker(t)
	expectEvents(t, tr.apply(threadUpdated(t, 121, "active", fixtureThread(t, 8,
		"pendingRuntimeRequest", map[string]any{"id": "req-1", "kind": "user_input", "createdAt": "2026-11-20T10:00:00.000Z"})), testNow))
}

func TestSnoozeWakesAtItsDeadlineAsWokeAndClearsOnVisit(t *testing.T) {
	tr := newFixtureTracker(t)
	wake := mustTime("2026-11-21T09:00:00Z")
	if d, ok := tr.deadline(); !ok || !d.Equal(wake) {
		t.Fatalf("deadline = %v %v, want %v", d, ok, wake)
	}
	want := agent.Agent{ID: id(8), Info: agent.Info{Project: "perch", Title: "Snoozed until tomorrow", Where: "/Users/dev/perch", Kind: "codex"},
		State: agent.Blocked, BlockedWhy: agent.WhyWoke, Since: wake, SortAt: mustTime("2026-11-20T10:02:00Z")}
	expectEvents(t, tr.expire(wake), agent.Upsert{Agent: want})
	if _, ok := tr.deadline(); ok {
		t.Fatal("deadline still set after the snooze ended")
	}

	// Visiting after the wake clears Woke to Idle.
	visited := fixtureThread(t, 8, "lastVisitedAt", iso(wake.Add(time.Minute)))
	want.State, want.BlockedWhy, want.Since = agent.Idle, "", mustTime("2026-11-20T10:02:00Z")
	expectEvents(t, tr.apply(threadUpdated(t, 121, "active", visited), wake.Add(time.Minute)), agent.Upsert{Agent: want})
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
	// During the 2s exit grace, the thread stays Working with its original activity start.
	expectEvents(t, tr.apply(threadUpdated(t, 121, "active", stopped(t, 1, testNow)), testNow))
	d, ok := tr.deadline()
	if want := testNow.Add(2 * time.Second); !ok || !d.Equal(want) {
		t.Fatalf("deadline = %v %v, want %v", d, ok, want)
	}
	expectEvents(t, tr.expire(testNow.Add(time.Second)))

	exitAt := testNow.Add(2 * time.Second)
	want := fixtureAgent(1)
	want.State, want.Since, want.SortAt = agent.Done, testNow, exitAt
	expectEvents(t, tr.expire(exitAt), agent.Upsert{Agent: want})
	if d, _ := tr.deadline(); d.Before(testNow.Add(time.Hour)) {
		t.Fatalf("grace deadline still armed: %v", d)
	}
}

func TestRestartWithinTheGraceStaysWorking(t *testing.T) {
	tr := newFixtureTracker(t)
	expectEvents(t, tr.apply(threadUpdated(t, 121, "active", stopped(t, 1, testNow)), testNow))
	later := testNow.Add(time.Second)
	restarted := fixtureThread(t, 1, "latestRunId", "run-1b", "activeRunId", "run-1b",
		"latestRunRequestedAt", iso(later), "latestRunStartedAt", iso(later),
		"activityRunStartedAt", iso(later), "latestUserAuthoredMessageAt", iso(later), "updatedAt", iso(later))
	got := tr.apply(threadUpdated(t, 122, "active", restarted), later)
	want := fixtureAgent(1)
	want.Since, want.SortAt = later, later
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
		sortAt time.Time
	}{
		"blocked": {stopped(t, 1, testNow, "pendingRuntimeRequest", map[string]any{"id": "r", "kind": "permission", "createdAt": iso(testNow)}), agent.Blocked, agent.WhyPermission, testNow, testNow},
		"error":   {stopped(t, 1, testNow, "status", "failed"), agent.Error, "", testNow, testNow},
		"stale":   {stopped(t, 1, testNow.Add(-time.Minute)), agent.Done, "", testNow.Add(-time.Minute), testNow},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			tr := newFixtureTracker(t)
			want := fixtureAgent(1)
			want.State, want.BlockedWhy, want.Since, want.SortAt = c.state, c.why, c.since, c.sortAt
			expectEvents(t, tr.apply(threadUpdated(t, 121, "active", c.thread), testNow), agent.Upsert{Agent: want})
		})
	}
}

func TestMovedThreadSkipsTheGrace(t *testing.T) {
	tr := newFixtureTracker(t)
	want := fixtureAgent(1)
	want.State, want.Since, want.SortAt = agent.Done, testNow, testNow
	expectEvents(t, tr.apply(threadUpdated(t, 121, "active", stopped(t, 1, testNow, "pinnedAt", iso(testNow))), testNow), agent.Upsert{Agent: want})
}

func TestWaitingStates(t *testing.T) {
	t.Run("background work shows Waiting from completedAt", func(t *testing.T) {
		tr := newFixtureTracker(t)
		doneAt := testNow.Add(-10 * time.Minute)
		th := stopped(t, 3, doneAt, "pendingBackgroundTasks", []any{map[string]any{"taskId": "t1", "kind": "subagent"}})
		want := fixtureAgent(3)
		want.State, want.WorkingKind, want.Since = agent.Working, agent.WorkWaiting, doneAt
		expectEvents(t, tr.apply(threadUpdated(t, 121, "active", th), testNow), agent.Upsert{Agent: want})
	})
	t.Run("blockedOnThreadWait shows Waiting with no duration", func(t *testing.T) {
		tr := newFixtureTracker(t)
		th := fixtureThread(t, 1, "blockedOnThreadWait", true)
		want := fixtureAgent(1)
		want.State, want.WorkingKind, want.Since = agent.Working, agent.WorkWaiting, time.Time{}
		expectEvents(t, tr.apply(threadUpdated(t, 121, "active", th), testNow), agent.Upsert{Agent: want})
	})
	t.Run("preparing shows Setting up and active goal shows Goal", func(t *testing.T) {
		tr := newFixtureTracker(t)
		th := fixtureThread(t, 1, "activityRunStatus", "preparing", "status", "preparing")
		want := fixtureAgent(1)
		want.WorkingKind = agent.WorkPreparing
		expectEvents(t, tr.apply(threadUpdated(t, 121, "active", th), testNow), agent.Upsert{Agent: want})

		th2 := fixtureThread(t, 1, "goal", map[string]any{"status": "active"})
		want.WorkingKind = agent.WorkGoal
		expectEvents(t, tr.apply(threadUpdated(t, 122, "active", th2), testNow), agent.Upsert{Agent: want})
	})
}

func TestFailureVisitAndUsageLimit(t *testing.T) {
	t.Run("visited failure settles to Idle", func(t *testing.T) {
		tr := newFixtureTracker(t)
		th := fixtureThread(t, 6, "lastVisitedAt", "2026-11-20T10:03:00.000Z")
		want := fixtureAgent(6)
		want.State = agent.Idle
		expectEvents(t, tr.apply(threadUpdated(t, 121, "active", th), testNow), agent.Upsert{Agent: want})
	})
	t.Run("never-visited failure stays Error", func(t *testing.T) {
		tr := newFixtureTracker(t)
		th := fixtureThread(t, 6, "lastVisitedAt", nil)
		expectEvents(t, tr.apply(threadUpdated(t, 121, "active", th), testNow))
	})
	t.Run("stale lastError on a running thread stays Working", func(t *testing.T) {
		tr := newFixtureTracker(t)
		th := fixtureThread(t, 1, "lastError", "old error")
		expectEvents(t, tr.apply(threadUpdated(t, 121, "active", th), testNow))
	})
	t.Run("usage_limit failure shows Limited and stays after visit", func(t *testing.T) {
		tr := newFixtureTracker(t)
		th := fixtureThread(t, 6, "lastErrorClass", "usage_limit", "lastVisitedAt", "2026-11-20T10:03:00.000Z")
		want := fixtureAgent(6)
		want.State, want.BlockedWhy = agent.Blocked, agent.WhyLimited
		expectEvents(t, tr.apply(threadUpdated(t, 121, "active", th), testNow), agent.Upsert{Agent: want})
	})
}

func TestPlanActiveRunIsWorking(t *testing.T) {
	tr := newFixtureTracker(t)
	th := fixtureThread(t, 13,
		"status", "running", "activeRunId", "run-13b", "latestRunId", "run-13b",
		"latestRunCompletedAt", nil, "activityRunStatus", "running",
		"activityRunStartedAt", "2026-11-20T10:04:00.000Z", "latestUserAuthoredMessageAt", "2026-11-20T10:04:00.000Z")
	want := fixtureAgent(13)
	want.State, want.Since, want.SortAt = agent.Working, mustTime("2026-11-20T10:04:00Z"), mustTime("2026-11-20T10:04:00Z")
	expectEvents(t, tr.apply(threadUpdated(t, 121, "active", th), testNow), agent.Upsert{Agent: want})
}

func TestSubagentRollupMatchesT3(t *testing.T) {
	sub := func(idStr, parentID, src string, kv ...any) map[string]any {
		base := fixtureThread(t, 10,
			"id", idStr,
			"creationSource", src,
			"lineage", map[string]any{"parentThreadId": parentID, "relationshipToParent": "subagent", "rootThreadId": parentID},
		)
		for i := 0; i < len(kv); i += 2 {
			base[kv[i].(string)] = kv[i+1]
		}
		return base
	}
	t.Run("delegated subagent approval blocks parent and wakes snoozed parent", func(t *testing.T) {
		tr := newFixtureTracker(t)
		child := sub("sub-1", tid(8), "mcp")
		got := tr.apply(threadUpdated(t, 121, "active", child), testNow)
		want := agent.Agent{
			ID: id(8), Info: agent.Info{Project: "perch", Title: "Snoozed until tomorrow", Where: "/Users/dev/perch", Kind: "codex"},
			State: agent.Blocked, BlockedWhy: agent.WhyPermission,
			Since: mustTime("2026-11-20T10:03:00Z"), SortAt: mustTime("2026-11-20T10:02:00Z"),
		}
		expectEvents(t, got, agent.Upsert{Agent: want})
	})
	t.Run("delegated subagent normal user_input does not block parent, userOnly does", func(t *testing.T) {
		tr := newFixtureTracker(t)
		normalQ := sub("sub-q", tid(3), "mcp",
			"pendingRuntimeRequest", map[string]any{"id": "rq", "kind": "user_input", "createdAt": "2026-11-20T10:04:00.000Z", "userOnly": false})
		expectEvents(t, tr.apply(threadUpdated(t, 121, "active", normalQ), testNow))

		userOnlyQ := sub("sub-q", tid(3), "mcp",
			"pendingRuntimeRequest", map[string]any{"id": "rq", "kind": "user_input", "createdAt": "2026-11-20T10:04:00.000Z", "userOnly": true})
		want := fixtureAgent(3)
		want.State, want.BlockedWhy, want.Since = agent.Blocked, agent.WhyQuestion, mustTime("2026-11-20T10:04:00Z")
		expectEvents(t, tr.apply(threadUpdated(t, 122, "active", userOnlyQ), testNow), agent.Upsert{Agent: want})
	})
	t.Run("failed or non-live subagent does not affect parent", func(t *testing.T) {
		tr := newFixtureTracker(t)
		failedChild := sub("sub-f", tid(3), "mcp",
			"status", "failed", "activeRunId", nil, "activityRunStatus", nil, "pendingRuntimeRequest", nil, "lastError", "boom")
		expectEvents(t, tr.apply(threadUpdated(t, 121, "active", failedChild), testNow))

		endedReq := sub("sub-e", tid(3), "mcp",
			"status", "completed", "activeRunId", nil, "activityRunStatus", nil,
			"pendingRuntimeRequest", map[string]any{"id": "rq", "kind": "command", "createdAt": "2026-11-20T10:04:00.000Z"})
		expectEvents(t, tr.apply(threadUpdated(t, 122, "active", endedReq), testNow))
	})
}

func TestSidechatIsHiddenAndRollsUpRequests(t *testing.T) {
	side := func(idStr, parentID string, kv ...any) map[string]any {
		base := fixtureThread(t, 1,
			"id", idStr,
			"title", "Side question",
			"lineage", map[string]any{"parentThreadId": parentID, "relationshipToParent": "fork", "rootThreadId": parentID, "sideChat": true},
		)
		for i := 0; i < len(kv); i += 2 {
			base[kv[i].(string)] = kv[i+1]
		}
		return base
	}

	t.Run("running and completed sidechats do not emit rows or change parent state", func(t *testing.T) {
		tr := newFixtureTracker(t)
		running := side("side-1", tid(3), "activityRunStartedAt", "2026-11-20T10:04:00.000Z")
		expectEvents(t, tr.apply(threadUpdated(t, 121, "active", running), testNow))

		done := side("side-1", tid(3),
			"status", "completed", "activeRunId", nil, "activityRunStatus", nil, "activityRunStartedAt", nil,
			"latestRunCompletedAt", "2026-11-20T10:04:30.000Z", "lastVisitedAt", "2026-11-20T10:03:00.000Z")
		expectEvents(t, tr.apply(threadUpdated(t, 122, "active", done), testNow))
	})

	t.Run("sidechat user_input and approval roll up to parent", func(t *testing.T) {
		tr := newFixtureTracker(t)
		q := side("side-q", tid(3),
			"pendingRuntimeRequest", map[string]any{"id": "rq", "kind": "user_input", "createdAt": "2026-11-20T10:04:00.000Z"})
		want := fixtureAgent(3)
		want.State, want.BlockedWhy, want.Since = agent.Blocked, agent.WhyQuestion, mustTime("2026-11-20T10:04:00Z")
		expectEvents(t, tr.apply(threadUpdated(t, 121, "active", q), testNow), agent.Upsert{Agent: want})

		ap := side("side-q", tid(3),
			"pendingRuntimeRequest", map[string]any{"id": "ra", "kind": "command", "createdAt": "2026-11-20T10:04:30.000Z"})
		want.BlockedWhy, want.Since = agent.WhyPermission, mustTime("2026-11-20T10:04:30Z")
		expectEvents(t, tr.apply(threadUpdated(t, 122, "active", ap), testNow), agent.Upsert{Agent: want})
	})

	t.Run("subagent approval under a sidechat walks through the sidechat to the listed parent", func(t *testing.T) {
		tr := newFixtureTracker(t)
		idleSide := side("side-4", tid(3),
			"status", "completed", "activeRunId", nil, "activityRunStatus", nil, "activityRunStartedAt", nil,
			"latestRunCompletedAt", "2026-11-20T10:02:00.000Z", "lastVisitedAt", "2026-11-20T10:03:00.000Z")
		expectEvents(t, tr.apply(threadUpdated(t, 121, "active", idleSide), testNow))

		sub := fixtureThread(t, 10,
			"id", "sub-under-side",
			"creationSource", "mcp",
			"lineage", map[string]any{"parentThreadId": "side-4", "relationshipToParent": "subagent", "rootThreadId": tid(3)},
		)
		want := fixtureAgent(3)
		want.State, want.BlockedWhy, want.Since = agent.Blocked, agent.WhyPermission, mustTime("2026-11-20T10:03:00Z")
		expectEvents(t, tr.apply(threadUpdated(t, 122, "active", sub), testNow), agent.Upsert{Agent: want})
	})
}

func TestSortTimestampsMatchT3(t *testing.T) {
	tr := newFixtureTracker(t)
	// Un-settling an idle thread bumps SortAt to unsettledAt without changing display Since.
	unsettled := fixtureThread(t, 3, "unsettledAt", "2026-11-20T10:04:30.000Z")
	want := fixtureAgent(3)
	want.SortAt = mustTime("2026-11-20T10:04:30Z")
	expectEvents(t, tr.apply(threadUpdated(t, 121, "active", unsettled), testNow), agent.Upsert{Agent: want})
}

func TestParentRemovalLeavesNoOrphanChild(t *testing.T) {
	tr := newFixtureTracker(t)
	expectEvents(t, tr.apply(threadUpdated(t, 121, "active", fixtureThread(t, 1, "settledOverride", "settled")), testNow),
		agent.Remove{ID: id(1)})
}
