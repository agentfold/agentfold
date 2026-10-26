package domain

import (
	"bytes"
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
	// Decision is how a gateway decided, approved or rejected; empty
	// otherwise. Feedback is the reason or what must change.
	Decision   string
	Feedback   string
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

// Gateway decisions.
const (
	DecisionApproved = "approved"
	DecisionRejected = "rejected"
)

// JobAgentRun is the queue job that runs an agent node.
const JobAgentRun = "agent.run"

// AgentJob is the payload of an agent.run job.
type AgentJob struct {
	NodeRunID uuid.UUID `json:"node_run_id"`
	// Prompt is the message for the agent: what the node before handed on,
	// answers to its questions or a revision request. A new session starts
	// with the node prompt and the card, followed by it.
	Prompt string `json:"prompt,omitempty"`
}

// TriggerRevision is the trigger of the runs that revise an artifact: they
// resume the node that collected it and end with it, without following
// its outputs.
const TriggerRevision = "artifact_revision"

// MessageFrom is what an output hands to the next agent: the structured
// output when there is one, else the result text.
func MessageFrom(output json.RawMessage) string {
	var v struct {
		Result string          `json:"result"`
		Output json.RawMessage `json:"output"`
	}
	if json.Unmarshal(output, &v) != nil {
		return ""
	}
	if len(v.Output) > 0 && string(v.Output) != "null" {
		var b bytes.Buffer
		_ = json.Indent(&b, v.Output, "", "  ") // decoded above: valid JSON
		return b.String()
	}
	return v.Result
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

// OutputKind names an entry of what an agent did.
type OutputKind string

// Output kinds.
const (
	OutputText       OutputKind = "text"
	OutputToolUse    OutputKind = "tool_use"
	OutputToolResult OutputKind = "tool_result"
	OutputResult     OutputKind = "result"
)

// NodeOutput is one step of what an agent did while running a node: a
// message, a tool call or its result, or the final result.
type NodeOutput struct {
	ID        uuid.UUID
	NodeRunID uuid.UUID
	Kind      OutputKind
	Text      string
	Tool      string
	IsError   bool
	CreatedAt time.Time
}

// Artifact is a file of the card workspace an agent node collected, by
// name. Its review chat resumes that node until the card leaves the lane.
type Artifact struct {
	ID        uuid.UUID
	CardID    uuid.UUID
	Name      string
	NodeRunID uuid.UUID
	FrozenAt  *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// EntityID returns the artifact id.
func (a Artifact) EntityID() uuid.UUID { return a.ID }

// ArtifactVersion is the content of an artifact as one run collected it.
type ArtifactVersion struct {
	ID         uuid.UUID
	ArtifactID uuid.UUID
	Version    int
	Path       string
	Content    string
	NodeRunID  uuid.UUID
	CreatedAt  time.Time
}

// Artifact message roles.
const (
	RoleHuman = "human"
	RoleAgent = "agent"
)

// ArtifactMessage is a message of the review chat of an artifact.
type ArtifactMessage struct {
	ID         uuid.UUID
	ArtifactID uuid.UUID
	// Version is the one the human was reading, or the agent revised.
	Version   int
	Role      string
	Text      string
	FlowRunID *uuid.UUID
	CreatedAt time.Time
}

// RevisionPrompt is the message that resumes the agent of an artifact with
// what the human asked in its review chat.
func RevisionPrompt(name, path string, version int, text string) string {
	return fmt.Sprintf("A human reviewed version %d of the artifact %q (the file %s) and asks:\n\n%s\n\n"+
		"Update %s to address it, then finish; the board collects the file again.", version, name, path, text, path)
}
