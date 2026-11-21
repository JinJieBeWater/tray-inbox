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
	startedAt   *time.Time
	completedAt *time.Time
}

type runtime struct {
	status         string
	activeRunID    *string
	activityStart  optTime
	lastError      *string
	lastErrorClass *string
	updatedAt      time.Time
}

func latestRun(th *shellThread) *run {
	if th.LatestRunID == nil {
		return nil
	}
	r := &run{
		id:          *th.LatestRunID,
		status:      th.Status,
		requestedAt: th.LatestRunRequestedAt,
		startedAt:   th.LatestRunStartedAt,
		completedAt: th.LatestRunCompletedAt.T,
	}
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
	rt := &runtime{
		status:         th.Status,
		activeRunID:    th.ActiveRunID,
		activityStart:  th.ActivityRunStartedAt,
		lastError:      th.LastError,
		lastErrorClass: th.LastErrorClass,
		updatedAt:      th.UpdatedAt,
	}
	switch {
	case parkAtIdle:
		rt.status = "idle"
	case th.ActivityRunStatus.S != nil:
		rt.status = *th.ActivityRunStatus.S
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

func isListedLineage(th *shellThread) bool {
	l := th.Lineage
	return !l.SideChat && (l.RelationshipToParent == nil || *l.RelationshipToParent != "subagent")
}

func isSubagent(th *shellThread) bool {
	return th.Lineage.RelationshipToParent != nil && *th.Lineage.RelationshipToParent == "subagent"
}

func isProviderNativeSubagent(th *shellThread) bool {
	return isSubagent(th) && th.CreationSource == "provider"
}

func isChildRunLive(th *shellThread) bool {
	if th.ActivityRunStatus.Set {
		return th.ActivityRunStatus.S != nil
	}
	return th.Status == "preparing" || th.Status == "starting" || th.Status == "running" || th.Status == "waiting"
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

func runSettled(th *shellThread, rt *runtime) bool {
	r := latestRun(th)
	return r != nil && !activeStatus(r.status) && (rt == nil || rt.activeRunID == nil || *rt.activeRunID != r.id)
}

func hasActionableFailure(th *shellThread) bool {
	if rt := shellRuntime(th); rt != nil && rt.lastError != nil {
		return true
	}
	r := latestRun(th)
	return r != nil && r.status == "failed"
}

// subagentReq is T3's rolled-up pendingSubagentRequest (threadShell.ts).
type subagentReq struct {
	kind      string // "approval" or "user_input"
	createdAt time.Time
}

func pendingSubagentRequests(threads map[string]*shellThread) map[string]subagentReq {
	reqs := map[string]subagentReq{}
	set := func(parentID string, req subagentReq) {
		prev, ok := reqs[parentID]
		if !ok || (prev.kind != "approval" && req.kind == "approval") || (prev.kind == req.kind && req.createdAt.After(prev.createdAt)) {
			reqs[parentID] = req
		}
	}
	flag := func(firstParent string, req subagentReq, selfID string) {
		seen := map[string]bool{selfID: true}
		for parentID := firstParent; parentID != "" && !seen[parentID]; {
			seen[parentID] = true
			set(parentID, req)
			p := threads[parentID]
			if p != nil && p.Lineage.ParentThreadID != nil && (isSubagent(p) || p.Lineage.SideChat) {
				parentID = *p.Lineage.ParentThreadID
			} else {
				break
			}
		}
	}
	for _, child := range threads {
		if child.DeletedAt != nil || child.ArchivedAt != nil || child.Lineage.ParentThreadID == nil {
			continue
		}
		parentID := *child.Lineage.ParentThreadID
		r := child.PendingRuntimeRequest
		if child.Lineage.SideChat {
			// A side chat asks the user directly, so both approvals and questions roll up (threadShell.ts).
			if r != nil && r.Kind != "auth_refresh" {
				kind := "approval"
				if r.Kind == "user_input" {
					kind = "user_input"
				}
				set(parentID, subagentReq{kind: kind, createdAt: r.CreatedAt})
			}
			continue
		}
		if !isSubagent(child) || isProviderNativeSubagent(child) || !isChildRunLive(child) {
			continue
		}
		if r == nil || r.Kind == "auth_refresh" {
			continue
		}
		// The coordinator answers child questions; approvals and user-only cards need the user (threadShell.ts).
		if r.Kind == "user_input" {
			if !r.UserOnly {
				continue
			}
			flag(parentID, subagentReq{kind: "user_input", createdAt: r.CreatedAt}, child.ID)
		} else {
			flag(parentID, subagentReq{kind: "approval", createdAt: r.CreatedAt}, child.ID)
		}
	}
	return reqs
}

func isThreadWorking(th *shellThread, subReq subagentReq) bool {
	if hasPendingApprovals(th) || hasPendingUserInput(th) || subReq.kind != "" {
		return false
	}
	rt := shellRuntime(th)
	if rt == nil || (!activeStatus(rt.status) && rt.status != "idle") {
		return false
	}
	return !(planWaiting(th) && runSettled(th, rt))
}

func workingKindOf(th *shellThread) agent.WorkingKind {
	rt := shellRuntime(th)
	if rt == nil {
		return agent.WorkRunning
	}
	if rt.status == "running" && th.BlockedOnThreadWait {
		return agent.WorkWaiting
	}
	if rt.status == "preparing" {
		return agent.WorkPreparing
	}
	if rt.status == "idle" {
		return agent.WorkWaiting
	}
	if th.Goal != nil && th.Goal.Status == "active" {
		return agent.WorkGoal
	}
	return agent.WorkRunning
}

// resolveWorkingStartedAt is resolveThreadWorkingStartedAt in models.ts.
func resolveWorkingStartedAt(th *shellThread) time.Time {
	rt := shellRuntime(th)
	if rt != nil && rt.activityStart.Set {
		if rt.activityStart.T != nil {
			return *rt.activityStart.T
		}
		return time.Time{}
	}
	r := latestRun(th)
	if r != nil && r.completedAt == nil && rt != nil && rt.activeRunID != nil && *rt.activeRunID == r.id {
		if r.startedAt != nil {
			return *r.startedAt
		}
		if r.requestedAt != nil {
			return *r.requestedAt
		}
	}
	return time.Time{}
}

func workingSince(th *shellThread, kind agent.WorkingKind) time.Time {
	if kind == agent.WorkWaiting {
		if r := latestRun(th); r != nil && r.completedAt != nil {
			return *r.completedAt
		}
		return time.Time{}
	}
	return resolveWorkingStartedAt(th)
}

// effectiveSnoozed, raisedHandWhileSnoozed, threadWokeAt, and isThreadWokeUnseen are threadSettled.ts's.
func effectiveSnoozed(th *shellThread, subReq subagentReq, now time.Time) bool {
	if th.SnoozedUntil == nil || !th.SnoozedUntil.After(now) {
		return false
	}
	return !raisedHandWhileSnoozed(th, subReq)
}

func raisedHandWhileSnoozed(th *shellThread, subReq subagentReq) bool {
	req := th.PendingRuntimeRequest
	if hasPendingApprovals(th) || (req != nil && req.Kind != "user_input") {
		return true
	}
	if hasPendingUserInput(th) && (th.SnoozedAt == nil || req.CreatedAt.After(*th.SnoozedAt)) {
		return true
	}
	if subReq.kind != "" {
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

func threadWokeAt(th *shellThread, subReq subagentReq, now time.Time) *time.Time {
	if th.SnoozedUntil == nil {
		return nil
	}
	if raisedHandWhileSnoozed(th, subReq) {
		failedAt, _ := failureEdgeAt(th)
		var questionAt *time.Time
		if hasPendingUserInput(th) {
			t := th.PendingRuntimeRequest.CreatedAt
			questionAt = &t
		}
		freshFailure := failedAt != nil && (th.SnoozedAt == nil || failedAt.After(*th.SnoozedAt))
		freshQuestion := questionAt != nil && th.SnoozedAt != nil && questionAt.After(*th.SnoozedAt)
		if freshFailure && (!freshQuestion || !failedAt.Before(*questionAt)) {
			return failedAt
		}
		if freshQuestion {
			return questionAt
		}
		if rt := shellRuntime(th); rt != nil {
			t := rt.updatedAt
			return &t
		}
		return th.SnoozedAt
	}
	if !th.SnoozedUntil.After(now) {
		return th.SnoozedUntil
	}
	return nil
}

func isThreadWokeUnseen(th *shellThread, subReq subagentReq, now time.Time) (time.Time, bool) {
	woke := threadWokeAt(th, subReq, now)
	if woke == nil || (th.SettledOverride != nil && *th.SettledOverride == "settled") {
		return time.Time{}, false
	}
	if th.LastVisitedAt != nil && !th.LastVisitedAt.Before(*woke) {
		return time.Time{}, false
	}
	if r := latestRun(th); r != nil && r.completedAt != nil && !r.completedAt.Before(*woke) {
		return time.Time{}, false
	}
	return *woke, true
}

func excluded(th *shellThread, subReq subagentReq, now time.Time) bool {
	return th.ArchivedAt != nil || th.DeletedAt != nil ||
		(th.SettledOverride != nil && *th.SettledOverride == "settled") ||
		effectiveSnoozed(th, subReq, now)
}

// unseenCompletion is hasUnseenCompletion in T3's web and mobile clients: a thread never visited has no unseen completion.
func unseenCompletion(th *shellThread) bool {
	r := latestRun(th)
	if r == nil || r.completedAt == nil || r.status == "interrupted" || r.status == "cancelled" || th.LastVisitedAt == nil {
		return false
	}
	return r.completedAt.After(*th.LastVisitedAt)
}

// unseenRunFailure is hasUnseenRunFailure in threadSettled.ts.
func unseenRunFailure(th *shellThread) bool {
	r := latestRun(th)
	if r == nil || r.status != "failed" {
		return false
	}
	if th.LastVisitedAt == nil {
		return true
	}
	failedAt := firstTime(r.completedAt, r.startedAt, r.requestedAt)
	if failedAt.IsZero() {
		return true
	}
	return failedAt.After(*th.LastVisitedAt)
}

func firstTime(ts ...*time.Time) time.Time {
	for _, t := range ts {
		if t != nil && !t.IsZero() {
			return *t
		}
	}
	return time.Time{}
}

func maxTime(ts ...time.Time) time.Time {
	var best time.Time
	for _, t := range ts {
		if t.After(best) {
			best = t
		}
	}
	return best
}

func ptrTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// resolveActivityTimestamp is resolveThreadActivityTimestamp in threadSort.ts.
func resolveActivityTimestamp(th *shellThread) time.Time {
	r := latestRun(th)
	var req, start, done time.Time
	if r != nil {
		req, start, done = ptrTime(r.requestedAt), ptrTime(r.startedAt), ptrTime(r.completedAt)
	}
	if best := maxTime(ptrTime(th.LatestUserMessageAt), req, start, done); !best.IsZero() {
		return best
	}
	return th.UpdatedAt
}

// sortWorkingAt is sortWorkingThreadsBySend in threadInbox.ts.
func sortWorkingAt(th *shellThread) time.Time {
	var send time.Time
	if th.LatestUserAuthoredMessageAt.Set {
		send = ptrTime(th.LatestUserAuthoredMessageAt.T)
	} else if r := latestRun(th); r != nil {
		send = ptrTime(r.requestedAt)
	}
	return maxTime(th.CreatedAt, send)
}

// sortInboxAt is sortInboxThreadsByReturn in threadInbox.ts.
func sortInboxAt(th *shellThread, observedReturn time.Time) time.Time {
	r := latestRun(th)
	var req, done time.Time
	if r != nil {
		req, done = ptrTime(r.requestedAt), ptrTime(r.completedAt)
	}
	return maxTime(th.CreatedAt, ptrTime(th.UnsettledAt), req, done, observedReturn)
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
	kind      agent.WorkingKind
	since     time.Time
}

// tracker holds one connection's shell and turns it into agents. It is not safe for concurrent use.
type tracker struct {
	env        string
	kinds      map[string]string // provider instance id → driver
	synced     bool
	seq        int64
	last       time.Time
	threads    map[string]*shellThread
	projects   map[string]shellProject
	grace      map[string]graceState
	returnedAt map[string]time.Time
	shown      map[string]agent.Agent
}

func newTracker(env string, kinds map[string]string) *tracker {
	return &tracker{
		env:        env,
		kinds:      kinds,
		threads:    map[string]*shellThread{},
		projects:   map[string]shellProject{},
		grace:      map[string]graceState{},
		returnedAt: map[string]time.Time{},
		shown:      map[string]agent.Agent{},
	}
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
			t.returnedAt[id] = now
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
		if isListedLineage(th) && th.SnoozedUntil != nil && th.SnoozedUntil.After(t.last) {
			earlier(*th.SnoozedUntil)
		}
	}
	return next, !next.IsZero()
}

// observe is the per-thread step of createInboxReturnTracker().observe in threadInbox.ts. Unlike T3, a pinned
// thread still gets the grace, since TrayInbox lists it like any other.
func (t *tracker) observe(now time.Time) {
	subReqs := pendingSubagentRequests(t.threads)
	for id, th := range t.threads {
		if !isListedLineage(th) {
			continue
		}
		subReq := subReqs[id]
		prev, had := t.grace[id]
		actual := isThreadWorking(th, subReq)
		placement := ""
		if actual || prev.working {
			placement = placementKey(th)
		}
		moved := had && prev.placement != placement
		actionable := hasPendingApprovals(th) || hasPendingUserInput(th) || subReq.kind != "" ||
			th.PendingGuestMessageCount > 0 || planWaiting(th) || hasActionableFailure(th)
		r := latestRun(th)
		endedLongAgo := r != nil && r.completedAt != nil && !r.completedAt.After(now.Add(-staleExit))
		var deadline time.Time
		if !actual && prev.working && !actionable && !moved && !excluded(th, subReq, now) && !endedLongAgo {
			deadline = prev.deadline
			if deadline.IsZero() {
				deadline = now.Add(workingExitGrace)
			}
		}
		working := actual || deadline.After(now)
		if prev.working && !working {
			t.returnedAt[id] = now
		}
		kind, since := prev.kind, prev.since
		if actual {
			kind = workingKindOf(th)
			since = workingSince(th, kind)
		} else if !working {
			deadline, kind, since = time.Time{}, "", time.Time{}
		}
		t.grace[id] = graceState{working: working, deadline: deadline, placement: placement, kind: kind, since: since}
	}
	for id := range t.grace {
		if th, ok := t.threads[id]; !ok || !isListedLineage(th) {
			delete(t.grace, id)
			delete(t.returnedAt, id)
		}
	}
}

func (t *tracker) agents(now time.Time) map[string]agent.Agent {
	subReqs := pendingSubagentRequests(t.threads)
	out := map[string]agent.Agent{}
	for _, th := range t.threads {
		if !isListedLineage(th) {
			continue
		}
		subReq := subReqs[th.ID]
		if excluded(th, subReq, now) {
			continue
		}
		a := t.agent(th, subReq, now)
		out[a.ID.Native] = a
	}
	return out
}

func (t *tracker) native(threadID string) string { return t.env + "/" + threadID }

func (t *tracker) agent(th *shellThread, subReq subagentReq, now time.Time) agent.Agent {
	a := agent.Agent{
		ID:   agent.ID{Orchestrator: orchestrator, Source: localSource, Native: t.native(th.ID)},
		Info: agent.Info{Title: th.Title, Kind: t.kind(th)},
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
	a.State, a.BlockedWhy, a.WorkingKind, a.Since, a.SortAt = t.classify(th, subReq, now)
	return a
}

func (t *tracker) classify(th *shellThread, subReq subagentReq, now time.Time) (agent.State, agent.BlockedWhy, agent.WorkingKind, time.Time, time.Time) {
	inboxSort := sortInboxAt(th, t.returnedAt[th.ID])
	reqTime := func() time.Time {
		if th.PendingRuntimeRequest != nil && !th.PendingRuntimeRequest.CreatedAt.IsZero() {
			return th.PendingRuntimeRequest.CreatedAt
		}
		if !subReq.createdAt.IsZero() {
			return subReq.createdAt
		}
		return resolveActivityTimestamp(th)
	}
	switch {
	case hasPendingApprovals(th) || subReq.kind == "approval":
		return agent.Blocked, agent.WhyPermission, "", reqTime(), inboxSort
	case hasPendingUserInput(th) || subReq.kind == "user_input":
		return agent.Blocked, agent.WhyQuestion, "", reqTime(), inboxSort
	case th.PendingGuestMessageCount > 0:
		return agent.Blocked, agent.WhyRequest, "", resolveActivityTimestamp(th), inboxSort
	case t.grace[th.ID].working:
		g := t.grace[th.ID]
		return agent.Working, "", g.kind, g.since, sortWorkingAt(th)
	}
	if rt := shellRuntime(th); rt != nil && rt.status == "failed" {
		if rt.lastErrorClass != nil && *rt.lastErrorClass == "usage_limit" {
			return agent.Blocked, agent.WhyLimited, "", resolveActivityTimestamp(th), inboxSort
		}
		r := latestRun(th)
		// A runtime failure without a failed run has nothing to date it, so it stays (Sidebar.logic.ts).
		if r == nil || r.status != "failed" || unseenRunFailure(th) {
			return agent.Error, "", "", resolveActivityTimestamp(th), inboxSort
		}
	}
	if wokeAt, ok := isThreadWokeUnseen(th, subReq, now); ok {
		return agent.Blocked, agent.WhyWoke, "", wokeAt, inboxSort
	}
	if unseenCompletion(th) {
		return agent.Done, "", "", resolveActivityTimestamp(th), inboxSort
	}
	return agent.Idle, "", "", resolveActivityTimestamp(th), inboxSort
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
