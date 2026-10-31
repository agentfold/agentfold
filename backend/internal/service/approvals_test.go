package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

// approvalRepo has a node run of the node approve, which approves the
// output of the node before.
func approvalRepo() (*Service, *fakeRepo, *fakePub) {
	s, repo, pub := newService()
	repo.nodeRun = domain.NodeRun{ID: uuid.New(), FlowRunID: uuid.New(), NodeKey: "approve", Status: domain.NodeAwaitingReview}
	repo.version = domain.FlowVersion{Graph: domain.Graph{Nodes: []domain.Node{
		{ID: "review", Type: domain.NodeHumanReview, Config: json.RawMessage(`{}`)},
		{ID: "approve", Type: domain.NodeHumanReview, Config: json.RawMessage(`{"subject":"output"}`)},
	}}}
	return s, repo, pub
}

func TestDecideApproval(t *testing.T) {
	s, repo, pub := approvalRepo()
	got, err := s.DecideApproval(ctx, repo.nodeRun.ID, domain.DecisionChangesRequested, " split it ")
	if err != nil || !got.Approval || got.Type != domain.NodeHumanReview || got.Feedback != "split it" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if !reflect.DeepEqual(eventTypes(pub), []string{"node_run.updated", "flow_run.updated"}) {
		t.Fatalf("events %v", eventTypes(pub))
	}
	if run := pub.events[1].Data.(domain.FlowRun); run.Status != domain.RunRunning {
		t.Fatalf("published run %+v", run)
	}
	if _, err := s.DecideApproval(ctx, repo.nodeRun.ID, domain.DecisionApproved, ""); err != nil {
		t.Fatal(err)
	}
}

func TestDecideApprovalRefusals(t *testing.T) {
	s, repo, _ := approvalRepo()
	var verr *domain.ValidationError
	for _, tc := range []struct{ decision, text, field string }{
		{"rejected", "", "decision"},
		{domain.DecisionChangesRequested, " ", "message"},
	} {
		if _, err := s.DecideApproval(ctx, repo.nodeRun.ID, tc.decision, tc.text); !errors.As(err, &verr) || verr.Field != tc.field {
			t.Fatalf("%s: got %v", tc.decision, err)
		}
	}
	repo.nodeRun.NodeKey = "review"
	if _, err := s.DecideApproval(ctx, repo.nodeRun.ID, domain.DecisionApproved, ""); !errors.As(err, &verr) || verr.Field != "id" {
		t.Fatalf("artifact review: got %v", err)
	}
	repo.nodeRun.NodeKey = "gone"
	if _, err := s.DecideApproval(ctx, repo.nodeRun.ID, domain.DecisionApproved, ""); !errors.As(err, &verr) {
		t.Fatalf("node gone: got %v", err)
	}
	for _, method := range []string{"GetNodeRun", "GetFlowRun", "GetFlowVersion", "DecideApproval"} {
		s, repo, _ := approvalRepo()
		repo.errs = map[string]error{method: domain.ErrNotFound}
		if _, err := s.DecideApproval(ctx, repo.nodeRun.ID, domain.DecisionApproved, ""); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("%s: got %v", method, err)
		}
	}
}
