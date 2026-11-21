// Package agent holds the types adapters hand to the aggregator.
package agent

import (
	"context"
	"time"
)

type State int

const (
	Idle State = iota
	Working
	Done // finished and not yet seen; a seen finish is Idle
	Blocked
	Error
)

func (s State) String() string {
	switch s {
	case Working:
		return "working"
	case Done:
		return "done"
	case Blocked:
		return "blocked"
	case Error:
		return "error"
	}
	return "idle"
}

type BlockedWhy string

const (
	WhyOther      BlockedWhy = ""
	WhyPermission BlockedWhy = "permission"
	WhyQuestion   BlockedWhy = "question"
	WhyRequest    BlockedWhy = "request"
	WhyLimited    BlockedWhy = "limited"
	WhyWoke       BlockedWhy = "woke"
)

type WorkingKind string

const (
	WorkRunning   WorkingKind = ""
	WorkPreparing WorkingKind = "preparing"
	WorkGoal      WorkingKind = "goal"
	WorkWaiting   WorkingKind = "waiting"
)

// Kind is an agent's type. It only picks an icon; adapters map their native names onto these,
// and anything else shows the generic mark.
type Kind string

const (
	KindClaude      Kind = "claude"
	KindCodex       Kind = "codex"
	KindAntigravity Kind = "antigravity"
	KindPi          Kind = "pi"
	KindOpenCode    Kind = "opencode"
	KindCursor      Kind = "cursor"
	KindGrok        Kind = "grok"
	KindMuse        Kind = "muse"
)

// Kinds lists every Kind; each needs a mark in the panel.
var Kinds = []Kind{KindClaude, KindCodex, KindAntigravity, KindPi, KindOpenCode, KindCursor, KindGrok, KindMuse}

// Reason says why a source is disconnected.
type Reason string

const (
	ReasonReconnecting  Reason = "reconnecting"
	ReasonAppNotRunning Reason = "app-not-running"
	ReasonNoCommand     Reason = "no-command"
)

// ID is an agent's identity; agents with equal names but different IDs are never merged.
type ID struct{ Orchestrator, Source, Native string }

// Info is what the panel shows of an agent, carried to its row unchanged.
type Info struct {
	Project string // empty: no project
	Title   string
	Where   string // directory or branch, may be empty
	Kind    Kind
}

type Agent struct {
	ID ID
	Info
	State       State
	BlockedWhy  BlockedWhy
	WorkingKind WorkingKind
	Since       time.Time // when the orchestrator says the current state/display timer began; may be zero
	SortAt      time.Time // optional sort timestamp from the orchestrator; zero falls back to At
	Parent      string    // native id of the parent agent in the same source, may be empty
}

type Event interface{ event() }

// Snapshot replaces every agent of one source and marks the source connected.
type Snapshot struct {
	Orchestrator, Source string
	Agents               []Agent
}

type Upsert struct{ Agent Agent }

type Remove struct{ ID ID }

// SourceStatus with Up false removes the source's agents and shows it as disconnected.
type SourceStatus struct {
	Orchestrator, Source string
	Up                   bool
	Reason               Reason
}

func (Snapshot) event()     {}
func (Upsert) event()       {}
func (Remove) event()       {}
func (SourceStatus) event() {}

// Adapter connects one orchestrator. Orchestrator is the name it stamps on every ID it
// emits, which routes jumps back to it. Run calls emit from one goroutine at a time
// and returns when ctx ends. Jump may be called concurrently with Run.
type Adapter interface {
	Orchestrator() string
	Run(ctx context.Context, emit func(Event)) error
	Jump(ctx context.Context, id ID) error
}
