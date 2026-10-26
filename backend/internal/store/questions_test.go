package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

// runningNode starts a flow run and a running node run in it.
func runningNode(t *testing.T, s *Store) (domain.FlowRun, domain.NodeRun) {
	t.Helper()
	ctx := context.Background()
	run, _ := publishedRun(t, s)
	nr, err := s.CreateNodeRun(ctx, run.ID, "dev", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if nr, err = s.StartNodeRun(ctx, nr.ID, uuid.New(), []byte("token-hash")); err != nil {
		t.Fatal(err)
	}
	return run, nr
}

func TestNodeRunReports(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	run, nr := runningNode(t, s)
	if got, err := s.RunningNodeRunByToken(ctx, []byte("token-hash")); err != nil || got.ID != nr.ID {
		t.Fatalf("RunningNodeRunByToken = %+v, %v", got, err)
	}
	if got, err := s.SetNodeRunSummary(ctx, nr.ID, "did it"); err != nil || got.Summary != "did it" {
		t.Fatalf("SetNodeRunSummary = %+v, %v", got, err)
	}
	if got, err := s.SetNodeRunOutput(ctx, nr.ID, json.RawMessage(`{"a": 1}`)); err != nil || string(got.StructuredOutput) != `{"a": 1}` {
		t.Fatalf("SetNodeRunOutput = %s, %v", got.StructuredOutput, err)
	}
	usage := domain.Usage{TokensIn: 1, TokensOut: 2, CacheRead: 3, CacheWrite: 4, CostUSD: 0.5, Turns: 1, DurationMS: 10}
	for range 2 {
		if _, err := s.FinishNodeRun(ctx, nr.ID, domain.NodeResult{Status: domain.NodeAwaitingInput, Usage: usage}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.GetNodeRun(ctx, nr.ID)
	if err != nil || got.TokensIn != 2 || got.CostUSD != 1 || got.Turns != 2 || got.Status != domain.NodeAwaitingInput {
		t.Fatalf("usage adds up: %+v, %v", got, err)
	}
	if _, err := s.RunningNodeRunByToken(ctx, []byte("token-hash")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("token after the run: %v", err)
	}
	nodes, err := s.ListNodeRuns(ctx, run.ID)
	if err != nil || len(nodes) != 1 || nodes[0].ID != nr.ID {
		t.Fatalf("ListNodeRuns = %+v, %v", nodes, err)
	}
	if _, err := s.ListNodeRuns(cancelled(), run.ID); err == nil {
		t.Error("ListNodeRuns: expected error")
	}

	waiting, err := s.WaitFlowRun(ctx, run.ID)
	if err != nil || waiting.Status != domain.RunWaiting {
		t.Fatalf("WaitFlowRun = %+v, %v", waiting, err)
	}
	if _, err := s.WaitFlowRun(ctx, run.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("waiting twice: %v", err)
	}
}

func TestQuestions(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	run, nr := runningNode(t, s)
	qs, err := s.AddQuestions(ctx, run.CardID, nr.ID, []string{"Which database?", "Which port?"})
	if err != nil || len(qs) != 2 || qs[0].Answer != nil {
		t.Fatalf("AddQuestions = %+v, %v", qs, err)
	}
	if n, err := s.PendingQuestions(ctx, nr.ID); err != nil || n != 2 {
		t.Fatalf("PendingQuestions = %d, %v", n, err)
	}
	// Still running: answers wait for the agent to end its turn.
	if _, err := s.AnswerQuestion(ctx, qs[0].ID, "Postgres"); err != nil {
		t.Fatal(err)
	}
	if _, resumed, err := s.ResumeIfAnswered(ctx, nr.ID); err != nil || resumed {
		t.Fatalf("resume while running = %v, %v", resumed, err)
	}
	if _, err := s.FinishNodeRun(ctx, nr.ID, domain.NodeResult{Status: domain.NodeAwaitingInput}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitFlowRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, resumed, err := s.ResumeIfAnswered(ctx, nr.ID); err != nil || resumed {
		t.Fatalf("resume with a question open = %v, %v", resumed, err)
	}
	if _, err := s.AnswerQuestion(ctx, qs[1].ID, "5432"); err != nil {
		t.Fatal(err)
	}
	fr, resumed, err := s.ResumeIfAnswered(ctx, nr.ID)
	if err != nil || !resumed || fr.Status != domain.RunRunning {
		t.Fatalf("resume = %+v, %v, %v", fr, resumed, err)
	}
	if got, _ := s.GetNodeRun(ctx, nr.ID); got.Status != domain.NodeQueued {
		t.Fatalf("node %+v", got)
	}
	var payload domain.AgentJob
	if err := pool.QueryRow(ctx, "SELECT payload FROM jobs WHERE kind = $1", domain.JobAgentRun).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.NodeRunID != nr.ID || payload.Prompt != domain.AnswersPrompt([]domain.Question{withAnswer(qs[0], "Postgres"), withAnswer(qs[1], "5432")}) {
		t.Fatalf("job payload %+v", payload)
	}
	if n, err := s.PendingQuestions(ctx, nr.ID); err != nil || n != 0 {
		t.Fatalf("PendingQuestions after = %d, %v", n, err)
	}
	all, err := s.ListCardQuestions(ctx, run.CardID)
	if err != nil || len(all) != 2 || all[1].DeliveredAt == nil || *all[1].Answer != "5432" {
		t.Fatalf("ListCardQuestions = %+v, %v", all, err)
	}
	if _, err := s.AnswerQuestion(ctx, qs[0].ID, "MySQL"); !errors.Is(err, domain.ErrQuestionClosed) {
		t.Fatalf("answering a delivered question: %v", err)
	}
	if _, err := s.AnswerQuestion(ctx, uuid.New(), "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("answering an unknown question: %v", err)
	}
	if _, _, err := s.ResumeIfAnswered(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("resuming an unknown node: %v", err)
	}
	if _, err := s.AnswerQuestion(cancelled(), qs[0].ID, "x"); err == nil {
		t.Error("AnswerQuestion: expected error")
	}
	if _, err := s.ListCardQuestions(cancelled(), run.CardID); err == nil {
		t.Error("ListCardQuestions: expected error")
	}
	if _, err := s.PendingQuestions(cancelled(), nr.ID); err == nil {
		t.Error("PendingQuestions: expected error")
	}
}

func withAnswer(q domain.Question, answer string) domain.Question {
	q.Answer = &answer
	return q
}

// waitingNode is a node run awaiting the answer to one answered question.
func waitingNode(t *testing.T, s *Store) domain.NodeRun {
	t.Helper()
	ctx := context.Background()
	run, nr := runningNode(t, s)
	qs, err := s.AddQuestions(ctx, run.CardID, nr.ID, []string{"Q?"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AnswerQuestion(ctx, qs[0].ID, "A"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishNodeRun(ctx, nr.ID, domain.NodeResult{Status: domain.NodeAwaitingInput}); err != nil {
		t.Fatal(err)
	}
	return nr
}

func TestResumeFailures(t *testing.T) {
	for _, method := range []string{"GetNodeRunForUpdate", "ListUndeliveredQuestions", "DeliverQuestions", "QueueNodeRun", "SetFlowRunStatus", "InsertJob"} {
		t.Run(method, func(t *testing.T) {
			s := newStore(t)
			nr := waitingNode(t, s)
			if _, err := s.WaitFlowRun(context.Background(), nr.FlowRunID); err != nil {
				t.Fatal(err)
			}
			failOn(s, method, 1)
			if _, resumed, err := s.ResumeIfAnswered(context.Background(), nr.ID); !errors.Is(err, errInjected) || resumed {
				t.Fatalf("got %v, %v", resumed, err)
			}
		})
	}
	t.Run("run not waiting", func(t *testing.T) {
		s := newStore(t)
		nr := waitingNode(t, s)
		if _, _, err := s.ResumeIfAnswered(context.Background(), nr.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("AddQuestions", func(t *testing.T) {
		s := newStore(t)
		run, nr := runningNode(t, s)
		failOn(s, "InsertQuestion", 1)
		if _, err := s.AddQuestions(context.Background(), run.CardID, nr.ID, []string{"Q?"}); !errors.Is(err, errInjected) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestSetNodeRunDecision(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, nr := runningNode(t, s)
	got, err := s.SetNodeRunDecision(ctx, nr.ID, domain.DecisionRejected, "add tests")
	if err != nil || got.Decision != domain.DecisionRejected || got.Feedback != "add tests" {
		t.Fatalf("SetNodeRunDecision = %+v, %v", got, err)
	}
	if got, err = s.SetNodeRunDecision(ctx, nr.ID, domain.DecisionApproved, ""); err != nil || got.Decision != domain.DecisionApproved || got.Feedback != "" {
		t.Fatalf("a later decision replaces it: %+v, %v", got, err)
	}
}
