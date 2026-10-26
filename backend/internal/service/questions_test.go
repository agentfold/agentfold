package service

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

// agentRun is a running node of kind typ with config cfg on card id.
func agentRun(typ domain.NodeType, cfg string) AgentRun {
	return AgentRun{
		NodeRun: domain.NodeRun{ID: uuid.New(), NodeKey: "dev", Iteration: 2},
		FlowRun: domain.FlowRun{ID: uuid.New(), CardID: id},
		Node:    domain.Node{ID: "dev", Type: typ, Config: json.RawMessage(cfg)},
	}
}

func TestAgentRun(t *testing.T) {
	s, repo, _ := newService()
	repo.nodeRun = domain.NodeRun{NodeKey: "dev"}
	repo.version = domain.FlowVersion{Graph: domain.Graph{Nodes: []domain.Node{{ID: "dev", Type: domain.NodeAgent}}}}
	run, err := s.AgentRun(ctx, "tok")
	sum := sha256.Sum256([]byte("tok"))
	if err != nil || run.Node.ID != "dev" || repo.ins["RunningNodeRunByToken"] != string(sum[:]) {
		t.Fatalf("AgentRun = %+v, %v", run, err)
	}

	repo.nodeRun.NodeKey = "gone"
	if _, err := s.AgentRun(ctx, "tok"); err == nil {
		t.Fatal("expected an error for a node missing from the version")
	}
	for _, method := range []string{"RunningNodeRunByToken", "GetFlowRun", "GetFlowVersion"} {
		s, repo, _ := newService()
		repo.errs = map[string]error{method: errRepo}
		if _, err := s.AgentRun(ctx, "tok"); !errors.Is(err, errRepo) {
			t.Fatalf("%s: got %v", method, err)
		}
	}
}

func TestAskHuman(t *testing.T) {
	s, repo, pub := newService()
	qs, err := s.AskHuman(ctx, agentRun(domain.NodeAgent, `{}`), []string{" Which DB? ", "  ", "Port?"})
	if err != nil || len(qs) != 2 || qs[0].Text != "Which DB?" {
		t.Fatalf("AskHuman = %+v, %v", qs, err)
	}
	want := []string{"question.created", "card_event.created", "question.created", "card_event.created"}
	if got := eventTypes(pub); !slices.Equal(got, want) {
		t.Fatalf("events %q", got)
	}
	if !slices.Contains(repo.calls, "AddCardEvent:question.asked") {
		t.Fatalf("calls %v", repo.calls)
	}

	var verr *domain.ValidationError
	if _, err := s.AskHuman(ctx, agentRun(domain.NodeAgent, `{}`), []string{" "}); !errors.As(err, &verr) {
		t.Fatalf("no questions: %v", err)
	}
	repo.errs = map[string]error{"AddQuestions": errRepo}
	if _, err := s.AskHuman(ctx, agentRun(domain.NodeAgent, `{}`), []string{"Q"}); !errors.Is(err, errRepo) {
		t.Fatalf("got %v", err)
	}
	repo.errs = map[string]error{"AddCardEvent:question.asked": errRepo}
	if _, err := s.AskHuman(ctx, agentRun(domain.NodeAgent, `{}`), []string{"Q"}); !errors.Is(err, errRepo) {
		t.Fatalf("timeline: got %v", err)
	}
}

func TestReportDone(t *testing.T) {
	s, repo, _ := newService()
	if err := s.ReportDone(ctx, agentRun(domain.NodeAgent, `{}`), " did it "); err != nil || repo.in != "did it" {
		t.Fatalf("got %v, %v", repo.in, err)
	}
	var verr *domain.ValidationError
	if err := s.ReportDone(ctx, agentRun(domain.NodeAgent, `{}`), " "); !errors.As(err, &verr) {
		t.Fatalf("empty summary: %v", err)
	}
}

func TestSubmitOutput(t *testing.T) {
	schema := `{"output_schema":{"type":"object","required":["n"],"properties":{"n":{"type":"integer"}}}}`
	s, repo, _ := newService()
	if err := s.SubmitOutput(ctx, agentRun(domain.NodeAgent, schema), json.RawMessage(`{"n": 1}`)); err != nil || repo.in != `{"n": 1}` {
		t.Fatalf("got %v, %v", repo.in, err)
	}
	var verr *domain.ValidationError
	if err := s.SubmitOutput(ctx, agentRun(domain.NodeAgent, schema), json.RawMessage(`{"n": "x"}`)); !errors.As(err, &verr) || verr.Field != "output" {
		t.Fatalf("invalid output: %v", err)
	}
	for _, run := range []AgentRun{agentRun(domain.NodeAgent, `{}`), agentRun(domain.NodeMoveCard, schema), agentRun(domain.NodeAgent, `[`)} {
		if err := s.SubmitOutput(ctx, run, json.RawMessage(`{}`)); !errors.Is(err, ErrNoOutputSchema) {
			t.Fatalf("%s %s: got %v", run.Node.Type, run.Node.Config, err)
		}
	}
}

func TestDecide(t *testing.T) {
	s, repo, _ := newService()
	gateway := agentRun(domain.NodeAgent, `{"gateway":true}`)
	if err := s.Decide(ctx, gateway, domain.DecisionRejected, " add tests "); err != nil || repo.in != "rejected:add tests" {
		t.Fatalf("reject: %v, %v", repo.in, err)
	}
	if err := s.Decide(ctx, gateway, domain.DecisionApproved, ""); err != nil || repo.in != "approved:" {
		t.Fatalf("approve: %v, %v", repo.in, err)
	}
	var verr *domain.ValidationError
	if err := s.Decide(ctx, gateway, domain.DecisionRejected, " "); !errors.As(err, &verr) || verr.Field != "feedback" {
		t.Fatalf("reject without feedback: %v", err)
	}
	for _, run := range []AgentRun{agentRun(domain.NodeAgent, `{}`), agentRun(domain.NodeMoveCard, `{"gateway":true}`), agentRun(domain.NodeAgent, `[`)} {
		if err := s.Decide(ctx, run, domain.DecisionApproved, ""); !errors.Is(err, ErrNotGateway) {
			t.Fatalf("%s %s: got %v", run.Node.Type, run.Node.Config, err)
		}
	}
	repo.errs = map[string]error{"SetNodeRunDecision": errRepo}
	if err := s.Decide(ctx, gateway, domain.DecisionApproved, ""); !errors.Is(err, errRepo) {
		t.Fatalf("got %v", err)
	}
}

func TestAnswerQuestion(t *testing.T) {
	s, repo, pub := newService()
	repo.question = domain.Question{ID: uuid.New(), CardID: id, NodeRunID: uuid.New(), Text: "DB?"}
	repo.resumed = true
	q, err := s.AnswerQuestion(ctx, repo.question.ID, " Postgres ")
	if err != nil || *q.Answer != "Postgres" {
		t.Fatalf("AnswerQuestion = %+v, %v", q, err)
	}
	if got := eventTypes(pub); !slices.Equal(got, []string{"question.updated", "card_event.created", "flow_run.updated"}) {
		t.Fatalf("events %q", got)
	}

	s, repo, pub = newService()
	if _, err := s.AnswerQuestion(ctx, id, "x"); err != nil || len(pub.events) != 2 {
		t.Fatalf("not resumed: %v, %v", err, pub.events)
	}
	var verr *domain.ValidationError
	if _, err := s.AnswerQuestion(ctx, id, " "); !errors.As(err, &verr) {
		t.Fatalf("empty answer: %v", err)
	}
	for _, method := range []string{"AnswerQuestion", "ResumeIfAnswered"} {
		repo.errs = map[string]error{method: errRepo}
		if _, err := s.AnswerQuestion(ctx, id, "x"); !errors.Is(err, errRepo) {
			t.Fatalf("%s: got %v", method, err)
		}
	}
}

func TestQuestionReads(t *testing.T) {
	s, repo, _ := newService()
	if _, err := s.ListCardQuestions(ctx, id); err != nil || !slices.Equal(repo.calls, []string{"GetCard", "ListCardQuestions"}) {
		t.Fatalf("calls %v, %v", repo.calls, err)
	}
	repo.errs = map[string]error{"GetCard": errRepo}
	if _, err := s.ListCardQuestions(ctx, id); !errors.Is(err, errRepo) {
		t.Fatalf("got %v", err)
	}

	s, repo, _ = newService()
	repo.flowRun = domain.FlowRun{ID: id}
	detail, err := s.GetRunDetail(ctx, id)
	if err != nil || detail.ID != id || len(detail.Nodes) != 1 {
		t.Fatalf("GetRunDetail = %+v, %v", detail, err)
	}
	repo.errs = map[string]error{"GetFlowRun": errRepo}
	if _, err := s.GetRunDetail(ctx, id); !errors.Is(err, errRepo) {
		t.Fatalf("got %v", err)
	}
}
