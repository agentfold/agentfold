package domain

import (
	"encoding/json"
	"fmt"
	"strings"
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
	Error string
	// What the agent reported through the board tools while the node ran.
	Summary          string
	StructuredOutput json.RawMessage
	ReviewApproved   *bool
	CreatedAt        time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
}

// NodeResult is how a node run ended.
type NodeResult struct {
	Status       NodeStatus
	Output       json.RawMessage
	OutputHandle string
	Error        string
	Usage
}

// JobAgentRun is the queue job that runs an agent or review node.
const JobAgentRun = "agent.run"

// AgentJob is the payload of an agent.run job.
type AgentJob struct {
	NodeRunID uuid.UUID `json:"node_run_id"`
	// Prompt continues an existing session of the node (answers, review
	// comments); the first run of a node uses its configured prompt.
	Prompt string `json:"prompt,omitempty"`
}

// Question is something an agent asked a human. Its node waits until every
// question it asked is answered.
type Question struct {
	ID          uuid.UUID
	CardID      uuid.UUID
	NodeRunID   uuid.UUID
	Text        string
	Answer      *string
	AnsweredAt  *time.Time
	DeliveredAt *time.Time
	CreatedAt   time.Time
}

// EntityID returns the question id.
func (q Question) EntityID() uuid.UUID { return q.ID }

// ReviewComment is a change a reviewer asks for.
type ReviewComment struct {
	ID        uuid.UUID
	CardID    uuid.UUID
	NodeRunID uuid.UUID
	Round     int
	File      string
	Line      *int
	Body      string
	CreatedAt time.Time
}

// ReviewInput is a comment as the reviewer submits it.
type ReviewInput struct {
	File string `json:"file,omitempty"`
	Line *int   `json:"line,omitempty"`
	Body string `json:"body"`
}

// AnswersPrompt is the prompt that resumes an agent with the answers to its
// questions.
func AnswersPrompt(questions []Question) string {
	var b strings.Builder
	b.WriteString("Answers to your questions:\n")
	for i, q := range questions {
		answer := ""
		if q.Answer != nil {
			answer = *q.Answer
		}
		fmt.Fprintf(&b, "\n%d. %s\nAnswer: %s\n", i+1, q.Text, answer)
	}
	return b.String()
}

// ReviewPrompt is the text a review hands to the next node: the comments
// to address, or that the work was approved.
func ReviewPrompt(approved bool, comments []ReviewComment) string {
	if approved {
		return "The review approved the changes."
	}
	var b strings.Builder
	b.WriteString("The review asked for changes. Address every comment:\n")
	for _, c := range comments {
		b.WriteString("\n- ")
		if c.File != "" {
			b.WriteString(c.File)
			if c.Line != nil {
				fmt.Fprintf(&b, ":%d", *c.Line)
			}
			b.WriteString(": ")
		}
		b.WriteString(c.Body)
	}
	return b.String()
}
