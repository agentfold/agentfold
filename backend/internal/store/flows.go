package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/store/sqlcgen"
)

// flowRow holds the columns shared by the flow queries.
type flowRow struct {
	ID            uuid.UUID
	Name          string
	Enabled       bool
	Draft         []byte
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LatestVersion int32
}

func parseGraph(data []byte) (domain.Graph, error) {
	var g domain.Graph
	if err := json.Unmarshal(data, &g); err != nil {
		return g, fmt.Errorf("stored graph: %w", err)
	}
	return g, nil
}

func toFlow(r flowRow) (domain.Flow, error) {
	draft, err := parseGraph(r.Draft)
	return domain.Flow{
		ID:            r.ID,
		Name:          r.Name,
		Enabled:       r.Enabled,
		Draft:         draft,
		LatestVersion: int(r.LatestVersion),
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}, err
}

func toFlowVersion(r sqlcgen.FlowVersion) (domain.FlowVersion, error) {
	graph, err := parseGraph(r.Graph)
	return domain.FlowVersion{
		ID:          r.ID,
		FlowID:      r.FlowID,
		Version:     int(r.Version),
		Graph:       graph,
		PublishedAt: r.PublishedAt,
	}, err
}

// mapParsed is mapRows for conversions that can fail.
func mapParsed[R, D any](rows []R, err error, conv func(R) (D, error)) ([]D, error) {
	if err != nil {
		return nil, err
	}
	out := make([]D, len(rows))
	for i, r := range rows {
		if out[i], err = conv(r); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func graphJSON(g domain.Graph) []byte {
	data, _ := json.Marshal(g) // graphs hold only marshalable values
	return data
}

// ListFlows returns the flows ordered by name.
func (s *Store) ListFlows(ctx context.Context) ([]domain.Flow, error) {
	rows, err := s.q.ListFlows(ctx)
	return mapParsed(rows, err, func(r sqlcgen.ListFlowsRow) (domain.Flow, error) {
		return toFlow(flowRow(r))
	})
}

// GetFlow returns one flow.
func (s *Store) GetFlow(ctx context.Context, id uuid.UUID) (domain.Flow, error) {
	row, err := s.q.GetFlow(ctx, id)
	if err != nil {
		return domain.Flow{}, mapErr(err)
	}
	return toFlow(flowRow(row))
}

// CreateFlow stores a flow with the given draft.
func (s *Store) CreateFlow(ctx context.Context, name string, draft domain.Graph) (domain.Flow, error) {
	row, err := s.q.InsertFlow(ctx, sqlcgen.InsertFlowParams{Name: name, Draft: graphJSON(draft)})
	if err != nil {
		return domain.Flow{}, mapErr(err)
	}
	return s.GetFlow(ctx, row.ID)
}

// UpdateFlow changes the name and the enabled flag of a flow.
func (s *Store) UpdateFlow(ctx context.Context, id uuid.UUID, in domain.FlowInput) (domain.Flow, error) {
	_, err := s.q.UpdateFlow(ctx, sqlcgen.UpdateFlowParams{ID: id, Name: in.Name, Enabled: in.Enabled})
	if err != nil {
		return domain.Flow{}, mapErr(err)
	}
	return s.GetFlow(ctx, id)
}

// SaveFlowDraft replaces the draft of a flow.
func (s *Store) SaveFlowDraft(ctx context.Context, id uuid.UUID, draft domain.Graph) (domain.Flow, error) {
	_, err := s.q.UpdateFlowDraft(ctx, sqlcgen.UpdateFlowDraftParams{ID: id, Draft: graphJSON(draft)})
	if err != nil {
		return domain.Flow{}, mapErr(err)
	}
	return s.GetFlow(ctx, id)
}

// DeleteFlow removes a flow and its versions.
func (s *Store) DeleteFlow(ctx context.Context, id uuid.UUID) error {
	return requireDeleted(s.q.DeleteFlow(ctx, id))
}

// PublishFlow stores graph as the next version of a flow.
func (s *Store) PublishFlow(ctx context.Context, id uuid.UUID, graph domain.Graph) (domain.FlowVersion, error) {
	var version domain.FlowVersion
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		if _, err := q.GetFlowForUpdate(ctx, id); err != nil {
			return mapErr(err)
		}
		next, err := q.NextFlowVersion(ctx, id)
		if err != nil {
			return err
		}
		row, err := q.InsertFlowVersion(ctx, sqlcgen.InsertFlowVersionParams{FlowID: id, Version: next, Graph: graphJSON(graph)})
		if err != nil {
			return err
		}
		version, err = toFlowVersion(row)
		return err
	})
	return version, err
}

// ListFlowVersions returns the versions of a flow, newest first.
func (s *Store) ListFlowVersions(ctx context.Context, flowID uuid.UUID) ([]domain.FlowVersion, error) {
	rows, err := s.q.ListFlowVersions(ctx, flowID)
	return mapParsed(rows, err, toFlowVersion)
}
