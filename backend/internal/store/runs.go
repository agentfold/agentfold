package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orquestrator/internal/domain"
	"orquestrator/internal/store/sqlcgen"
)

func toFlowRun(r sqlcgen.FlowRun) domain.FlowRun {
	run := domain.FlowRun{
		ID:            r.ID,
		FlowID:        r.FlowID,
		FlowVersionID: r.FlowVersionID,
		CardID:        r.CardID,
		Trigger:       r.Trigger,
		Status:        domain.RunStatus(r.Status),
		StartedAt:     r.StartedAt,
		FinishedAt:    r.FinishedAt,
	}
	if r.Error != nil {
		run.Error = *r.Error
	}
	return run
}

// namedRun converts a run read with the name of its flow.
func namedRun(r sqlcgen.FlowRun, flowName string) domain.FlowRun {
	run := toFlowRun(r)
	run.FlowName = flowName
	return run
}

func toNodeRun(r sqlcgen.NodeRun) domain.NodeRun {
	n := domain.NodeRun{
		ID:        r.ID,
		FlowRunID: r.FlowRunID,
		NodeKey:   r.NodeKey,
		Iteration: int(r.Iteration),
		Status:    domain.NodeStatus(r.Status),
		Input:     r.Input,
		Output:    r.Output,
		SessionID: r.SessionID,
		Usage: domain.Usage{
			TokensIn:   r.TokensIn,
			TokensOut:  r.TokensOut,
			CacheRead:  r.CacheRead,
			CacheWrite: r.CacheWrite,
			CostUSD:    r.CostUsd,
			Turns:      int(r.Turns),
			DurationMS: r.DurationMs,
		},
		CreatedAt:  r.CreatedAt,
		StartedAt:  r.StartedAt,
		FinishedAt: r.FinishedAt,
	}
	if r.OutputHandle != nil {
		n.OutputHandle = *r.OutputHandle
	}
	if r.Error != nil {
		n.Error = *r.Error
	}
	return n
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// StartFlowRun starts a run of a flow version for a card. It reports false,
// starting nothing, when the flow is already running for the card.
func (s *Store) StartFlowRun(ctx context.Context, version domain.FlowVersion, cardID uuid.UUID, trigger string) (domain.FlowRun, bool, error) {
	row, err := s.q.InsertFlowRun(ctx, sqlcgen.InsertFlowRunParams{
		FlowID: version.FlowID, FlowVersionID: version.ID, CardID: cardID, Trigger: trigger,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FlowRun{}, false, nil
	}
	run, err := mapRow(row, err, toFlowRun)
	return run, err == nil, err
}

// GetFlowRun returns one flow run.
func (s *Store) GetFlowRun(ctx context.Context, id uuid.UUID) (domain.FlowRun, error) {
	row, err := s.q.GetFlowRun(ctx, id)
	return mapRow(row, err, func(r sqlcgen.GetFlowRunRow) domain.FlowRun { return namedRun(r.FlowRun, r.FlowName) })
}

// ListLatestFlowRuns returns one run per card that ever ran a flow: an
// active one if there is any, else the last one.
func (s *Store) ListLatestFlowRuns(ctx context.Context) ([]domain.FlowRun, error) {
	rows, err := s.q.ListLatestFlowRuns(ctx)
	return mapRows(rows, err, func(r sqlcgen.ListLatestFlowRunsRow) domain.FlowRun { return namedRun(r.FlowRun, r.FlowName) })
}

// FinishFlowRun ends an active run; ErrNotFound when it is not active.
func (s *Store) FinishFlowRun(ctx context.Context, id uuid.UUID, status domain.RunStatus, msg string) (domain.FlowRun, error) {
	row, err := s.q.FinishFlowRun(ctx, sqlcgen.FinishFlowRunParams{ID: id, Status: string(status), Error: optional(msg)})
	return mapRow(row, err, toFlowRun)
}

// CardHasActiveRun reports whether a flow is running for the card.
func (s *Store) CardHasActiveRun(ctx context.Context, cardID uuid.UUID) (bool, error) {
	return s.q.CardHasActiveRun(ctx, cardID)
}

// RunnableFlows returns the last published version of every enabled flow.
func (s *Store) RunnableFlows(ctx context.Context) ([]domain.RunnableFlow, error) {
	rows, err := s.q.ListRunnableFlows(ctx)
	return mapParsed(rows, err, func(r sqlcgen.ListRunnableFlowsRow) (domain.RunnableFlow, error) {
		version, err := toFlowVersion(r.FlowVersion)
		return domain.RunnableFlow{Name: r.FlowName, Version: version}, err
	})
}

// GetFlowVersion returns one published version.
func (s *Store) GetFlowVersion(ctx context.Context, id uuid.UUID) (domain.FlowVersion, error) {
	row, err := s.q.GetFlowVersion(ctx, id)
	if err != nil {
		return domain.FlowVersion{}, mapErr(err)
	}
	return toFlowVersion(row)
}

// CreateNodeRun queues the execution of a node.
func (s *Store) CreateNodeRun(ctx context.Context, flowRunID uuid.UUID, nodeKey string, iteration int, input json.RawMessage) (domain.NodeRun, error) {
	if input == nil {
		input = json.RawMessage(`{}`)
	}
	row, err := s.q.InsertNodeRun(ctx, sqlcgen.InsertNodeRunParams{
		FlowRunID: flowRunID, NodeKey: nodeKey, Iteration: int32(iteration), //nolint:gosec // small counter
		Status: string(domain.NodeQueued), Input: input,
	})
	return mapRow(row, err, toNodeRun)
}

// GetNodeRun returns one node run.
func (s *Store) GetNodeRun(ctx context.Context, id uuid.UUID) (domain.NodeRun, error) {
	row, err := s.q.GetNodeRun(ctx, id)
	return mapRow(row, err, toNodeRun)
}

// CountNodeRuns tells how many times a node was entered in a run.
func (s *Store) CountNodeRuns(ctx context.Context, flowRunID uuid.UUID, nodeKey string) (int, error) {
	n, err := s.q.CountNodeRuns(ctx, sqlcgen.CountNodeRunsParams{FlowRunID: flowRunID, NodeKey: nodeKey})
	return int(n), err
}

// ContinueNodeRun claims the right to follow the output of a finished node
// run. It reports false when the run is not finished or was followed already.
func (s *Store) ContinueNodeRun(ctx context.Context, id uuid.UUID) (domain.NodeRun, bool, error) {
	row, err := s.q.ContinueNodeRun(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NodeRun{}, false, nil
	}
	run, err := mapRow(row, err, toNodeRun)
	return run, err == nil, err
}

// StartNodeRun marks a node run as running in an agent session.
func (s *Store) StartNodeRun(ctx context.Context, id, sessionID uuid.UUID) (domain.NodeRun, error) {
	row, err := s.q.StartNodeRun(ctx, sqlcgen.StartNodeRunParams{ID: id, SessionID: &sessionID})
	return mapRow(row, err, toNodeRun)
}

// FinishNodeRun records how a node run ended.
func (s *Store) FinishNodeRun(ctx context.Context, id uuid.UUID, r domain.NodeResult) (domain.NodeRun, error) {
	row, err := s.q.FinishNodeRun(ctx, sqlcgen.FinishNodeRunParams{
		ID:           id,
		Status:       string(r.Status),
		Output:       r.Output,
		OutputHandle: optional(r.OutputHandle),
		Error:        optional(r.Error),
		TokensIn:     r.TokensIn,
		TokensOut:    r.TokensOut,
		CacheRead:    r.CacheRead,
		CacheWrite:   r.CacheWrite,
		CostUsd:      r.CostUSD,
		Turns:        int32(r.Turns), //nolint:gosec // turn counts are small
		DurationMs:   r.DurationMS,
	})
	return mapRow(row, err, toNodeRun)
}

// AgentSession returns the Claude session of an agent node in a run.
func (s *Store) AgentSession(ctx context.Context, flowRunID uuid.UUID, nodeKey string) (uuid.UUID, error) {
	row, err := s.q.GetAgentSession(ctx, sqlcgen.GetAgentSessionParams{FlowRunID: flowRunID, NodeKey: nodeKey})
	return row.ClaudeSessionID, mapErr(err)
}

// CreateAgentSession records the Claude session of an agent node in a run.
func (s *Store) CreateAgentSession(ctx context.Context, flowRunID uuid.UUID, nodeKey string, profileID, sessionID uuid.UUID) error {
	_, err := s.q.InsertAgentSession(ctx, sqlcgen.InsertAgentSessionParams{
		FlowRunID: flowRunID, NodeKey: nodeKey, ProfileID: profileID, ClaudeSessionID: sessionID,
	})
	return mapErr(err)
}
