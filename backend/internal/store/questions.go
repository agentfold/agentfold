package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orquestrator/internal/domain"
	"orquestrator/internal/store/sqlcgen"
)

func toQuestion(r sqlcgen.Question) domain.Question {
	return domain.Question{
		ID:          r.ID,
		CardID:      r.CardID,
		NodeRunID:   r.NodeRunID,
		Text:        r.Text,
		Answer:      r.Answer,
		AnsweredAt:  r.AnsweredAt,
		DeliveredAt: r.DeliveredAt,
		CreatedAt:   r.CreatedAt,
	}
}

func toReviewComment(r sqlcgen.ReviewComment) domain.ReviewComment {
	c := domain.ReviewComment{
		ID:        r.ID,
		CardID:    r.CardID,
		NodeRunID: r.NodeRunID,
		Round:     int(r.Round),
		Body:      r.Body,
		CreatedAt: r.CreatedAt,
	}
	if r.File != nil {
		c.File = *r.File
	}
	if r.Line != nil {
		line := int(*r.Line)
		c.Line = &line
	}
	return c
}

// AddQuestions records the questions an agent asked while running a node.
func (s *Store) AddQuestions(ctx context.Context, cardID, nodeRunID uuid.UUID, texts []string) ([]domain.Question, error) {
	out := make([]domain.Question, 0, len(texts))
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		for _, text := range texts {
			row, err := q.InsertQuestion(ctx, sqlcgen.InsertQuestionParams{CardID: cardID, NodeRunID: nodeRunID, Text: text})
			if err != nil {
				return mapErr(err)
			}
			out = append(out, toQuestion(row))
		}
		return nil
	})
	return out, err
}

// GetQuestion returns one question.
func (s *Store) GetQuestion(ctx context.Context, id uuid.UUID) (domain.Question, error) {
	row, err := s.q.GetQuestion(ctx, id)
	return mapRow(row, err, toQuestion)
}

// ListCardQuestions returns the questions asked on a card, oldest first.
func (s *Store) ListCardQuestions(ctx context.Context, cardID uuid.UUID) ([]domain.Question, error) {
	rows, err := s.q.ListCardQuestions(ctx, cardID)
	return mapRows(rows, err, toQuestion)
}

// AnswerQuestion records the answer to a question. Questions whose answers
// were already delivered to the agent cannot change: ErrQuestionClosed.
func (s *Store) AnswerQuestion(ctx context.Context, id uuid.UUID, answer string) (domain.Question, error) {
	row, err := s.q.AnswerQuestion(ctx, sqlcgen.AnswerQuestionParams{ID: id, Answer: &answer})
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := s.GetQuestion(ctx, id); err != nil {
			return domain.Question{}, err
		}
		return domain.Question{}, domain.ErrQuestionClosed
	}
	return mapRow(row, err, toQuestion)
}

// ResumeIfAnswered resumes a node run waiting for answers once every
// question it asked is answered: it hands the answers to a new agent.run
// job and sets the node and flow runs running again. It reports false, doing
// nothing, when the node is not waiting or some question has no answer yet.
func (s *Store) ResumeIfAnswered(ctx context.Context, nodeRunID uuid.UUID) (domain.FlowRun, bool, error) {
	var run domain.FlowRun
	resumed := false
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		nr, err := q.GetNodeRunForUpdate(ctx, nodeRunID)
		if err != nil {
			return mapErr(err)
		}
		if nr.Status != string(domain.NodeAwaitingInput) {
			return nil
		}
		rows, err := q.ListUndeliveredQuestions(ctx, nodeRunID)
		if err != nil {
			return err
		}
		questions := make([]domain.Question, len(rows))
		for i, r := range rows {
			if r.Answer == nil {
				return nil
			}
			questions[i] = toQuestion(r)
		}
		if err := q.DeliverQuestions(ctx, nodeRunID); err != nil {
			return err
		}
		if err := q.QueueNodeRun(ctx, nodeRunID); err != nil {
			return err
		}
		row, err := q.SetFlowRunStatus(ctx, sqlcgen.SetFlowRunStatusParams{
			ID: nr.FlowRunID, Status: string(domain.RunRunning), FromStatus: string(domain.RunWaiting),
		})
		if err != nil {
			return mapErr(err)
		}
		run = toFlowRun(row)
		_, err = insertJob(ctx, q, NewJob{
			Kind:        domain.JobAgentRun,
			Payload:     domain.AgentJob{NodeRunID: nodeRunID, Prompt: domain.AnswersPrompt(questions)},
			MaxAttempts: 2,
		})
		resumed = err == nil
		return err
	})
	return run, resumed, err
}

// PendingQuestions counts the questions of a node run not yet delivered to
// its agent.
func (s *Store) PendingQuestions(ctx context.Context, nodeRunID uuid.UUID) (int, error) {
	rows, err := s.q.ListUndeliveredQuestions(ctx, nodeRunID)
	return len(rows), err
}

// SubmitReview records the verdict of a review node run, replacing what it
// submitted before.
func (s *Store) SubmitReview(ctx context.Context, nr domain.NodeRun, cardID uuid.UUID, approved bool, comments []domain.ReviewInput) ([]domain.ReviewComment, error) {
	out := make([]domain.ReviewComment, 0, len(comments))
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		if err := q.DeleteReviewComments(ctx, nr.ID); err != nil {
			return err
		}
		for _, c := range comments {
			var line *int32
			if c.Line != nil {
				l := int32(*c.Line) //nolint:gosec // line numbers are small
				line = &l
			}
			row, err := q.InsertReviewComment(ctx, sqlcgen.InsertReviewCommentParams{
				CardID: cardID, NodeRunID: nr.ID, Round: int32(nr.Iteration), //nolint:gosec // small counter
				File: optional(c.File), Line: line, Body: c.Body,
			})
			if err != nil {
				return mapErr(err)
			}
			out = append(out, toReviewComment(row))
		}
		_, err := q.SetNodeRunReview(ctx, sqlcgen.SetNodeRunReviewParams{ID: nr.ID, ReviewApproved: &approved})
		return err
	})
	return out, err
}

// ListReviewComments returns the comments of a review node run.
func (s *Store) ListReviewComments(ctx context.Context, nodeRunID uuid.UUID) ([]domain.ReviewComment, error) {
	rows, err := s.q.ListReviewComments(ctx, nodeRunID)
	return mapRows(rows, err, toReviewComment)
}
