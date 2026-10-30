package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/events"
)

// TaskSpec is a task an agent creates: a child card of its card, of the
// card type named Type, which starts in the type's initial lane.
type TaskSpec struct {
	domain.ChildCardSpec
	Type string
}

// Task is a child card as the agent that splits its card sees it.
type Task struct {
	Card     domain.Card
	TypeName string
	LaneName string
	// LastRun is the last flow run of the task, nil before any.
	LastRun *TaskRun
}

// TaskRun is how the last flow run of a task went. Result is the last
// message an agent of the run handed on (a gateway's feedback included).
type TaskRun struct {
	FlowName string
	Status   domain.RunStatus
	Error    string
	Result   string
}

// childrenCreatedPayload matches the children.created entries of the engine.
type childrenCreatedPayload struct {
	FlowRunID uuid.UUID          `json:"flow_run_id"`
	NodeKey   string             `json:"node_key"`
	Cards     []childCardPayload `json:"cards"`
}

type childCardPayload struct {
	ID      uuid.UUID `json:"id"`
	Title   string    `json:"title"`
	Project string    `json:"project,omitempty"`
}

// CreateTasks creates the tasks of an agent as child cards of its card,
// all or none, and returns them in the order of specs. The error messages
// are meant for the agent.
func (s *Service) CreateTasks(ctx context.Context, run AgentRun, specs []TaskSpec) ([]domain.Card, error) {
	if len(specs) == 0 {
		return nil, errors.New("informe ao menos uma task")
	}
	types, err := s.repo.ListCardTypes(ctx)
	if err != nil {
		return nil, err
	}
	projects, err := s.repo.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	byName := map[string]domain.CardType{}
	names := make([]string, len(types))
	for i, t := range types {
		byName[t.Name] = t
		names[i] = t.Name
	}
	children := make([]domain.ChildCardSpec, len(specs))
	for i, spec := range specs {
		children[i] = spec.ChildCardSpec
	}
	cards, err := domain.ChildCards(run.FlowRun.CardID, children, projects, func(i int) (domain.Placement, error) {
		name := strings.TrimSpace(specs[i].Type)
		t, ok := byName[name]
		if !ok {
			return domain.Placement{}, fmt.Errorf("a task %q usa o tipo %q, que não existe (tipos: %s)", children[i].Title, name, strings.Join(names, ", "))
		}
		if t.InitialLaneID == nil {
			return domain.Placement{}, fmt.Errorf("o tipo %q não tem baia inicial: peça a um humano para configurá-la no cadastro de tipos de card", t.Name)
		}
		return domain.Placement{LaneID: *t.InitialLaneID, CardTypeID: t.ID}, nil
	})
	if err != nil {
		return nil, err
	}
	created, err := s.repo.CreateChildCards(ctx, cards)
	switch {
	case errors.Is(err, domain.ErrUnknownDependency):
		return nil, errors.New("uma task depende de um id que não é de nenhum card")
	case err != nil:
		return nil, err
	}
	payload := childrenCreatedPayload{FlowRunID: run.FlowRun.ID, NodeKey: run.NodeRun.NodeKey}
	for i, c := range created {
		s.pub.Publish(events.Event{Type: "card.created", ID: c.ID, Data: c})
		payload.Cards = append(payload.Cards, childCardPayload{ID: c.ID, Title: c.Title, Project: children[i].Project})
	}
	return created, s.timeline(ctx, run.FlowRun.CardID, domain.ChildrenCreated, payload)
}

// ListTasks returns the child cards of the card of an agent, with how
// their last flow run went.
func (s *Service) ListTasks(ctx context.Context, run AgentRun) ([]Task, error) {
	children, err := s.repo.ListChildCards(ctx, run.FlowRun.CardID)
	if err != nil {
		return nil, err
	}
	types, err := s.repo.ListCardTypes(ctx)
	if err != nil {
		return nil, err
	}
	lanes, err := s.repo.ListLanes(ctx)
	if err != nil {
		return nil, err
	}
	names := map[uuid.UUID]string{}
	for _, t := range types {
		names[t.ID] = t.Name
	}
	for _, l := range lanes {
		names[l.ID] = l.Name
	}
	out := make([]Task, len(children))
	for i, c := range children {
		out[i] = Task{Card: c, TypeName: names[c.CardTypeID], LaneName: names[c.LaneID]}
		if out[i].LastRun, err = s.lastRun(ctx, c.ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// lastRun tells how the last flow run of a card went, nil before any.
func (s *Service) lastRun(ctx context.Context, cardID uuid.UUID) (*TaskRun, error) {
	runs, err := s.repo.ListCardFlowRuns(ctx, cardID)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	last := runs[0]
	nodes, err := s.repo.ListNodeRuns(ctx, last.ID)
	if err != nil {
		return nil, err
	}
	out := &TaskRun{FlowName: last.FlowName, Status: last.Status, Error: last.Error}
	for _, nr := range slices.Backward(nodes) {
		if msg := domain.MessageFrom(nr.Output); msg != "" {
			out.Result = msg
			break
		}
	}
	return out, nil
}

// task returns a child card of the card of an agent.
func (s *Service) task(ctx context.Context, run AgentRun, id uuid.UUID) (domain.Card, error) {
	card, err := s.repo.GetCard(ctx, id)
	if errors.Is(err, domain.ErrNotFound) || err == nil && (card.ParentCardID == nil || *card.ParentCardID != run.FlowRun.CardID) {
		return domain.Card{}, fmt.Errorf("o card %s não é uma task deste card (veja list_tasks)", id)
	}
	return card, err
}

// RetryTask runs again the flow whose last run of a task failed or was
// canceled, after taking back its cancel. A description replaces the
// task's own, to tell what to do differently.
func (s *Service) RetryTask(ctx context.Context, run AgentRun, id uuid.UUID, description string) (domain.Card, error) {
	card, err := s.task(ctx, run, id)
	if err != nil {
		return domain.Card{}, err
	}
	runs, err := s.repo.ListCardFlowRuns(ctx, id)
	switch {
	case err != nil:
		return domain.Card{}, err
	case len(runs) == 0:
		return domain.Card{}, errors.New("a task ainda não rodou nenhum fluxo: não há o que tentar de novo")
	case runs[0].Status.Active():
		return domain.Card{}, errors.New("a task ainda está rodando: espere ela terminar ou cancele-a")
	}
	if description = strings.TrimSpace(description); description != "" {
		card, err = s.UpdateCard(ctx, id, domain.CardInput{
			Title: card.Title, Description: description, CardTypeID: card.CardTypeID, ProjectID: card.ProjectID,
		})
		if err != nil {
			return domain.Card{}, err
		}
	}
	card, err = s.repo.RetryCard(ctx, id)
	return changed(s, "card.updated", card, err)
}

// CancelTask marks a task canceled, so WaitChildren no longer counts it,
// and cancels its active flow run, if any.
func (s *Service) CancelTask(ctx context.Context, run AgentRun, id uuid.UUID) (domain.Card, error) {
	if _, err := s.task(ctx, run, id); err != nil {
		return domain.Card{}, err
	}
	card, err := s.repo.CancelCard(ctx, id)
	if card, err = changed(s, "card.updated", card, err); err != nil {
		return card, err
	}
	runs, err := s.repo.ListCardFlowRuns(ctx, id)
	if err != nil || len(runs) == 0 || !runs[0].Status.Active() {
		return card, err
	}
	_, err = s.CancelFlowRun(ctx, runs[0].ID)
	return card, err
}
