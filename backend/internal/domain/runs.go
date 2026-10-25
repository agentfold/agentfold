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

// FlowRun is one execution of a flow version for a card.
type FlowRun struct {
	ID            uuid.UUID
	FlowVersionID uuid.UUID
	CardID        uuid.UUID
	Trigger       string
	Status        RunStatus
	StartedAt     time.Time
	FinishedAt    *time.Time
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
