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

func toArtifact(r sqlcgen.Artifact) domain.Artifact {
	return domain.Artifact{
		ID:              r.ID,
		CardID:          r.CardID,
		Name:            r.Name,
		NodeRunID:       r.NodeRunID,
		ReviewNodeRunID: r.ReviewNodeRunID,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
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
		NodeRunID:  r.NodeRunID,
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
// its card: a new version, unless it is the same as the last one. It
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

// ArtifactReview tells where the review of an artifact stands.
func (s *Store) ArtifactReview(ctx context.Context, artifactID uuid.UUID) (domain.ReviewState, error) {
	row, err := s.q.ArtifactReview(ctx, artifactID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReviewNone, nil
	}
	if err != nil {
		return "", err
	}
	handle := ""
	if row.OutputHandle != nil {
		handle = *row.OutputHandle
	}
	switch {
	case row.Status == string(domain.NodeAwaitingReview):
		return domain.ReviewOpen, nil
	case handle == "approved":
		return domain.ReviewApproved, nil
	case handle == "changes_requested" && domain.RunStatus(row.RunStatus).Active():
		return domain.ReviewRevising, nil
	}
	return domain.ReviewClosed, nil
}

// OpenReview holds a HumanReview node run until the human decides, with
// the artifacts it reviews under it: those named, or with no names those
// collected in its run. When the agent comes back from changes the node
// asked for, reply is its answer, added to the chat of the artifact the
// human wrote at. Without artifacts to review it changes nothing and
// returns none; ErrNotFound when the run ended meanwhile.
func (s *Store) OpenReview(ctx context.Context, nodeRunID, cardID uuid.UUID, names []string, reply string) (domain.NodeRun, []domain.Artifact, error) {
	var (
		nr        domain.NodeRun
		artifacts []domain.Artifact
	)
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		row, err := q.GetNodeRunForUpdate(ctx, nodeRunID)
		if err != nil {
			return mapErr(err)
		}
		if names == nil {
			names = []string{} // NULL would match nothing
		}
		rows, err := q.OpenArtifactReview(ctx, sqlcgen.OpenArtifactReviewParams{
			NodeRunID: &nodeRunID, CardID: cardID, Names: names, FlowRunID: row.FlowRunID,
		})
		if err != nil || len(rows) == 0 {
			nr = toNodeRun(row)
			return err
		}
		artifacts = make([]domain.Artifact, len(rows))
		for i, r := range rows {
			artifacts[i] = toArtifact(r)
		}
		if row, err = q.AwaitReviewNodeRun(ctx, nodeRunID); err != nil {
			return mapErr(err)
		}
		nr = toNodeRun(row)
		if _, err := q.SetFlowRunStatus(ctx, sqlcgen.SetFlowRunStatusParams{
			ID: row.FlowRunID, Status: string(domain.RunWaiting), FromStatus: string(domain.RunRunning),
		}); err != nil {
			return mapErr(err)
		}
		return addReviewReply(ctx, q, row, reply)
	})
	return nr, artifacts, err
}

// addReviewReply adds the agent answer to the chat of the artifact the
// human asked changes at, when nr comes back from such a request.
func addReviewReply(ctx context.Context, q sqlcgen.Querier, nr sqlcgen.NodeRun, reply string) error {
	if nr.Iteration < 2 {
		return nil
	}
	before, err := q.GetNodeRunByIteration(ctx, sqlcgen.GetNodeRunByIterationParams{
		FlowRunID: nr.FlowRunID, NodeKey: nr.NodeKey, Iteration: nr.Iteration - 1,
	})
	if err != nil {
		return err // iterations count up from 1
	}
	request, err := q.ReviewRequest(ctx, &before.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // approved, or the request went with its artifact
	}
	if err != nil {
		return err
	}
	last, err := latestVersion(ctx, q, request.ArtifactID)
	if err != nil {
		return err
	}
	_, err = q.InsertArtifactMessage(ctx, sqlcgen.InsertArtifactMessageParams{
		ArtifactID: request.ArtifactID, Version: int32(last.Version), //nolint:gosec // small counter
		Role: domain.RoleAgent, Text: reply, NodeRunID: &nr.ID,
	})
	return err
}

// RequestChanges records what a human asks in the review of an artifact
// and sends its HumanReview node out by changes_requested, handing the
// agent the request. ErrReviewClosed when no review awaits the artifact.
func (s *Store) RequestChanges(ctx context.Context, artifactID uuid.UUID, text string) (domain.ArtifactMessage, domain.NodeRun, error) {
	var (
		msg domain.ArtifactMessage
		nr  domain.NodeRun
	)
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		a, review, err := awaitingReview(ctx, q, artifactID)
		if err != nil {
			return err
		}
		last, err := latestVersion(ctx, q, a.ID)
		if err != nil {
			return err
		}
		row, err := q.InsertArtifactMessage(ctx, sqlcgen.InsertArtifactMessageParams{
			ArtifactID: a.ID, Version: int32(last.Version), //nolint:gosec // small counter
			Role: domain.RoleHuman, Text: text, NodeRunID: &review.ID,
		})
		if err != nil {
			return err
		}
		msg = toArtifactMessage(row)
		output, _ := json.Marshal(map[string]any{ // plain values
			"result": domain.ReviewPrompt(a.Name, last.Path, text), "artifact_id": a.ID, "feedback": text,
		})
		nr, err = decide(ctx, q, review, domain.NodeResult{Status: domain.NodeDone, OutputHandle: "changes_requested", Output: output})
		return err
	})
	return msg, nr, err
}

// ApproveReview approves the review an artifact is under: its HumanReview
// node goes out by approved, handing on what it received. ErrReviewClosed
// when no review awaits the artifact.
func (s *Store) ApproveReview(ctx context.Context, artifactID uuid.UUID) (domain.NodeRun, error) {
	var nr domain.NodeRun
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		_, review, err := awaitingReview(ctx, q, artifactID)
		if err != nil {
			return err
		}
		nr, err = decide(ctx, q, review, domain.NodeResult{Status: domain.NodeDone, OutputHandle: "approved", Output: review.Input})
		return err
	})
	return nr, err
}

// awaitingReview locks an artifact and the HumanReview node run awaiting
// it; ErrReviewClosed when none does.
func awaitingReview(ctx context.Context, q sqlcgen.Querier, artifactID uuid.UUID) (domain.Artifact, sqlcgen.NodeRun, error) {
	row, err := q.GetArtifactForUpdate(ctx, artifactID)
	if err != nil {
		return domain.Artifact{}, sqlcgen.NodeRun{}, mapErr(err)
	}
	a := toArtifact(row)
	if a.ReviewNodeRunID == nil {
		return a, sqlcgen.NodeRun{}, domain.ErrReviewClosed
	}
	review, err := q.GetNodeRunForUpdate(ctx, *a.ReviewNodeRunID)
	if err != nil {
		return a, review, err
	}
	if review.Status != string(domain.NodeAwaitingReview) {
		return a, review, domain.ErrReviewClosed
	}
	return a, review, nil
}

// decide ends a HumanReview node run with r, sets its flow run running
// again and queues the job that follows its output.
func decide(ctx context.Context, q sqlcgen.Querier, review sqlcgen.NodeRun, r domain.NodeResult) (domain.NodeRun, error) {
	if _, err := q.SetFlowRunStatus(ctx, sqlcgen.SetFlowRunStatusParams{
		ID: review.FlowRunID, Status: string(domain.RunRunning), FromStatus: string(domain.RunWaiting),
	}); err != nil {
		return domain.NodeRun{}, err // a node awaiting review holds its run waiting
	}
	row, err := q.FinishNodeRun(ctx, finishParams(review.ID, r))
	if err != nil {
		return domain.NodeRun{}, err
	}
	_, err = insertJob(ctx, q, NewJob{Kind: domain.JobFlowContinue, Payload: domain.ContinueJob{NodeRunID: review.ID}})
	return toNodeRun(row), err
}
