package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// RunStatus is the state of a flow run.
type RunStatus string

// Flow run states.
const (
	RunRunning  RunStatus = "running"
	RunWaiting  RunStatus = "waiting"
	RunDone     RunStatus = "done"
	RunFailed   RunStatus = "failed"
	RunCanceled RunStatus = "canceled"
)

// Active reports whether the run has not ended yet.
func (s RunStatus) Active() bool {
	return s == RunRunning || s == RunWaiting
}

// FlowRun is one execution of a flow version for a card.
type FlowRun struct {
	ID            uuid.UUID
	FlowID        uuid.UUID
	FlowVersionID uuid.UUID
	// FlowName is filled in when reading runs, not when changing them.
	FlowName   string
	CardID     uuid.UUID
	Trigger    string
	Status     RunStatus
	Error      string
	StartedAt  time.Time
	FinishedAt *time.Time
}

// EntityID returns the run id.
func (r FlowRun) EntityID() uuid.UUID { return r.ID }

// RunnableFlow is the version of an enabled flow that new runs use: the
// last one published.
type RunnableFlow struct {
	Name    string
	Version FlowVersion
}

// JobFlowTrigger is the queue job that starts the flows a card event
// triggers. The store enqueues it with the event.
const JobFlowTrigger = "flow.trigger"

// TriggerJob is the payload of a flow.trigger job.
type TriggerJob struct {
	CardEventID uuid.UUID `json:"card_event_id"`
}

// NodeStatus is the state of a node run.
type NodeStatus string

// Node run states.
const (
	NodeQueued        NodeStatus = "queued"
	NodeRunning       NodeStatus = "running"
	NodeAwaitingInput NodeStatus = "awaiting_input"
	NodeDone          NodeStatus = "done"
	NodeFailed        NodeStatus = "failed"
)

// Usage is what an agent run consumed.
type Usage struct {
	TokensIn   int64
	TokensOut  int64
	CacheRead  int64
	CacheWrite int64
	CostUSD    float64
	Turns      int
	DurationMS int64
}

// NodeRun is one execution of a node within a flow run.
type NodeRun struct {
	ID           uuid.UUID
	FlowRunID    uuid.UUID
	NodeKey      string
	Iteration    int
	Status       NodeStatus
	Input        json.RawMessage
	Output       json.RawMessage
	OutputHandle string
	SessionID    *uuid.UUID
	Usage
	Error      string
	CreatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
}

// NodeResult is how a node run ended.
type NodeResult struct {
	Status       NodeStatus
	Output       json.RawMessage
	OutputHandle string
	Error        string
	Usage
}
