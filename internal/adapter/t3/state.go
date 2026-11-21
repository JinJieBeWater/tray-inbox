package t3

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
)

const (
	orchestrator = "t3"
	localSource  = "local"

	// WORKING_EXIT_GRACE_MS and STALE_EXIT_MS in T3's packages/client-runtime/src/state/threadInbox.ts.
	workingExitGrace = 2 * time.Second
	staleExit        = 60 * time.Second
)

// run and runtime are T3's ThreadRunSummary and ThreadRuntimeSummary as presentThreadShell derives them (models.ts).
type run struct {
	id, status  string
	requestedAt *time.Time
	completedAt *time.Time
}

type runtime struct {
	status      string
	activeRunID *string
	lastError   *string
	updatedAt   time.Time
}

func latestRun(th *shellThread) *run {
	if th.LatestRunID == nil {
		return nil
	}
	r := &run{id: *th.LatestRunID, status: th.Status, requestedAt: th.LatestRunRequestedAt, completedAt: th.LatestRunCompletedAt.T}
	if th.Status == "idle" {
		r.status = "completed"
	}
	if !th.LatestRunCompletedAt.Set && (th.Status == "idle" || terminalStatus(th.Status)) {
		r.completedAt = &th.UpdatedAt
	}
	return r
}

func shellRuntime(th *shellThread) *runtime {
	parkAtIdle := backgroundWorkHoldsCompletion(th) && th.Status != "failed"
	if th.LatestRunID == nil && !parkAtIdle {
		return nil
	}
	rt := &runtime{status: th.Status, activeRunID: th.ActiveRunID, lastError: th.LastError, updatedAt: th.UpdatedAt}
	switch {
	case parkAtIdle:
		rt.status = "idle"
	case th.ActivityRunStatus != nil:
		rt.status = *th.ActivityRunStatus
	}
	return rt
}

// backgroundWorkHoldsCompletion is packages/shared/src/orchestrationV2PendingBackgroundWork.ts.
func backgroundWorkHoldsCompletion(th *shellThread) bool {
	for _, task := range th.PendingBackgroundTasks {
		if task.Awaited || task.Kind == "subagent" || task.Kind == "background_task" {
			return true
		}
	}
	return false
}

func activeStatus(s string) bool {
	switch s {
	case "preparing", "queued", "starting", "running", "waiting":
		return true
	}
	return false
}

func terminalStatus(s string) bool {
	switch s {
	case "completed", "interrupted", "failed", "cancelled", "rolled_back":
		return true
	}
	return false
}

func hasPendingApprovals(th *shellThread) bool {
	r := th.PendingRuntimeRequest
	return r != nil && r.Kind != "user_input" && r.Kind != "auth_refresh"
}

func hasPendingUserInput(th *shellThread) bool {
	return th.PendingRuntimeRequest != nil && th.PendingRuntimeRequest.Kind == "user_input"
}

func planWaiting(th *shellThread) bool {
	return th.InteractionMode == "plan" && th.HasActionableProposedPlan
}

func failed(th *shellThread) bool {
	if rt := shellRuntime(th); rt != nil && rt.lastError != nil {
		return true
	}
	r := latestRun(th)
	return r != nil && r.status == "failed"
}

// isThreadWorking is threadInbox.ts's; TrayInbox does not compute pendingSubagentRequest, since the child reports itself.
func isThreadWorking(th *shellThread) bool {
	if hasPendingApprovals(th) || hasPendingUserInput(th) {
		return false
	}
	rt := shellRuntime(th)
	if rt == nil || (!activeStatus(rt.status) && rt.status != "idle") {
		return false
	}
	r := latestRun(th)
	runSettled := r != nil && !activeStatus(r.status) && (rt.activeRunID == nil || *rt.activeRunID != r.id)
	return !(planWaiting(th) && runSettled)
}

// effectiveSnoozed and raisedHandWhileSnoozed are threadSettled.ts's.
func effectiveSnoozed(th *shellThread, now time.Time) bool {
	if th.SnoozedUntil == nil || !th.SnoozedUntil.After(now) {
		return false
	}
	return !raisedHandWhileSnoozed(th)
}

func raisedHandWhileSnoozed(th *shellThread) bool {
	req := th.PendingRuntimeRequest
	if hasPendingApprovals(th) || (req != nil && req.Kind != "user_input") {
		return true
	}
	if hasPendingUserInput(th) && (th.SnoozedAt == nil || req.CreatedAt.After(*th.SnoozedAt)) {
		return true
	}
	if th.PendingGuestMessageCount > 0 {
		return true
	}
	failedAt, ok := failureEdgeAt(th)
	return ok && (th.SnoozedAt == nil || (failedAt != nil && failedAt.After(*th.SnoozedAt)))
}

func failureEdgeAt(th *shellThread) (*time.Time, bool) {
	rt := shellRuntime(th)
	if rt == nil || (rt.status != "error" && rt.status != "failed") {
		return nil, false
	}
	if r := latestRun(th); r != nil && r.status == "failed" {
		return r.completedAt, true
	}
	return &rt.updatedAt, true
}

func excluded(th *shellThread, now time.Time) bool {
	return th.ArchivedAt != nil || th.DeletedAt != nil ||
		(th.SettledOverride != nil && *th.SettledOverride == "settled") ||
		effectiveSnoozed(th, now)
}

// unseenCompletion is hasUnseenCompletion in T3's web and mobile clients: a thread never visited has no unseen completion.
func unseenCompletion(th *shellThread) bool {
	r := latestRun(th)
	if r == nil || r.completedAt == nil || r.status == "interrupted" || r.status == "cancelled" || th.LastVisitedAt == nil {
		return false
	}
	return r.completedAt.After(*th.LastVisitedAt)
}

func placementKey(th *shellThread) string {
	ts := func(t *time.Time) string {
		if t == nil {
			return "-"
		}
		return t.UTC().Format(time.RFC3339Nano)
	}
	s := func(p *string) string {
		if p == nil {
			return "-"
		}
		return fmt.Sprintf("%q", *p)
	}
	return fmt.Sprint(ts(th.ArchivedAt), ts(th.DeletedAt), ts(th.PinnedAt), s(th.SettledOverride), ts(th.SettledAt),
		ts(th.SnoozedAt), ts(th.SnoozedUntil), s(th.ActiveOrderKey))
}

type graceState struct {
	working   bool
	deadline  time.Time // zero: none
	placement string    // empty: not tracked
}

// tracker holds one connection's shell and turns it into agents. It is not safe for concurrent use.
type tracker struct {
	env      string
	kinds    map[string]string // provider instance id → driver
	synced   bool
	seq      int64
	last     time.Time
	threads  map[string]*shellThread
	projects map[string]shellProject
	grace    map[string]graceState
	shown    map[string]agent.Agent
}

func newTracker(env string, kinds map[string]string) *tracker {
	return &tracker{env: env, kinds: kinds, threads: map[string]*shellThread{}, projects: map[string]shellProject{},
		grace: map[string]graceState{}, shown: map[string]agent.Agent{}}
}

func (t *tracker) apply(it item, now time.Time) []agent.Event {
	t.last = now
	if it.Kind == "snapshot" {
		if it.Patch {
			return nil
		}
		t.threads = map[string]*shellThread{}
		for i := range it.Snapshot.Threads {
			th := &it.Snapshot.Threads[i]
			t.threads[th.ID] = th
		}
		t.projects = map[string]shellProject{}
		for _, p := range it.Snapshot.Projects {
			t.projects[p.ID] = p
		}
		t.seq = it.Snapshot.SnapshotSequence
		first := !t.synced
		t.synced = true
		t.observe(now)
		agents := t.agents(now)
		list := sortedAgents(agents)
		if !first && maps.Equal(t.shown, agents) {
			return nil
		}
		t.shown = agents
		return []agent.Event{agent.Snapshot{Orchestrator: orchestrator, Source: localSource, Agents: list}}
	}
	if !t.synced || it.Sequence <= t.seq {
		return nil
	}
	switch it.Kind {
	case "thread.updated":
		if it.Location == "active" {
			t.threads[it.Thread.ID] = it.Thread
		} else {
			delete(t.threads, it.Thread.ID)
		}
	case "thread.removed":
		delete(t.threads, it.ThreadID)
	case "project.updated":
		t.projects[it.Project.ID] = *it.Project
	case "project.removed":
		delete(t.projects, it.ProjectID)
	default:
		return nil
	}
	t.seq = it.Sequence
	t.observe(now)
	return t.diff(now)
}

// expire ends the graces that are due and wakes threads whose snooze ended.
func (t *tracker) expire(now time.Time) []agent.Event {
	t.last = now
	for id, g := range t.grace {
		if !g.deadline.IsZero() && !g.deadline.After(now) {
			g.working, g.deadline = false, time.Time{}
			t.grace[id] = g
		}
	}
	return t.diff(now)
}

// deadline is when expire next has something to do.
func (t *tracker) deadline() (time.Time, bool) {
	var next time.Time
	earlier := func(d time.Time) {
		if next.IsZero() || d.Before(next) {
			next = d
		}
	}
	for _, g := range t.grace {
		if !g.deadline.IsZero() {
			earlier(g.deadline)
		}
	}
	for _, th := range t.threads {
		if th.SnoozedUntil != nil && th.SnoozedUntil.After(t.last) {
			earlier(*th.SnoozedUntil)
		}
	}
	return next, !next.IsZero()
}

// observe is the per-thread step of createInboxReturnTracker().observe in threadInbox.ts. Unlike T3, a pinned
// thread still gets the grace, since TrayInbox lists it like any other.
func (t *tracker) observe(now time.Time) {
	for id, th := range t.threads {
		prev, had := t.grace[id]
		actual := isThreadWorking(th)
		placement := ""
		if actual || prev.working {
			placement = placementKey(th)
		}
		moved := had && prev.placement != placement
		actionable := hasPendingApprovals(th) || hasPendingUserInput(th) || th.PendingGuestMessageCount > 0 ||
			planWaiting(th) || failed(th)
		r := latestRun(th)
		endedLongAgo := r != nil && r.completedAt != nil && !r.completedAt.After(now.Add(-staleExit))
		var deadline time.Time
		if !actual && prev.working && !actionable && !moved && !excluded(th, now) && !endedLongAgo {
			deadline = prev.deadline
			if deadline.IsZero() {
				deadline = now.Add(workingExitGrace)
			}
		}
		working := actual || deadline.After(now)
		if !working {
			deadline = time.Time{}
		}
		t.grace[id] = graceState{working: working, deadline: deadline, placement: placement}
	}
	for id := range t.grace {
		if _, ok := t.threads[id]; !ok {
			delete(t.grace, id)
		}
	}
}

func (t *tracker) agents(now time.Time) map[string]agent.Agent {
	out := map[string]agent.Agent{}
	for _, th := range t.threads {
		if !t.listed(th, now) {
			continue
		}
		a := t.agent(th)
		out[a.ID.Native] = a
	}
	return out
}

// listed is T3's sidebar rule: a sub-agent or side chat has no row of its own and shows only under a listed parent.
func (t *tracker) listed(th *shellThread, now time.Time) bool {
	for range len(t.threads) {
		if excluded(th, now) {
			return false
		}
		parent := childOf(th)
		if parent == "" {
			return true
		}
		if th = t.threads[parent]; th == nil {
			return false
		}
	}
	return false
}

// childOf is the parent of a sub-agent or side chat. A fork shares parentThreadId but is a thread of its own.
func childOf(th *shellThread) string {
	if l := th.Lineage; l.ParentThreadID != nil && (l.SideChat || (l.RelationshipToParent != nil && *l.RelationshipToParent == "subagent")) {
		return *l.ParentThreadID
	}
	return ""
}

func (t *tracker) native(threadID string) string { return t.env + "/" + threadID }

func (t *tracker) agent(th *shellThread) agent.Agent {
	a := agent.Agent{
		ID:    agent.ID{Orchestrator: orchestrator, Source: localSource, Native: t.native(th.ID)},
		Info:  agent.Info{Title: th.Title, Kind: t.kind(th)},
		Since: th.UpdatedAt,
	}
	p, hasProject := t.projects[th.ProjectID]
	a.Project = p.Title
	switch {
	case th.Branch != nil:
		a.Where = *th.Branch
	case th.WorktreePath != nil:
		a.Where = *th.WorktreePath
	case hasProject:
		a.Where = p.WorkspaceRoot
	}
	if r := latestRun(th); r != nil {
		switch {
		case r.completedAt != nil:
			a.Since = *r.completedAt
		case r.requestedAt != nil:
			a.Since = *r.requestedAt
		}
	}
	if parent := childOf(th); parent != "" {
		a.Parent = t.native(parent)
	}
	a.State, a.BlockedWhy = t.state(th)
	if r := th.PendingRuntimeRequest; a.State == agent.Blocked && r != nil && !r.CreatedAt.IsZero() {
		a.Since = r.CreatedAt
	}
	return a
}

func (t *tracker) state(th *shellThread) (agent.State, agent.BlockedWhy) {
	switch {
	case hasPendingApprovals(th):
		return agent.Blocked, agent.WhyPermission
	case hasPendingUserInput(th):
		return agent.Blocked, agent.WhyQuestion
	case planWaiting(th):
		return agent.Blocked, agent.WhyPermission
	case th.PendingGuestMessageCount > 0:
		return agent.Blocked, agent.WhyOther
	case failed(th):
		return agent.Error, ""
	case t.grace[th.ID].working:
		return agent.Working, ""
	case unseenCompletion(th):
		return agent.Done, ""
	}
	return agent.Idle, ""
}

func (t *tracker) kind(th *shellThread) agent.Kind {
	instance := th.ModelSelection.InstanceID
	if instance == "" {
		instance = th.ModelSelection.Provider
	}
	if shellRuntime(th) != nil {
		instance = th.ProviderInstanceID
	}
	kind := instance
	if driver, ok := t.kinds[instance]; ok {
		kind = driver
	}
	if kind == "claudeAgent" {
		return agent.KindClaude
	}
	return agent.Kind(kind)
}

func (t *tracker) diff(now time.Time) []agent.Event {
	next := t.agents(now)
	var evs []agent.Event
	for _, a := range sortedAgents(next) {
		if prev, ok := t.shown[a.ID.Native]; !ok || prev != a {
			evs = append(evs, agent.Upsert{Agent: a})
		}
	}
	var gone []string
	for id := range t.shown {
		if _, ok := next[id]; !ok {
			gone = append(gone, id)
		}
	}
	sort.Strings(gone)
	for _, id := range gone {
		evs = append(evs, agent.Remove{ID: agent.ID{Orchestrator: orchestrator, Source: localSource, Native: id}})
	}
	t.shown = next
	return evs
}

func sortedAgents(m map[string]agent.Agent) []agent.Agent {
	list := make([]agent.Agent, 0, len(m))
	for _, a := range m {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID.Native < list[j].ID.Native })
	return list
}
