package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/events"
)

// AgentRun is a running node as its agent reaches the board tools.
type AgentRun struct {
	NodeRun domain.NodeRun
	FlowRun domain.FlowRun
	Node    domain.Node
}

// ErrNoOutputSchema means submit_output was called by a node without one.
var ErrNoOutputSchema = errors.New("this node has no output schema: call report_done instead")

// ErrNotReview means submit_review was called by a node that is no review.
var ErrNotReview = errors.New("only review nodes submit reviews")

// AgentRun finds the running node a board tool token belongs to.
func (s *Service) AgentRun(ctx context.Context, token string) (AgentRun, error) {
	sum := sha256.Sum256([]byte(token))
	nr, err := s.repo.RunningNodeRunByToken(ctx, sum[:])
	if err != nil {
		return AgentRun{}, err
	}
	fr, err := s.repo.GetFlowRun(ctx, nr.FlowRunID)
	if err != nil {
		return AgentRun{}, err
	}
	version, err := s.repo.GetFlowVersion(ctx, fr.FlowVersionID)
	if err != nil {
		return AgentRun{}, err
	}
	for _, n := range version.Graph.Nodes {
		if n.ID == nr.NodeKey {
			return AgentRun{NodeRun: nr, FlowRun: fr, Node: n}, nil
		}
	}
	return AgentRun{}, fmt.Errorf("node %q not found in the flow version", nr.NodeKey)
}

// AskHuman records the questions of an agent on its card. The node waits
// for the answers once the agent ends its turn.
func (s *Service) AskHuman(ctx context.Context, run AgentRun, texts []string) ([]domain.Question, error) {
	clean := make([]string, 0, len(texts))
	for _, t := range texts {
		if t = strings.TrimSpace(t); t != "" {
			clean = append(clean, t)
		}
	}
	if len(clean) == 0 {
		return nil, &domain.ValidationError{Field: "questions", Message: "ask at least one question"}
	}
	questions, err := s.repo.AddQuestions(ctx, run.FlowRun.CardID, run.NodeRun.ID, clean)
	if err != nil {
		return nil, err
	}
	for _, q := range questions {
		s.pub.Publish(events.Event{Type: "question.created", ID: q.ID, Data: q})
		err = errors.Join(err, s.timeline(ctx, q.CardID, domain.QuestionAsked, questionPayload{
			QuestionID: q.ID, NodeKey: run.NodeRun.NodeKey, Text: q.Text,
		}))
	}
	return questions, err
}

// ReportDone records the summary of an agent's work.
func (s *Service) ReportDone(ctx context.Context, run AgentRun, summary string) error {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return &domain.ValidationError{Field: "summary", Message: "is required"}
	}
	_, err := s.repo.SetNodeRunSummary(ctx, run.NodeRun.ID, summary)
	return err
}

// SubmitOutput records the structured output of an agent node, which must
// match the node schema.
func (s *Service) SubmitOutput(ctx context.Context, run AgentRun, output json.RawMessage) error {
	var cfg domain.AgentConfig
	if run.Node.Type != domain.NodeAgent || json.Unmarshal(run.Node.Config, &cfg) != nil || !cfg.HasOutputSchema() {
		return ErrNoOutputSchema
	}
	if err := domain.ValidateOutput(cfg.OutputSchema, output); err != nil {
		return &domain.ValidationError{Field: "output", Message: err.Error()}
	}
	_, err := s.repo.SetNodeRunOutput(ctx, run.NodeRun.ID, output)
	return err
}

// SubmitReview records the verdict of a review node: approved, or the
// comments the developer must address.
func (s *Service) SubmitReview(ctx context.Context, run AgentRun, approved bool, comments []domain.ReviewInput) error {
	if run.Node.Type != domain.NodeReview {
		return ErrNotReview
	}
	for i := range comments {
		if comments[i].Body = strings.TrimSpace(comments[i].Body); comments[i].Body == "" {
			return &domain.ValidationError{Field: "comments", Message: "every comment needs a body"}
		}
	}
	if !approved && len(comments) == 0 {
		return &domain.ValidationError{Field: "comments", Message: "say what must change when not approving"}
	}
	saved, err := s.repo.SubmitReview(ctx, run.NodeRun, run.FlowRun.CardID, approved, comments)
	if err != nil {
		return err
	}
	return s.timeline(ctx, run.FlowRun.CardID, domain.ReviewSubmitted, reviewPayload{
		NodeKey: run.NodeRun.NodeKey, Round: run.NodeRun.Iteration, Approved: approved, Comments: toCommentPayloads(saved),
	})
}

// ListCardQuestions returns the questions asked on a card, oldest first.
func (s *Service) ListCardQuestions(ctx context.Context, cardID uuid.UUID) ([]domain.Question, error) {
	if _, err := s.repo.GetCard(ctx, cardID); err != nil {
		return nil, err
	}
	return s.repo.ListCardQuestions(ctx, cardID)
}

// AnswerQuestion records the answer to a question. Once every question its
// agent asked is answered, the agent resumes with the answers.
func (s *Service) AnswerQuestion(ctx context.Context, id uuid.UUID, answer string) (domain.Question, error) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return domain.Question{}, &domain.ValidationError{Field: "answer", Message: "is required"}
	}
	q, err := s.repo.AnswerQuestion(ctx, id, answer)
	if err != nil {
		return domain.Question{}, err
	}
	s.pub.Publish(events.Event{Type: "question.updated", ID: q.ID, Data: q})
	err = s.timeline(ctx, q.CardID, domain.QuestionAnswered, questionPayload{QuestionID: q.ID, Text: q.Text, Answer: answer})
	fr, resumed, resumeErr := s.repo.ResumeIfAnswered(ctx, q.NodeRunID)
	if resumed {
		s.pub.Publish(events.Event{Type: "flow_run.updated", ID: fr.ID, Data: fr})
	}
	return q, errors.Join(err, resumeErr)
}

// timeline adds an entry to a card timeline and announces it.
func (s *Service) timeline(ctx context.Context, cardID uuid.UUID, typ domain.CardEventType, payload any) error {
	event, err := s.repo.AddCardEvent(ctx, cardID, typ, payload)
	if err == nil {
		s.pub.Publish(events.Event{Type: "card_event.created", ID: event.ID, Data: event})
	}
	return err
}

// Timeline payloads.
type (
	questionPayload struct {
		QuestionID uuid.UUID `json:"question_id"`
		NodeKey    string    `json:"node_key,omitempty"`
		Text       string    `json:"text"`
		Answer     string    `json:"answer,omitempty"`
	}
	commentPayload struct {
		File string `json:"file,omitempty"`
		Line *int   `json:"line,omitempty"`
		Body string `json:"body"`
	}
	reviewPayload struct {
		NodeKey  string           `json:"node_key"`
		Round    int              `json:"round"`
		Approved bool             `json:"approved"`
		Comments []commentPayload `json:"comments"`
	}
)

func toCommentPayloads(comments []domain.ReviewComment) []commentPayload {
	out := make([]commentPayload, len(comments))
	for i, c := range comments {
		out[i] = commentPayload{File: c.File, Line: c.Line, Body: c.Body}
	}
	return out
}
