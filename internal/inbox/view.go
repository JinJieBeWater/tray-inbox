// Package inbox is the pure aggregation: no I/O, the clock is passed in.
package inbox

import (
	"time"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
)

// Row is one line of the panel: an agent with its sub-agents folded in.
type Row struct {
	ID agent.ID
	agent.Info
	State       agent.State
	BlockedWhy  agent.BlockedWhy
	WorkingKind agent.WorkingKind
	At          time.Time // display time (inbox idle: last activity; running: work/wait start; may be zero)
	SortAt      time.Time // sort time, newest first
}

type Disconnect struct {
	Orchestrator string
	Reason       agent.Reason
}

type Counts struct{ Error, Blocked, Done, Idle, Working int }

// View is everything the menu bar and the panel show.
type View struct {
	Disconnected []Disconnect
	Inbox        []Row
	Running      []Row
	Counts       Counts
}
