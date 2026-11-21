// Package t3 reads the local T3 Code server's thread shell (orchestration protocol 2).
package t3

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// shellThread holds the OrchestrationV2ThreadShell fields TrayInbox reads (packages/contracts/src/orchestrationV2.ts).
type shellThread struct {
	CreationSource     string `json:"creationSource"`
	ID                 string `json:"id"`
	ProjectID          string `json:"projectId"`
	Title              string `json:"title"`
	ProviderInstanceID string `json:"providerInstanceId"`
	ModelSelection     struct {
		InstanceID string `json:"instanceId"`
		Provider   string `json:"provider"`
	} `json:"modelSelection"`
	InteractionMode string  `json:"interactionMode"`
	Branch          *string `json:"branch"`
	WorktreePath    *string `json:"worktreePath"`
	Lineage         struct {
		ParentThreadID       *string `json:"parentThreadId"`
		RelationshipToParent *string `json:"relationshipToParent"`
		SideChat             bool    `json:"sideChat"`
	} `json:"lineage"`
	LatestRunID           *string    `json:"latestRunId"`
	LatestRunRequestedAt  *time.Time `json:"latestRunRequestedAt"`
	LatestRunStartedAt    *time.Time `json:"latestRunStartedAt"`
	LatestRunCompletedAt  optTime    `json:"latestRunCompletedAt"`
	ActiveRunID           *string    `json:"activeRunId"`
	ActivityRunStartedAt  optTime    `json:"activityRunStartedAt"`
	ActivityRunStatus     optString  `json:"activityRunStatus"`
	BlockedOnThreadWait   bool       `json:"blockedOnThreadWait"`
	Status                string     `json:"status"`
	LastError             *string    `json:"lastError"`
	LastErrorClass        *string    `json:"lastErrorClass"`
	PendingRuntimeRequest *struct {
		Kind      string    `json:"kind"`
		CreatedAt time.Time `json:"createdAt"`
		UserOnly  bool      `json:"userOnly"`
	} `json:"pendingRuntimeRequest"`
	PendingGuestMessageCount    int        `json:"pendingGuestMessageCount"`
	LatestUserMessageAt         *time.Time `json:"latestUserMessageAt"`
	LatestUserAuthoredMessageAt optTime    `json:"latestUserAuthoredMessageAt"`
	HasActionableProposedPlan   bool       `json:"hasActionableProposedPlan"`
	PendingBackgroundTasks      []struct {
		Kind    string `json:"kind"`
		Awaited bool   `json:"awaited"`
	} `json:"pendingBackgroundTasks"`
	Goal *struct {
		Status string `json:"status"`
	} `json:"goal"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	ArchivedAt      *time.Time `json:"archivedAt"`
	DeletedAt       *time.Time `json:"deletedAt"`
	PinnedAt        *time.Time `json:"pinnedAt"`
	SettledOverride *string    `json:"settledOverride"`
	SettledAt       *time.Time `json:"settledAt"`
	UnsettledAt     *time.Time `json:"unsettledAt"`
	SnoozedUntil    *time.Time `json:"snoozedUntil"`
	SnoozedAt       *time.Time `json:"snoozedAt"`
	ActiveOrderKey  *string    `json:"activeOrderKey"`
	LastVisitedAt   *time.Time `json:"lastVisitedAt"`
}

// optTime tells an absent key from null, matching T3's `=== undefined` vs `=== null` semantics.
type optTime struct {
	Set bool
	T   *time.Time
}

func (o *optTime) UnmarshalJSON(b []byte) error {
	o.Set = true
	return json.Unmarshal(b, &o.T)
}

type optString struct {
	Set bool
	S   *string
}

func (o *optString) UnmarshalJSON(b []byte) error {
	o.Set = true
	return json.Unmarshal(b, &o.S)
}

type shellProject struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	WorkspaceRoot string `json:"workspaceRoot"`
}

type shellSnapshot struct {
	SnapshotSequence int64          `json:"snapshotSequence"`
	Threads          []shellThread  `json:"threads"`
	Projects         []shellProject `json:"projects"`
}

// item is one OrchestrationV2ShellStreamItem.
type item struct {
	Kind      string
	Snapshot  *shellSnapshot
	Patch     bool // a project-only refresh that must not replace the threads
	Sequence  int64
	Location  string
	Thread    *shellThread
	ThreadID  string
	Project   *shellProject
	ProjectID string
}

func decodeItem(b []byte) (item, error) {
	var raw struct {
		Kind      string          `json:"kind"`
		Snapshot  *shellSnapshot  `json:"snapshot"`
		Roots     json.RawMessage `json:"resolvedRepositoryIdentityRoots"`
		Sequence  int64           `json:"sequence"`
		Location  string          `json:"location"`
		Thread    *shellThread    `json:"thread"`
		ThreadID  string          `json:"threadId"`
		Project   *shellProject   `json:"project"`
		ProjectID string          `json:"projectId"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return item{}, fmt.Errorf("t3: shell item: %w", err)
	}
	it := item{
		Kind: raw.Kind, Snapshot: raw.Snapshot, Patch: raw.Roots != nil, Sequence: raw.Sequence,
		Location: raw.Location, Thread: raw.Thread, ThreadID: raw.ThreadID, Project: raw.Project, ProjectID: raw.ProjectID,
	}
	var missing bool
	switch it.Kind {
	case "snapshot":
		missing = it.Snapshot == nil
	case "thread.updated":
		missing = it.Thread == nil
	case "thread.removed":
		missing = it.ThreadID == ""
	case "project.updated":
		missing = it.Project == nil
	case "project.removed":
		missing = it.ProjectID == ""
	}
	if missing {
		return item{}, errors.New("t3: shell item " + it.Kind + " is incomplete")
	}
	return it, nil
}
