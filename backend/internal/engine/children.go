package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
	byName := map[string]uuid.UUID{}
	names := make([]string, 0, len(projects))
	for _, p := range projects {
		byName[p.Name] = p.ID
		names = append(names, p.Name)
	}
	parent := r.CardID
	cards := make([]domain.ChildCard, len(specs))
	for i, spec := range specs {
		cards[i].NewCard = domain.NewCard{
			CardInput:    domain.CardInput{Title: spec.Title, Description: spec.Description, CardTypeID: cfg.CardTypeID},
			LaneID:       cfg.LaneID,
			ParentCardID: &parent,
		}
		if spec.Project == "" {
			continue
		}
		id, ok := byName[spec.Project]
		if !ok {
			slices.Sort(names)
			return failed(fmt.Sprintf("o card %q usa o projeto %q, que não existe (projetos: %s)", spec.Title, spec.Project, strings.Join(names, ", "))), nil
		}
		cards[i].ProjectID = &id
	}
	created, err := e.Store.CreateChildCards(ctx, cards)
	if errors.Is(err, domain.ErrInvalidReference) {
		return failed("a baia ou o tipo dos cards criados não existe mais"), nil
	}
	if err != nil {
		return domain.NodeResult{}, err
	}
	out := childrenCreated{FlowRunID: r.ID, NodeKey: node.ID}
	for i, c := range created {
		e.Pub.Publish(events.Event{Type: "card.created", ID: c.ID, Data: c})
		out.Cards = append(out.Cards, childCard{ID: c.ID, Title: c.Title, Project: specs[i].Project})
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
// card: all_done when every child is in the done lane, some_failed as soon
// as the last run of a child failed or was canceled before it got there.
// It returns nil while some child is still on its way.
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
	states := make([]childCard, len(children))
	count := map[string]int{}
	for i, c := range children {
		state := childPending
		if c.LaneID == cfg.DoneLaneID {
			state = childDone
		} else {
			runs, err := e.Store.ListCardFlowRuns(ctx, c.ID)
			if err != nil {
				return nil, err
			}
			if len(runs) > 0 && (runs[0].Status == domain.RunFailed || runs[0].Status == domain.RunCanceled) {
				state = childFailed
			}
		}
		count[state]++
		states[i] = childCard{ID: c.ID, Title: c.Title, State: state}
	}
	output, _ := json.Marshal(map[string]any{"children": states}) // plain values
	switch {
	case count[childFailed] > 0:
		return &domain.NodeResult{Status: domain.NodeDone, OutputHandle: "some_failed", Output: output}, nil
	case count[childPending] == 0:
		return &domain.NodeResult{Status: domain.NodeDone, OutputHandle: "all_done", Output: output}, nil
	}
	return nil, nil
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
		Title   string    `json:"title"`
		Project string    `json:"project,omitempty"`
		State   string    `json:"state,omitempty"`
	}
	childrenCreated struct {
		FlowRunID uuid.UUID   `json:"flow_run_id"`
		NodeKey   string      `json:"node_key"`
		Cards     []childCard `json:"cards"`
	}
)
