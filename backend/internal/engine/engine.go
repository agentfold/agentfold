// Package engine runs flows: it starts them when card events match their
// triggers and walks their graphs, running the board nodes itself and
// queueing agent nodes for the workers.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/google/uuid"

	"orquestrator/internal/agent"
	"orquestrator/internal/domain"
	"orquestrator/internal/events"
	"orquestrator/internal/store"
)

// Store is the persistence the engine needs. *store.Store implements it.
type Store interface {
	GetCardEvent(ctx context.Context, id uuid.UUID) (domain.CardEvent, error)
	AddCardEvent(ctx context.Context, cardID uuid.UUID, typ domain.CardEventType, payload any) (domain.CardEvent, error)
	GetCard(ctx context.Context, id uuid.UUID) (domain.Card, error)
	MoveCard(ctx context.Context, id, laneID uuid.UUID, position int) (domain.Card, error)
	RunnableFlows(ctx context.Context) ([]domain.RunnableFlow, error)
	GetFlowVersion(ctx context.Context, id uuid.UUID) (domain.FlowVersion, error)
	StartFlowRun(ctx context.Context, version domain.FlowVersion, cardID uuid.UUID, trigger string) (domain.FlowRun, bool, error)
	GetFlowRun(ctx context.Context, id uuid.UUID) (domain.FlowRun, error)
	FinishFlowRun(ctx context.Context, id uuid.UUID, status domain.RunStatus, msg string) (domain.FlowRun, error)
	CountNodeRuns(ctx context.Context, flowRunID uuid.UUID, nodeKey string) (int, error)
	CreateNodeRun(ctx context.Context, flowRunID uuid.UUID, nodeKey string, iteration int, input json.RawMessage) (domain.NodeRun, error)
	FinishNodeRun(ctx context.Context, id uuid.UUID, r domain.NodeResult) (domain.NodeRun, error)
	ContinueNodeRun(ctx context.Context, id uuid.UUID) (domain.NodeRun, bool, error)
	GetNodeRun(ctx context.Context, id uuid.UUID) (domain.NodeRun, error)
	WaitFlowRun(ctx context.Context, id uuid.UUID) (domain.FlowRun, error)
	ResumeIfAnswered(ctx context.Context, nodeRunID uuid.UUID) (domain.FlowRun, bool, error)
	EnqueueJob(ctx context.Context, j store.NewJob) (uuid.UUID, error)
	OpenReview(ctx context.Context, nodeRunID, cardID uuid.UUID, names []string, reply string) (domain.NodeRun, []domain.Artifact, error)
	ListProjects(ctx context.Context) ([]domain.Project, error)
	CreateChildCards(ctx context.Context, cards []domain.NewCard) ([]domain.Card, error)
	ListChildCards(ctx context.Context, parentID uuid.UUID) ([]domain.Card, error)
	ListCardFlowRuns(ctx context.Context, cardID uuid.UUID) ([]domain.FlowRun, error)
	WaitNodeRun(ctx context.Context, id uuid.UUID) (domain.NodeRun, error)
	WaitingChildrenNodeRun(ctx context.Context, cardID uuid.UUID) (domain.NodeRun, error)
	SettleWait(ctx context.Context, id uuid.UUID, r domain.NodeResult) (domain.NodeRun, bool, error)
}

// Publisher announces the changes the engine makes to the board.
type Publisher interface {
	Publish(e events.Event)
}

// AgentHandler runs agent.run jobs. *agent.Executor implements it.
type AgentHandler interface {
	Handle(ctx context.Context, payload json.RawMessage) error
}

// maxSteps bounds the board nodes run in a row without an agent, so a cycle
// of board nodes fails the run instead of spinning forever.
const maxSteps = 100

// Engine runs flows.
type Engine struct {
	Store  Store
	Agents AgentHandler
	Pub    Publisher
}

// HandleTrigger runs flow.trigger jobs: it starts every runnable flow whose
// trigger matches the card event. A child card entering a lane may settle
// the WaitChildren node of its parent.
func (e *Engine) HandleTrigger(ctx context.Context, payload json.RawMessage) error {
	var p domain.TriggerJob
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("flow.trigger payload: %w", err)
	}
	event, err := e.Store.GetCardEvent(ctx, p.CardEventID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil // the card was deleted meanwhile
	}
	if err != nil {
		return err
	}
	card, err := e.Store.GetCard(ctx, event.CardID)
	if err != nil {
		return err
	}
	flows, err := e.Store.RunnableFlows(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, flow := range flows {
		node, trigger, ok := matches(flow.Version.Graph, event, card)
		if ok {
			errs = append(errs, e.start(ctx, flow, card.ID, node, trigger.Event))
		}
	}
	return errors.Join(append(errs, e.wakeParent(ctx, card))...)
}

// HandleAgent runs agent.run jobs: it runs the agent node, then follows its
// output, or holds the run while the agent waits for answers. A retried job
// only follows the output if that is still pending.
func (e *Engine) HandleAgent(ctx context.Context, payload json.RawMessage) error {
	var p agent.Payload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("agent.run payload: %w", err)
	}
	err := e.Agents.Handle(ctx, payload)
	ctx = context.WithoutCancel(ctx)
	nr, getErr := e.Store.GetNodeRun(ctx, p.NodeRunID)
	if getErr != nil {
		return errors.Join(err, getErr)
	}
	if nr.Status == domain.NodeAwaitingInput {
		return errors.Join(err, e.await(ctx, nr))
	}
	return errors.Join(err, e.continueFrom(ctx, p.NodeRunID))
}

// await holds the run of a node waiting for answers. Answers given while
// the agent still ran resume it right away.
func (e *Engine) await(ctx context.Context, nr domain.NodeRun) error {
	fr, err := e.Store.WaitFlowRun(ctx, nr.FlowRunID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil // the run ended meanwhile
	}
	if err != nil {
		return err
	}
	e.Pub.Publish(events.Event{Type: "flow_run.updated", ID: fr.ID, Data: fr})
	fr, resumed, err := e.Store.ResumeIfAnswered(ctx, nr.ID)
	if err == nil && resumed {
		e.Pub.Publish(events.Event{Type: "flow_run.updated", ID: fr.ID, Data: fr})
	}
	return err
}

// run is a flow run being walked.
type run struct {
	domain.FlowRun
	graph domain.Graph
}

// start runs a flow for a card, from its trigger node.
func (e *Engine) start(ctx context.Context, flow domain.RunnableFlow, cardID uuid.UUID, triggerNode, trigger string) error {
	fr, started, err := e.Store.StartFlowRun(ctx, flow.Version, cardID, trigger)
	if err != nil {
		return err
	}
	if !started {
		return e.timeline(ctx, cardID, domain.FlowSkipped, flowSkipped{
			FlowID: flow.Version.FlowID, FlowName: flow.Name, Reason: "already_running",
		})
	}
	fr.FlowName = flow.Name
	e.Pub.Publish(events.Event{Type: "flow_run.created", ID: fr.ID, Data: fr})
	r := &run{FlowRun: fr, graph: flow.Version.Graph}
	err = e.timeline(ctx, cardID, domain.FlowStarted, flowStarted{
		FlowRunID: fr.ID, FlowID: fr.FlowID, FlowName: flow.Name, Version: flow.Version.Version, Trigger: trigger,
	})
	if err != nil {
		return err
	}
	next, err := e.route(ctx, r, triggerNode, "next", "")
	if err != nil || next == "" {
		return err
	}
	return e.advance(ctx, r, next, nil)
}

// continueFrom follows the output of a finished agent node run.
func (e *Engine) continueFrom(ctx context.Context, nodeRunID uuid.UUID) error {
	nr, ok, err := e.Store.ContinueNodeRun(ctx, nodeRunID)
	if err != nil || !ok {
		return err
	}
	fr, err := e.Store.GetFlowRun(ctx, nr.FlowRunID)
	if err != nil {
		return err
	}
	version, err := e.Store.GetFlowVersion(ctx, fr.FlowVersionID)
	if err != nil {
		return err
	}
	r := &run{FlowRun: fr, graph: version.Graph}
	node, _ := findNode(r.graph, nr.NodeKey)
	if err := e.nodeFinished(ctx, r, nr, node.Type); err != nil {
		return err
	}
	if !fr.Status.Active() {
		return nil // the run ended meanwhile
	}
	next, err := e.route(ctx, r, nr.NodeKey, nr.OutputHandle, nr.Error)
	if err != nil || next == "" {
		return err
	}
	return e.advance(ctx, r, next, nr.Output)
}

// advance enters the node key, then the nodes after it, until the run ends
// or waits for an agent. input is the output of the node before.
func (e *Engine) advance(ctx context.Context, r *run, key string, input json.RawMessage) error {
	for step := 0; key != ""; step++ {
		if step == maxSteps {
			return e.finish(ctx, r, domain.RunFailed, fmt.Sprintf("o fluxo passou por %d nós seguidos sem agente; verifique se há um ciclo", maxSteps))
		}
		node, ok := findNode(r.graph, key)
		if !ok {
			return e.finish(ctx, r, domain.RunFailed, fmt.Sprintf("o nó %s não existe na versão do fluxo", key))
		}
		nr, err := e.enter(ctx, r, node, input)
		if err != nil {
			return err
		}
		if node.Type == domain.NodeAgent {
			return e.queueAgent(ctx, nr, input)
		}
		if node.Type == domain.NodeHumanReview {
			return e.review(ctx, r, node, nr, input)
		}
		result, err := e.runBoardNode(ctx, r, node, input)
		if err != nil {
			return err
		}
		if result.Status == domain.NodeWaiting {
			return e.waitChildren(ctx, r, nr)
		}
		if nr, err = e.Store.FinishNodeRun(ctx, nr.ID, result); err != nil {
			return err
		}
		if err := e.nodeFinished(ctx, r, nr, node.Type); err != nil {
			return err
		}
		if node.Type == domain.NodeEnd {
			return e.finish(ctx, r, domain.RunDone, "")
		}
		if key, err = e.route(ctx, r, key, nr.OutputHandle, nr.Error); err != nil {
			return err
		}
		input = nr.Output
	}
	return nil
}

// enter records that the run entered a node.
func (e *Engine) enter(ctx context.Context, r *run, node domain.Node, input json.RawMessage) (domain.NodeRun, error) {
	count, err := e.Store.CountNodeRuns(ctx, r.ID, node.ID)
	if err != nil {
		return domain.NodeRun{}, err
	}
	nr, err := e.Store.CreateNodeRun(ctx, r.ID, node.ID, count+1, input)
	if err != nil {
		return domain.NodeRun{}, err
	}
	return nr, e.timeline(ctx, r.CardID, domain.NodeStarted, nodeStarted{
		FlowRunID: r.ID, NodeKey: node.ID, NodeType: node.Type, Iteration: nr.Iteration,
	})
}

// queueAgent queues the run of an agent node with what the node before
// handed on as its message: after the node prompt in a new session, alone
// when the node is re-entered and resumes its session.
func (e *Engine) queueAgent(ctx context.Context, nr domain.NodeRun, input json.RawMessage) error {
	payload := agent.Payload{NodeRunID: nr.ID, Prompt: domain.MessageFrom(input)}
	// No requirements yet: agents run on the single host of the MVP.
	_, err := e.Store.EnqueueJob(ctx, store.NewJob{Kind: agent.JobKind, Payload: payload, MaxAttempts: 2})
	return err
}

// runBoardNode runs a node that does not need an agent. MoveCard and End
// hand on the output of the node before, so agents after them still get
// it. A WaitChildren node whose children are on their way waits.
func (e *Engine) runBoardNode(ctx context.Context, r *run, node domain.Node, input json.RawMessage) (domain.NodeResult, error) {
	var result domain.NodeResult
	var err error
	switch node.Type {
	case domain.NodeMoveCard:
		result, err = e.moveCard(ctx, r, node)
	case domain.NodeEnd:
		result = domain.NodeResult{Status: domain.NodeDone}
	case domain.NodeCreateCards:
		return e.createCards(ctx, r, node, input)
	case domain.NodeWaitChildren:
		settled, err := e.childrenResult(ctx, r.CardID, node)
		if settled == nil {
			return domain.NodeResult{Status: domain.NodeWaiting}, err
		}
		return *settled, err
	default:
		return failed(fmt.Sprintf("o nó %s (%s) ainda não é suportado", node.ID, node.Type)), nil
	}
	if result.Status == domain.NodeDone {
		result.Output = input
	}
	return result, err
}

// moveCard moves the card to the end of the node's lane.
func (e *Engine) moveCard(ctx context.Context, r *run, node domain.Node) (domain.NodeResult, error) {
	var cfg domain.MoveCardConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return failed("configuração inválida: " + err.Error()), nil
	}
	card, err := e.Store.MoveCard(ctx, r.CardID, cfg.LaneID, math.MaxInt32)
	if errors.Is(err, domain.ErrInvalidReference) {
		return failed("a baia de destino não existe mais"), nil
	}
	if err != nil {
		return domain.NodeResult{}, err
	}
	e.Pub.Publish(events.Event{Type: "card.moved", ID: card.ID, Data: card})
	return domain.NodeResult{Status: domain.NodeDone, OutputHandle: "next"}, nil
}

func failed(msg string) domain.NodeResult {
	return domain.NodeResult{Status: domain.NodeFailed, Error: msg}
}

// route returns the node an output of key leads to. When it leads nowhere
// the run ends: failed if the node failed, done otherwise.
func (e *Engine) route(ctx context.Context, r *run, key, handle, nodeErr string) (string, error) {
	for _, edge := range r.graph.Edges {
		if edge.Source == key && edge.SourceHandle == handle && handle != "" {
			return edge.Target, nil
		}
	}
	if nodeErr != "" || handle == "failed" {
		return "", e.finish(ctx, r, domain.RunFailed, nodeErr)
	}
	return "", e.finish(ctx, r, domain.RunDone, "")
}

// finish ends the run.
func (e *Engine) finish(ctx context.Context, r *run, status domain.RunStatus, msg string) error {
	fr, err := e.Store.FinishFlowRun(ctx, r.ID, status, msg)
	if errors.Is(err, domain.ErrNotFound) {
		return nil // ended meanwhile
	}
	if err != nil {
		return err
	}
	fr.FlowName = r.FlowName
	e.Pub.Publish(events.Event{Type: "flow_run.updated", ID: fr.ID, Data: fr})
	err = e.timeline(ctx, r.CardID, domain.FlowFinished, flowFinished{
		FlowRunID: r.ID, FlowName: r.FlowName, Status: status, Error: msg,
	})
	if err != nil {
		return err
	}
	// A child that failed may settle the WaitChildren node of its parent.
	card, err := e.Store.GetCard(ctx, r.CardID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil // deleted meanwhile
	}
	if err != nil {
		return err
	}
	return e.wakeParent(ctx, card)
}

// nodeFinished records the end of a node run in the card timeline.
func (e *Engine) nodeFinished(ctx context.Context, r *run, nr domain.NodeRun, typ domain.NodeType) error {
	return e.timeline(ctx, r.CardID, domain.NodeFinished, nodeFinished{
		FlowRunID: r.ID, NodeKey: nr.NodeKey, NodeType: typ, Iteration: nr.Iteration,
		Status: nr.Status, OutputHandle: nr.OutputHandle, Error: nr.Error,
	})
}

// timeline adds an entry to the card timeline and announces it.
func (e *Engine) timeline(ctx context.Context, cardID uuid.UUID, typ domain.CardEventType, payload any) error {
	event, err := e.Store.AddCardEvent(ctx, cardID, typ, payload)
	if err != nil {
		return err
	}
	e.Pub.Publish(events.Event{Type: "card_event.created", ID: event.ID, Data: event})
	return nil
}

func findNode(g domain.Graph, key string) (domain.Node, bool) {
	for _, n := range g.Nodes {
		if n.ID == key {
			return n, true
		}
	}
	return domain.Node{}, false
}

// Timeline payloads.
type (
	flowStarted struct {
		FlowRunID uuid.UUID `json:"flow_run_id"`
		FlowID    uuid.UUID `json:"flow_id"`
		FlowName  string    `json:"flow_name"`
		Version   int       `json:"version"`
		Trigger   string    `json:"trigger"`
	}
	flowSkipped struct {
		FlowID   uuid.UUID `json:"flow_id"`
		FlowName string    `json:"flow_name"`
		Reason   string    `json:"reason"`
	}
	flowFinished struct {
		FlowRunID uuid.UUID        `json:"flow_run_id"`
		FlowName  string           `json:"flow_name"`
		Status    domain.RunStatus `json:"status"`
		Error     string           `json:"error,omitempty"`
	}
	nodeStarted struct {
		FlowRunID uuid.UUID       `json:"flow_run_id"`
		NodeKey   string          `json:"node_key"`
		NodeType  domain.NodeType `json:"node_type"`
		Iteration int             `json:"iteration"`
	}
	nodeFinished struct {
		FlowRunID    uuid.UUID         `json:"flow_run_id"`
		NodeKey      string            `json:"node_key"`
		NodeType     domain.NodeType   `json:"node_type"`
		Iteration    int               `json:"iteration"`
		Status       domain.NodeStatus `json:"status"`
		OutputHandle string            `json:"output_handle,omitempty"`
		Error        string            `json:"error,omitempty"`
	}
)
