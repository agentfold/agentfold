package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

// approval is a node run awaiting the approval of {"result":"planned"}.
func approval(t *testing.T, s *Store) (domain.FlowRun, domain.NodeRun) {
	t.Helper()
	ctx := context.Background()
	run, dev := runningNode(t, s)
	if _, err := s.FinishNodeRun(ctx, dev.ID, domain.NodeResult{Status: domain.NodeDone}); err != nil {
		t.Fatal(err)
	}
	nr, err := s.CreateNodeRun(ctx, run.ID, "approve", 1, json.RawMessage(`{"result":"planned"}`))
	if err != nil {
		t.Fatal(err)
	}
	held, err := s.AwaitApproval(ctx, nr.ID)
	if err != nil || held.Status != domain.NodeAwaitingReview {
		t.Fatalf("AwaitApproval = %+v, %v", held, err)
	}
	if got, _ := s.GetFlowRun(ctx, run.ID); got.Status != domain.RunWaiting || !got.AwaitingReview {
		t.Fatalf("run %+v", got)
	}
	return run, held
}

func TestApproveOutput(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	run, nr := approval(t, s)
	got, err := s.DecideApproval(ctx, nr.ID, domain.DecisionApproved, "")
	if err != nil || got.Status != domain.NodeDone || got.OutputHandle != "approved" || got.Decision != domain.DecisionApproved ||
		string(got.Output) != `{"result": "planned"}` {
		t.Fatalf("approved = %+v (%s), %v", got, got.Output, err)
	}
	if fr, _ := s.GetFlowRun(ctx, run.ID); fr.Status != domain.RunRunning {
		t.Fatalf("run %+v", fr)
	}
	if _, err := s.DecideApproval(ctx, nr.ID, domain.DecisionApproved, ""); !errors.Is(err, domain.ErrReviewClosed) {
		t.Fatalf("deciding twice: %v", err)
	}
	if _, err := s.DecideApproval(ctx, uuid.New(), domain.DecisionApproved, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing node run: %v", err)
	}
	var jobs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind = $1 AND payload->>'node_run_id' = $2`, domain.JobFlowContinue, nr.ID.String()).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("continue jobs %d, %v", jobs, err)
	}
}

func TestRequestOutputChanges(t *testing.T) {
	s := newStore(t)
	_, nr := approval(t, s)
	got, err := s.DecideApproval(context.Background(), nr.ID, domain.DecisionChangesRequested, "split it")
	if err != nil || got.OutputHandle != "changes_requested" || got.Decision != domain.DecisionChangesRequested || got.Feedback != "split it" {
		t.Fatalf("changes = %+v, %v", got, err)
	}
	if msg := domain.MessageFrom(got.Output); msg != domain.ApprovalPrompt("split it") {
		t.Fatalf("message %q", msg)
	}
}

func TestAwaitApprovalOfAnEndedRun(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.AwaitApproval(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing node run: %v", err)
	}
	run, _ := runningNode(t, s)
	nr, err := s.CreateNodeRun(ctx, run.ID, "approve", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishFlowRun(ctx, run.ID, domain.RunCanceled, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AwaitApproval(ctx, nr.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ended run: %v", err)
	}
}

func TestApprovalFailures(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	run, nr := approval(t, s)
	for _, method := range []string{"GetNodeRunForUpdate", "SetNodeRunDecision", "SetFlowRunStatus"} {
		failOn(s, method, 1)
		if _, err := s.DecideApproval(ctx, nr.ID, domain.DecisionApproved, ""); !errors.Is(err, errInjected) {
			t.Fatalf("DecideApproval %s: got %v", method, err)
		}
	}
	s = New(pool)
	other, err := s.CreateNodeRun(ctx, run.ID, "approve", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"AwaitReviewNodeRun", "SetFlowRunStatus"} {
		failOn(s, method, 1)
		if _, err := s.AwaitApproval(ctx, other.ID); !errors.Is(err, errInjected) {
			t.Fatalf("AwaitApproval %s: got %v", method, err)
		}
	}
}
