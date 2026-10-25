package store

import (
	"context"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/store/sqlcgen"
)

func toNodeOutput(r sqlcgen.NodeRunOutput) domain.NodeOutput {
	o := domain.NodeOutput{
		ID: r.ID, NodeRunID: r.NodeRunID, Kind: domain.OutputKind(r.Kind), Text: r.Text, IsError: r.IsError, CreatedAt: r.CreatedAt,
	}
	if r.Tool != nil {
		o.Tool = *r.Tool
	}
	return o
}

// AddNodeOutput records what an agent did while running a node.
func (s *Store) AddNodeOutput(ctx context.Context, nodeRunID uuid.UUID, entries []domain.NodeOutput) error {
	for _, e := range entries {
		_, err := s.q.InsertNodeOutput(ctx, sqlcgen.InsertNodeOutputParams{
			NodeRunID: nodeRunID, Kind: string(e.Kind), Text: e.Text, Tool: optional(e.Tool), IsError: e.IsError,
		})
		if err != nil {
			return mapErr(err)
		}
	}
	return nil
}

// ListNodeOutput returns what an agent did while running a node, in order.
func (s *Store) ListNodeOutput(ctx context.Context, nodeRunID uuid.UUID) ([]domain.NodeOutput, error) {
	rows, err := s.q.ListNodeOutput(ctx, nodeRunID)
	return mapRows(rows, err, toNodeOutput)
}

// ListCardFlowRuns returns the runs of a card, newest first.
func (s *Store) ListCardFlowRuns(ctx context.Context, cardID uuid.UUID) ([]domain.FlowRun, error) {
	rows, err := s.q.ListCardFlowRuns(ctx, cardID)
	return mapRows(rows, err, func(r sqlcgen.ListCardFlowRunsRow) domain.FlowRun { return namedRun(r.FlowRun, r.FlowName) })
}

// CancelFlowRun ends an active run: its open nodes fail, its open questions
// close and the jobs of its agents stop. ErrNotFound when it is not active.
func (s *Store) CancelFlowRun(ctx context.Context, id uuid.UUID) (domain.FlowRun, error) {
	var run domain.FlowRun
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		row, err := q.CancelFlowRun(ctx, id)
		if err != nil {
			return mapErr(err)
		}
		run = toFlowRun(row)
		if err := q.FailOpenNodeRuns(ctx, id); err != nil {
			return err
		}
		if err := q.CloseRunQuestions(ctx, id); err != nil {
			return err
		}
		return q.CancelRunJobs(ctx, id)
	})
	return run, err
}
