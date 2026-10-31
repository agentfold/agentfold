package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/store/sqlcgen"
)

// AwaitApproval holds a HumanReview node run that approves the output of
// the node before, and its flow run, until the human decides. ErrNotFound
// when the node run or its flow run ended meanwhile.
func (s *Store) AwaitApproval(ctx context.Context, nodeRunID uuid.UUID) (domain.NodeRun, error) {
	var nr domain.NodeRun
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		row, err := q.AwaitReviewNodeRun(ctx, nodeRunID)
		if err != nil {
			return mapErr(err)
		}
		nr = toNodeRun(row)
		_, err = q.SetFlowRunStatus(ctx, sqlcgen.SetFlowRunStatusParams{
			ID: row.FlowRunID, Status: string(domain.RunWaiting), FromStatus: string(domain.RunRunning),
		})
		return mapErr(err)
	})
	return nr, err
}

// DecideApproval records how the human decided the output approval of a
// node run and ends it: approved hands on what the node received, and
// changes_requested sends text to the agent the node leads to.
// ErrReviewClosed when the node run awaits no decision.
func (s *Store) DecideApproval(ctx context.Context, nodeRunID uuid.UUID, decision, text string) (domain.NodeRun, error) {
	var nr domain.NodeRun
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		row, err := q.GetNodeRunForUpdate(ctx, nodeRunID)
		if err != nil {
			return mapErr(err)
		}
		if row.Status != string(domain.NodeAwaitingReview) {
			return domain.ErrReviewClosed
		}
		if _, err := q.SetNodeRunDecision(ctx, sqlcgen.SetNodeRunDecisionParams{ID: row.ID, Decision: &decision, Feedback: optional(text)}); err != nil {
			return err
		}
		result := domain.NodeResult{Status: domain.NodeDone, OutputHandle: "approved", Output: row.Input}
		if decision == domain.DecisionChangesRequested {
			output, _ := json.Marshal(map[string]string{"result": domain.ApprovalPrompt(text), "feedback": text}) // strings marshal
			result = domain.NodeResult{Status: domain.NodeDone, OutputHandle: "changes_requested", Output: output}
		}
		nr, err = decide(ctx, q, row, result)
		return err
	})
	return nr, err
}
