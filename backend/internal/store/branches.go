package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/store/sqlcgen"
)

func toBranch(b sqlcgen.FlowBranch) domain.Branch {
	out := domain.Branch{
		ID: b.ID, FlowRunID: b.FlowRunID, SplitNodeRunID: b.SplitNodeRunID, SplitKey: b.SplitKey,
		ParentID: b.ParentBranchID, Name: b.Name, Label: b.Label, Status: domain.BranchStatus(b.Status),
		Output: json.RawMessage(b.Output),
	}
	if b.Error != nil {
		out.Error = *b.Error
	}
	return out
}

// CreateBranchNodeRun records that a run entered a node in a branch (nil
// for the main path of the run).
func (s *Store) CreateBranchNodeRun(ctx context.Context, flowRunID uuid.UUID, branch *domain.Branch, nodeKey string, iteration int, input json.RawMessage) (domain.NodeRun, error) {
	if input == nil {
		input = json.RawMessage(`{}`)
	}
	params := sqlcgen.InsertNodeRunParams{
		FlowRunID: flowRunID, NodeKey: nodeKey, Iteration: int32(iteration), //nolint:gosec // small counter
		Status: string(domain.NodeQueued), Input: input,
	}
	if branch != nil {
		params.BranchID, params.Branch = &branch.ID, branch.Label
	}
	row, err := s.q.InsertNodeRun(ctx, params)
	return mapRow(row, err, toNodeRun)
}

// StartSplit records that the Split node run split opened a branch per
// name, nested in parent (nil on the main path of the run).
func (s *Store) StartSplit(ctx context.Context, split domain.NodeRun, parent *domain.Branch, names []string) ([]domain.Branch, error) {
	var out []domain.Branch
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		if err := q.InsertFlowSplit(ctx, sqlcgen.InsertFlowSplitParams{NodeRunID: split.ID, FlowRunID: split.FlowRunID}); err != nil {
			return mapErr(err)
		}
		prefix := ""
		var parentID *uuid.UUID
		if parent != nil {
			prefix, parentID = parent.Label+"/", &parent.ID
		}
		for _, name := range names {
			row, err := q.InsertFlowBranch(ctx, sqlcgen.InsertFlowBranchParams{
				FlowRunID: split.FlowRunID, SplitNodeRunID: split.ID, SplitKey: split.NodeKey,
				ParentBranchID: parentID, Name: name, Label: prefix + split.NodeKey + "." + name,
			})
			if err != nil {
				return mapErr(err)
			}
			out = append(out, toBranch(row))
		}
		return nil
	})
	return out, err
}

// GetBranch returns a branch.
func (s *Store) GetBranch(ctx context.Context, id uuid.UUID) (domain.Branch, error) {
	row, err := s.q.GetFlowBranch(ctx, id)
	return mapRow(row, err, toBranch)
}

// SettleBranch ends an active branch and tells whether that settles its
// split: decide gets the branches of the split. Settling cancels the
// branches still active, and those nested in them, failing their open
// node runs. Nothing happens to a branch that ended already or whose split
// settled; the settlement then reports false.
func (s *Store) SettleBranch(ctx context.Context, id uuid.UUID, end domain.BranchEnd, decide func([]domain.Branch) bool) (domain.Settlement, error) {
	var out domain.Settlement
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		row, err := q.GetFlowBranch(ctx, id)
		if err != nil {
			return mapErr(err)
		}
		split, err := q.GetFlowSplitForUpdate(ctx, row.SplitNodeRunID)
		if err != nil || split.SettledAt != nil {
			return mapErr(err)
		}
		ended, err := q.EndFlowBranch(ctx, sqlcgen.EndFlowBranchParams{
			ID: id, Status: string(end.Status), Output: end.Output, Error: optional(end.Error),
		})
		if err != nil || ended == 0 {
			return err
		}
		if out.Branches, err = splitBranches(ctx, q, row.SplitNodeRunID); err != nil || !decide(out.Branches) {
			return err
		}
		if err := q.SettleFlowSplit(ctx, row.SplitNodeRunID); err != nil {
			return err
		}
		canceled, err := q.CancelSplitBranches(ctx, row.SplitNodeRunID)
		if err != nil {
			return err
		}
		if err := q.FailBranchNodeRuns(ctx, canceled); err != nil {
			return err
		}
		if out.Branches, err = splitBranches(ctx, q, row.SplitNodeRunID); err != nil {
			return err
		}
		out.Settled = true
		if row.ParentBranchID != nil {
			parent, err := q.GetFlowBranch(ctx, *row.ParentBranchID)
			if err != nil {
				return err
			}
			b := toBranch(parent)
			out.Parent = &b
		}
		return nil
	})
	return out, err
}

func splitBranches(ctx context.Context, q sqlcgen.Querier, split uuid.UUID) ([]domain.Branch, error) {
	rows, err := q.ListSplitBranches(ctx, split)
	return mapRows(rows, err, toBranch)
}
