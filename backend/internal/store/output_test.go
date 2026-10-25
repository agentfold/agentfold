package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

func TestNodeOutput(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, nr := runningNode(t, s)
	err := s.AddNodeOutput(ctx, nr.ID, []domain.NodeOutput{
		{Kind: domain.OutputToolUse, Tool: "Read", Text: "{}"},
		{Kind: domain.OutputResult, Text: "done", IsError: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.ListNodeOutput(ctx, nr.ID)
	if err != nil || len(out) != 2 || out[0].Tool != "Read" || out[1].Tool != "" || !out[1].IsError {
		t.Fatalf("ListNodeOutput = %+v, %v", out, err)
	}
	if err := s.AddNodeOutput(ctx, uuid.New(), []domain.NodeOutput{{Kind: domain.OutputText, Text: "x"}}); !errors.Is(err, domain.ErrInvalidReference) {
		t.Fatalf("output of an unknown node run: %v", err)
	}
	if _, err := s.ListNodeOutput(cancelled(), nr.ID); err == nil {
		t.Error("ListNodeOutput: expected error")
	}
}

func TestCancelFlowRun(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	run, nr := runningNode(t, s)
	queued, err := s.CreateNodeRun(ctx, run.ID, "other", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddQuestions(ctx, run.CardID, nr.ID, []string{"Q?"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnqueueJob(ctx, NewJob{Kind: domain.JobAgentRun, Payload: domain.AgentJob{NodeRunID: queued.ID}}); err != nil {
		t.Fatal(err)
	}
	other, err := s.EnqueueJob(ctx, NewJob{Kind: domain.JobAgentRun, Payload: domain.AgentJob{NodeRunID: uuid.New()}})
	if err != nil {
		t.Fatal(err)
	}

	canceled, err := s.CancelFlowRun(ctx, run.ID)
	if err != nil || canceled.Status != domain.RunCanceled || canceled.Error != "cancelado" || canceled.FinishedAt == nil {
		t.Fatalf("CancelFlowRun = %+v, %v", canceled, err)
	}
	for _, id := range []uuid.UUID{nr.ID, queued.ID} {
		if got, _ := s.GetNodeRun(ctx, id); got.Status != domain.NodeFailed || got.Error != "cancelado" {
			t.Fatalf("node %+v", got)
		}
	}
	if n, _ := s.PendingQuestions(ctx, nr.ID); n != 0 {
		t.Fatalf("%d questions still open", n)
	}
	rows, err := pool.Query(ctx, "SELECT id, status FROM jobs WHERE kind = 'agent.run'")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id uuid.UUID
		var status string
		if err := rows.Scan(&id, &status); err != nil {
			t.Fatal(err)
		}
		if want := map[bool]string{true: "queued", false: "failed"}[id == other]; status != want {
			t.Fatalf("job %s is %s, want %s", id, status, want)
		}
	}
	if busy, _ := s.CardHasActiveRun(ctx, run.CardID); busy {
		t.Fatal("the card is still held")
	}
	if _, err := s.CancelFlowRun(ctx, run.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("canceling twice: %v", err)
	}
	runs, err := s.ListCardFlowRuns(ctx, run.CardID)
	if err != nil || len(runs) != 1 || runs[0].FlowName != "dev" {
		t.Fatalf("ListCardFlowRuns = %+v, %v", runs, err)
	}
	if _, err := s.ListCardFlowRuns(cancelled(), run.CardID); err == nil {
		t.Error("ListCardFlowRuns: expected error")
	}
}

func TestCancelFailures(t *testing.T) {
	for _, method := range []string{"FailOpenNodeRuns", "CloseRunQuestions", "CancelRunJobs"} {
		s := newStore(t)
		run, _ := runningNode(t, s)
		failOn(s, method, 1)
		if _, err := s.CancelFlowRun(context.Background(), run.ID); !errors.Is(err, errInjected) {
			t.Fatalf("%s: got %v", method, err)
		}
	}
}
