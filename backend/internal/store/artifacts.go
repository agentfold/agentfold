package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orquestrator/internal/domain"
	"orquestrator/internal/store/sqlcgen"
)

func toArtifact(r sqlcgen.Artifact) domain.Artifact {
	return domain.Artifact{
		ID:        r.ID,
		CardID:    r.CardID,
		Name:      r.Name,
		NodeRunID: r.NodeRunID,
		FrozenAt:  r.FrozenAt,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func toArtifactVersion(r sqlcgen.ArtifactVersion) domain.ArtifactVersion {
	return domain.ArtifactVersion{
		ID:         r.ID,
		ArtifactID: r.ArtifactID,
		Version:    int(r.Version),
		Path:       r.Path,
		Content:    r.Content,
		NodeRunID:  r.NodeRunID,
		CreatedAt:  r.CreatedAt,
	}
}

func toArtifactMessage(r sqlcgen.ArtifactMessage) domain.ArtifactMessage {
	return domain.ArtifactMessage{
		ID:         r.ID,
		ArtifactID: r.ArtifactID,
		Version:    int(r.Version),
		Role:       r.Role,
		Text:       r.Text,
		FlowRunID:  r.FlowRunID,
		CreatedAt:  r.CreatedAt,
	}
}

// latestVersion returns the last version of an artifact, the zero value
// when it has none yet.
func latestVersion(ctx context.Context, q sqlcgen.Querier, artifactID uuid.UUID) (domain.ArtifactVersion, error) {
	row, err := q.LatestArtifactVersion(ctx, artifactID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ArtifactVersion{}, nil
	}
	return toArtifactVersion(row), err
}

// SaveArtifact records the content a node run collected for an artifact of
// its card: a new version, unless it is the same as the last one. Either
// way the artifact opens for review again, revised by that node run. It
// reports the version the artifact is at and whether it is new.
func (s *Store) SaveArtifact(ctx context.Context, cardID, nodeRunID uuid.UUID, name, path, content string) (domain.Artifact, int, bool, error) {
	var (
		artifact domain.Artifact
		version  int
		created  bool
	)
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		row, err := q.UpsertArtifact(ctx, sqlcgen.UpsertArtifactParams{CardID: cardID, Name: name, NodeRunID: nodeRunID})
		if err != nil {
			return mapErr(err)
		}
		artifact = toArtifact(row)
		last, err := latestVersion(ctx, q, artifact.ID)
		if err != nil {
			return err
		}
		version = last.Version
		if last.Version > 0 && last.Path == path && last.Content == content {
			return nil
		}
		version, created = last.Version+1, true
		_, err = q.InsertArtifactVersion(ctx, sqlcgen.InsertArtifactVersionParams{
			ArtifactID: artifact.ID, Version: int32(version), //nolint:gosec // small counter
			Path: path, Content: content, NodeRunID: nodeRunID,
		})
		return err
	})
	return artifact, version, created, err
}

// GetArtifact returns one artifact.
func (s *Store) GetArtifact(ctx context.Context, id uuid.UUID) (domain.Artifact, error) {
	row, err := s.q.GetArtifact(ctx, id)
	return mapRow(row, err, toArtifact)
}

// ListCardArtifacts returns the artifacts of a card, oldest first.
func (s *Store) ListCardArtifacts(ctx context.Context, cardID uuid.UUID) ([]domain.Artifact, error) {
	rows, err := s.q.ListCardArtifacts(ctx, cardID)
	return mapRows(rows, err, toArtifact)
}

// ListArtifactVersions returns the versions of an artifact, oldest first.
func (s *Store) ListArtifactVersions(ctx context.Context, artifactID uuid.UUID) ([]domain.ArtifactVersion, error) {
	rows, err := s.q.ListArtifactVersions(ctx, artifactID)
	return mapRows(rows, err, toArtifactVersion)
}

// ListArtifactMessages returns the review chat of an artifact, oldest first.
func (s *Store) ListArtifactMessages(ctx context.Context, artifactID uuid.UUID) ([]domain.ArtifactMessage, error) {
	rows, err := s.q.ListArtifactMessages(ctx, artifactID)
	return mapRows(rows, err, toArtifactMessage)
}

// ArtifactRevising reports whether a revision of the artifact is running.
func (s *Store) ArtifactRevising(ctx context.Context, artifactID uuid.UUID) (bool, error) {
	return s.q.ArtifactRevising(ctx, artifactID)
}

// StartRevision records what a human asked in the review chat of an
// artifact, reading version, and starts the run that revises it: the node
// run that collected the artifact runs again in a run of its own, resuming
// its agent session with the request. ErrArtifactFrozen once the card left
// the lane; ErrCardBusy while a flow runs for the card.
func (s *Store) StartRevision(ctx context.Context, artifactID uuid.UUID, version int, text string) (domain.ArtifactMessage, domain.FlowRun, error) {
	var (
		msg domain.ArtifactMessage
		run domain.FlowRun
	)
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		row, err := q.GetArtifactForUpdate(ctx, artifactID)
		if err != nil {
			return mapErr(err)
		}
		a := toArtifact(row)
		if a.FrozenAt != nil {
			return domain.ErrArtifactFrozen
		}
		last, err := latestVersion(ctx, q, a.ID)
		if err != nil {
			return err
		}
		if version < 1 || version > last.Version {
			return &domain.ValidationError{Field: "version", Message: "no such version of the artifact"}
		}
		source, err := q.GetNodeRun(ctx, a.NodeRunID)
		if err != nil {
			return err
		}
		sourceRun, err := q.GetFlowRun(ctx, source.FlowRunID)
		if err != nil {
			return err
		}
		busy, err := q.CardHasActiveRun(ctx, a.CardID)
		if err != nil {
			return err
		}
		if busy {
			return domain.ErrCardBusy
		}
		runRow, err := q.InsertFlowRun(ctx, sqlcgen.InsertFlowRunParams{
			FlowID: sourceRun.FlowRun.FlowID, FlowVersionID: sourceRun.FlowRun.FlowVersionID,
			CardID: a.CardID, Trigger: domain.TriggerRevision,
		})
		if err != nil {
			return err // the card has no active run: no conflict
		}
		run = namedRun(runRow, sourceRun.FlowName)
		nr, err := q.InsertNodeRun(ctx, sqlcgen.InsertNodeRunParams{
			FlowRunID: run.ID, NodeKey: source.NodeKey, Iteration: 1, Status: string(domain.NodeQueued), Input: []byte(`{}`),
		})
		if err != nil {
			return err
		}
		if err := copySession(ctx, q, source, run.ID); err != nil {
			return err
		}
		msgRow, err := q.InsertArtifactMessage(ctx, sqlcgen.InsertArtifactMessageParams{
			ArtifactID: a.ID, Version: int32(version), //nolint:gosec // small counter
			Role: domain.RoleHuman, Text: text, FlowRunID: &run.ID,
		})
		if err != nil {
			return err
		}
		msg = toArtifactMessage(msgRow)
		_, err = insertJob(ctx, q, NewJob{
			Kind:        domain.JobAgentRun,
			Payload:     domain.AgentJob{NodeRunID: nr.ID, Prompt: domain.RevisionPrompt(a.Name, last.Path, version, text)},
			MaxAttempts: 2,
		})
		return err
	})
	return msg, run, err
}

// copySession hands the agent session of a node run to the same node in
// another run, so the agent there resumes it.
func copySession(ctx context.Context, q sqlcgen.Querier, source sqlcgen.NodeRun, runID uuid.UUID) error {
	session, err := q.GetAgentSession(ctx, sqlcgen.GetAgentSessionParams{FlowRunID: source.FlowRunID, NodeKey: source.NodeKey})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // never started: the revision starts a session
	}
	if err != nil {
		return err
	}
	_, err = q.InsertAgentSession(ctx, sqlcgen.InsertAgentSessionParams{
		FlowRunID: runID, NodeKey: source.NodeKey, ProfileID: session.ProfileID, ClaudeSessionID: session.ClaudeSessionID,
	})
	return err
}

// AddRevisionReply records the answer of the agent to the revision a run
// made, at the version the artifact is at now.
func (s *Store) AddRevisionReply(ctx context.Context, flowRunID uuid.UUID, text string) (domain.ArtifactMessage, error) {
	var msg domain.ArtifactMessage
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		request, err := q.GetRevisionMessage(ctx, &flowRunID)
		if err != nil {
			return mapErr(err)
		}
		last, err := latestVersion(ctx, q, request.ArtifactID)
		if err != nil {
			return err
		}
		row, err := q.InsertArtifactMessage(ctx, sqlcgen.InsertArtifactMessageParams{
			ArtifactID: request.ArtifactID, Version: int32(last.Version), //nolint:gosec // small counter
			Role: domain.RoleAgent, Text: text, FlowRunID: &flowRunID,
		})
		msg = toArtifactMessage(row)
		return err
	})
	return msg, err
}
