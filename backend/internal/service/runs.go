package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/events"
)

// GetFlowRun returns one flow run.
func (s *Service) GetFlowRun(ctx context.Context, id uuid.UUID) (domain.FlowRun, error) {
	return s.repo.GetFlowRun(ctx, id)
}

// ListLatestFlowRuns returns one run per card that ever ran a flow: an
// active one if there is any, else the last one.
func (s *Service) ListLatestFlowRuns(ctx context.Context) ([]domain.FlowRun, error) {
	return s.repo.ListLatestFlowRuns(ctx)
}

// Entity returns the entity a board event of type typ ("<entity>.<change>")
// refers to, for the events other processes announce: cards, flow runs,
// node runs and card events. Node output events carry no entity.
func (s *Service) Entity(ctx context.Context, typ string, id uuid.UUID) (any, error) {
	entity, _, _ := strings.Cut(typ, ".")
	switch entity {
	case "card":
		return s.repo.GetCard(ctx, id)
	case "flow_run":
		return s.repo.GetFlowRun(ctx, id)
	case "card_event":
		return s.repo.GetCardEvent(ctx, id)
	case "node_run":
		return s.repo.GetNodeRun(ctx, id)
	case "node_output":
		return nil, nil // clients reload the output of the node run id
	}
	return nil, fmt.Errorf("no entity for event %q", typ)
}

// RunNode is a node run with the type of its node.
type RunNode struct {
	domain.NodeRun
	Type domain.NodeType
}

// RunDetail is a flow run with its node runs, in order.
type RunDetail struct {
	domain.FlowRun
	Nodes []RunNode
}

// GetRunDetail returns a flow run with its node runs.
func (s *Service) GetRunDetail(ctx context.Context, id uuid.UUID) (RunDetail, error) {
	run, err := s.repo.GetFlowRun(ctx, id)
	if err != nil {
		return RunDetail{}, err
	}
	return s.runDetail(ctx, run)
}

func (s *Service) runDetail(ctx context.Context, run domain.FlowRun) (RunDetail, error) {
	nodes, err := s.repo.ListNodeRuns(ctx, run.ID)
	if err != nil {
		return RunDetail{}, err
	}
	version, err := s.repo.GetFlowVersion(ctx, run.FlowVersionID)
	if err != nil {
		return RunDetail{}, err
	}
	types := map[string]domain.NodeType{}
	for _, n := range version.Graph.Nodes {
		types[n.ID] = n.Type
	}
	detail := RunDetail{FlowRun: run, Nodes: make([]RunNode, len(nodes))}
	for i, n := range nodes {
		detail.Nodes[i] = RunNode{NodeRun: n, Type: types[n.NodeKey]}
	}
	return detail, nil
}

// ListCardRuns returns the runs of a card with their node runs, newest first.
func (s *Service) ListCardRuns(ctx context.Context, cardID uuid.UUID) ([]RunDetail, error) {
	if _, err := s.repo.GetCard(ctx, cardID); err != nil {
		return nil, err
	}
	runs, err := s.repo.ListCardFlowRuns(ctx, cardID)
	if err != nil {
		return nil, err
	}
	out := make([]RunDetail, len(runs))
	for i, r := range runs {
		if out[i], err = s.runDetail(ctx, r); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ListNodeOutput returns what an agent did while running a node.
func (s *Service) ListNodeOutput(ctx context.Context, nodeRunID uuid.UUID) ([]domain.NodeOutput, error) {
	if _, err := s.repo.GetNodeRun(ctx, nodeRunID); err != nil {
		return nil, err
	}
	return s.repo.ListNodeOutput(ctx, nodeRunID)
}

// CancelFlowRun stops an active run and frees its card: its agents are
// interrupted and its open questions closed. ErrNotFound when the run is
// not active.
func (s *Service) CancelFlowRun(ctx context.Context, id uuid.UUID) (domain.FlowRun, error) {
	run, err := s.repo.GetFlowRun(ctx, id)
	if err != nil {
		return domain.FlowRun{}, err
	}
	canceled, err := s.repo.CancelFlowRun(ctx, id)
	if err != nil {
		return domain.FlowRun{}, err
	}
	canceled.FlowName = run.FlowName
	s.pub.Publish(events.Event{Type: "flow_run.updated", ID: id, Data: canceled})
	return canceled, s.timeline(ctx, run.CardID, domain.FlowFinished, flowFinishedPayload{
		FlowRunID: id, FlowName: run.FlowName, Status: domain.RunCanceled, Error: canceled.Error,
	})
}

// flowFinishedPayload matches the flow.finished entries of the engine.
type flowFinishedPayload struct {
	FlowRunID uuid.UUID        `json:"flow_run_id"`
	FlowName  string           `json:"flow_name"`
	Status    domain.RunStatus `json:"status"`
	Error     string           `json:"error,omitempty"`
}
