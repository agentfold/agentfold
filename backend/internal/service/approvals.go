package service

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/events"
)

// runNode is a node run with what the graph of its version tells of it.
func runNode(g domain.Graph, nr domain.NodeRun) RunNode {
	i := slices.IndexFunc(g.Nodes, func(n domain.Node) bool { return n.ID == nr.NodeKey })
	if i < 0 {
		return RunNode{NodeRun: nr} // a node gone from the version
	}
	return RunNode{NodeRun: nr, Type: g.Nodes[i].Type, Approval: domain.ApprovesOutput(g.Nodes[i])}
}

// DecideApproval decides the approval of the output a HumanReview node
// run awaits: approved goes on, and changes_requested sends text to the
// agent the node leads to.
func (s *Service) DecideApproval(ctx context.Context, nodeRunID uuid.UUID, decision, text string) (RunNode, error) {
	text = strings.TrimSpace(text)
	switch {
	case decision != domain.DecisionApproved && decision != domain.DecisionChangesRequested:
		return RunNode{}, &domain.ValidationError{Field: "decision", Message: "must be approved or changes_requested"}
	case decision == domain.DecisionChangesRequested && text == "":
		return RunNode{}, &domain.ValidationError{Field: "message", Message: "is required to ask for changes"}
	}
	nr, err := s.repo.GetNodeRun(ctx, nodeRunID)
	if err != nil {
		return RunNode{}, err
	}
	run, err := s.repo.GetFlowRun(ctx, nr.FlowRunID)
	if err != nil {
		return RunNode{}, err
	}
	version, err := s.repo.GetFlowVersion(ctx, run.FlowVersionID)
	if err != nil {
		return RunNode{}, err
	}
	if !runNode(version.Graph, nr).Approval {
		return RunNode{}, &domain.ValidationError{Field: "id", Message: "the node does not approve an output; review its artifacts instead"}
	}
	if nr, err = s.repo.DecideApproval(ctx, nodeRunID, decision, text); err != nil {
		return RunNode{}, err
	}
	s.pub.Publish(events.Event{Type: "node_run.updated", ID: nr.ID, Data: nr})
	run.Status = domain.RunRunning // deciding sets it running again
	s.pub.Publish(events.Event{Type: "flow_run.updated", ID: run.ID, Data: run})
	return runNode(version.Graph, nr), nil
}
