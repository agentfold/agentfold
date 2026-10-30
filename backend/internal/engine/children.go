package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/events"
)

// createCards creates a child card for each card the node before handed
// on, all or none, and hands on the cards it created.
func (e *Engine) createCards(ctx context.Context, r *run, node domain.Node, input json.RawMessage) (domain.NodeResult, error) {
	var cfg domain.CreateCardsConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return failed("configuração inválida: " + err.Error()), nil
	}
	specs, err := domain.ChildCardsFrom(input)
	if err != nil {
		return failed(err.Error()), nil
	}
	projects, err := e.Store.ListProjects(ctx)
	if err != nil {
		return domain.NodeResult{}, err
	}
	at := domain.Placement(cfg)
	cards, err := domain.ChildCards(r.CardID, specs, projects, func(int) (domain.Placement, error) { return at, nil })
	if err != nil {
		return failed(err.Error()), nil
	}
	created, err := e.Store.CreateChildCards(ctx, cards)
	if errors.Is(err, domain.ErrUnknownDependency) {
		return failed("um card da lista depende de um card que não existe"), nil
	}
	if errors.Is(err, domain.ErrInvalidReference) {
		return failed("a baia ou o tipo dos cards criados não existe mais"), nil
	}
	if err != nil {
		return domain.NodeResult{}, err
	}
	out := childrenCreated{FlowRunID: r.ID, NodeKey: node.ID}
	for i, c := range created {
		e.Pub.Publish(events.Event{Type: "card.created", ID: c.ID, Data: c})
		out.Cards = append(out.Cards, childCard{ID: c.ID, Ref: specs[i].Ref, Title: c.Title, Project: specs[i].Project})
	}
	if err := e.timeline(ctx, r.CardID, domain.ChildrenCreated, out); err != nil {
		return domain.NodeResult{}, err
	}
	output, _ := json.Marshal(map[string]any{"cards": out.Cards}) // plain values
	return domain.NodeResult{Status: domain.NodeDone, OutputHandle: "next", Output: output}, nil
}

// waitResult settles a WaitChildren or CheckProperty node, or returns nil
// while it waits.
func (e *Engine) waitResult(ctx context.Context, cardID uuid.UUID, node domain.Node) (*domain.NodeResult, error) {
	if node.Type == domain.NodeCheckProperty {
		return e.propertyResult(ctx, cardID, node)
	}
	return e.childrenResult(ctx, cardID, node)
}

// Child card states, as WaitChildren sees them.
const (
	childPending = "pending"
	childDone    = "done"
	childFailed  = "failed"
)

// childrenResult settles a WaitChildren node from the child cards of the
// card that were not canceled: a child is done once it has the done
// property, failed when its last run failed or was canceled before that.
// all_done when every child is done; some_failed as soon as one failed,
// or in all_finished mode once every child is done or failed. It returns
// nil while it waits. The output reports each child, and what went wrong
// with the failed ones, for the agent the flow takes it to.
func (e *Engine) childrenResult(ctx context.Context, cardID uuid.UUID, node domain.Node) (*domain.NodeResult, error) {
	var cfg domain.WaitChildrenConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		r := failed("configuração inválida: " + err.Error())
		r.OutputHandle = "some_failed"
		return &r, nil
	}
	children, err := e.Store.ListChildCards(ctx, cardID)
	if err != nil {
		return nil, err
	}
	key, value := strings.TrimSpace(cfg.PropertyKey), strings.TrimSpace(cfg.PropertyValue)
	states := make([]childCard, 0, len(children))
	count := map[string]int{}
	for _, c := range children {
		if c.CanceledAt != nil {
			continue // canceled tasks no longer count
		}
		state := childCard{ID: c.ID, Title: c.Title, State: childPending}
		done := c.HasProperty(key, value)
		if cfg.DoneLaneID != nil {
			done = c.LaneID == *cfg.DoneLaneID
		}
		if done {
			state.State = childDone
		} else if err := e.failure(ctx, c.ID, &state); err != nil {
			return nil, err
		}
		count[state.State]++
		states = append(states, state)
	}
	handle := ""
	switch {
	case count[childFailed] > 0 && (cfg.Mode != domain.WaitAllFinished || count[childPending] == 0):
		handle = "some_failed"
	case count[childFailed] == 0 && count[childPending] == 0:
		handle = "all_done"
	default:
		return nil, nil
	}
	output, _ := json.Marshal(map[string]any{"result": childrenReport(states, count), "children": states}) // plain values
	return &domain.NodeResult{Status: domain.NodeDone, OutputHandle: handle, Output: output}, nil
}

// failure marks state failed when the last run of the child failed or was
// canceled, with what went wrong.
func (e *Engine) failure(ctx context.Context, childID uuid.UUID, state *childCard) error {
	runs, err := e.Store.ListCardFlowRuns(ctx, childID)
	if err != nil || len(runs) == 0 || (runs[0].Status != domain.RunFailed && runs[0].Status != domain.RunCanceled) {
		return err
	}
	nodes, err := e.Store.ListNodeRuns(ctx, runs[0].ID)
	if err != nil {
		return err
	}
	state.State, state.Flow, state.Error, state.Result = childFailed, runs[0].FlowName, runs[0].Error, domain.LastMessage(nodes)
	return nil
}

// childrenReport tells how the children went, in words for an agent.
func childrenReport(states []childCard, count map[string]int) string {
	if count[childFailed] == 0 {
		return fmt.Sprintf("Todas as %d tasks foram concluídas.", len(states))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d de %d tasks concluídas; %d falharam", count[childDone], len(states), count[childFailed])
	if count[childPending] > 0 {
		fmt.Fprintf(&b, " e %d ainda não terminaram", count[childPending])
	}
	b.WriteString(".")
	for _, c := range states {
		if c.State != childFailed {
			continue
		}
		fmt.Fprintf(&b, "\n\n- %q (%s) falhou no fluxo %s", c.Title, c.ID, c.Flow)
		if c.Error != "" {
			fmt.Fprintf(&b, ": %s", c.Error)
		}
		if c.Result != "" {
			fmt.Fprintf(&b, "\n  Última mensagem do agente: %s", c.Result)
		}
	}
	return b.String()
}

// wait holds the run at a WaitChildren or CheckProperty node until the
// cards it waits for settle it. They may have settled while the node was
// being held, so it looks once more afterwards.
func (e *Engine) wait(ctx context.Context, r *run, nr domain.NodeRun) error {
	held, err := e.Store.WaitNodeRun(ctx, nr.ID)
	if err != nil {
		return err
	}
	e.Pub.Publish(events.Event{Type: "node_run.updated", ID: held.ID, Data: held})
	fr, err := e.Store.WaitFlowRun(ctx, r.ID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil // the run ended meanwhile
	}
	if err != nil {
		return err
	}
	fr.FlowName = r.FlowName
	e.Pub.Publish(events.Event{Type: "flow_run.updated", ID: fr.ID, Data: fr})
	return e.settle(ctx, r.CardID)
}

// HandleChildChanged runs flow.child_changed jobs: something happened to
// a child card outside the engine.
func (e *Engine) HandleChildChanged(ctx context.Context, payload json.RawMessage) error {
	var p domain.ChildChangedJob
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("flow.child_changed payload: %w", err)
	}
	card, err := e.Store.GetCard(ctx, p.CardID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil // deleted meanwhile
	}
	if err != nil {
		return err
	}
	return e.wakeParent(ctx, card)
}

// wakeParent settles the WaitChildren node the parent of card waits at, if
// the children are now settled.
func (e *Engine) wakeParent(ctx context.Context, card domain.Card) error {
	if card.ParentCardID == nil {
		return nil
	}
	return e.settle(ctx, *card.ParentCardID)
}

// settle ends the WaitChildren or CheckProperty node the flow of card
// waits at once the cards it waits for settle it, and follows its output.
func (e *Engine) settle(ctx context.Context, cardID uuid.UUID) error {
	nr, err := e.Store.WaitingNodeRun(ctx, cardID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
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
	node, _ := findNode(version.Graph, nr.NodeKey) // waiting node runs come from the graph
	result, err := e.waitResult(ctx, cardID, node)
	if err != nil || result == nil {
		return err
	}
	nr, settled, err := e.Store.SettleWait(ctx, nr.ID, *result)
	if err != nil || !settled {
		return err
	}
	e.Pub.Publish(events.Event{Type: "node_run.updated", ID: nr.ID, Data: nr})
	fr.Status = domain.RunRunning
	e.Pub.Publish(events.Event{Type: "flow_run.updated", ID: fr.ID, Data: fr})
	return e.continueFrom(ctx, nr.ID)
}

// Timeline payloads and outputs of the child card nodes.
type (
	childCard struct {
		ID      uuid.UUID `json:"id"`
		Ref     string    `json:"ref,omitempty"`
		Title   string    `json:"title"`
		Project string    `json:"project,omitempty"`
		State   string    `json:"state,omitempty"`
		// How the last run of a failed child went.
		Flow   string `json:"flow,omitempty"`
		Error  string `json:"error,omitempty"`
		Result string `json:"result,omitempty"`
	}
	childrenCreated struct {
		FlowRunID uuid.UUID   `json:"flow_run_id"`
		NodeKey   string      `json:"node_key"`
		Cards     []childCard `json:"cards"`
	}
)
