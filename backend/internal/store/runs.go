package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/store/sqlcgen"
)

func toFlowRun(r sqlcgen.FlowRun) domain.FlowRun {
	return domain.FlowRun{
		ID:            r.ID,
		FlowVersionID: r.FlowVersionID,
		CardID:        r.CardID,
		Trigger:       r.Trigger,
		Status:        domain.RunStatus(r.Status),
		StartedAt:     r.StartedAt,
		FinishedAt:    r.FinishedAt,
	}
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

// CreateFlowRun starts a run of a flow version for a card.
func (s *Store) CreateFlowRun(ctx context.Context, versionID, cardID uuid.UUID, trigger string) (domain.FlowRun, error) {
	row, err := s.q.InsertFlowRun(ctx, sqlcgen.InsertFlowRunParams{
		FlowVersionID: versionID, CardID: cardID, Trigger: trigger, Status: string(domain.RunRunning),
	})
	return mapRow(row, err, toFlowRun)
}

// GetFlowRun returns one flow run.
func (s *Store) GetFlowRun(ctx context.Context, id uuid.UUID) (domain.FlowRun, error) {
	row, err := s.q.GetFlowRun(ctx, id)
	return mapRow(row, err, toFlowRun)
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
