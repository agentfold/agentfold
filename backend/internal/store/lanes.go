package store

import (
	"context"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/store/sqlcgen"
)

func toLane(r sqlcgen.Lane) domain.Lane {
	return domain.Lane{ID: r.ID, Name: r.Name, Color: r.Color, Position: int(r.Position), CreatedAt: r.CreatedAt}
}

// ListLanes returns the lanes in board order.
func (s *Store) ListLanes(ctx context.Context) ([]domain.Lane, error) {
	rows, err := s.q.ListLanes(ctx)
	return mapRows(rows, err, toLane)
}

// GetLane returns one lane.
func (s *Store) GetLane(ctx context.Context, id uuid.UUID) (domain.Lane, error) {
	row, err := s.q.GetLane(ctx, id)
	return mapRow(row, err, toLane)
}

// CreateLane appends a lane to the end of the board.
func (s *Store) CreateLane(ctx context.Context, in domain.LaneInput) (domain.Lane, error) {
	var lane domain.Lane
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		count, err := q.CountLanes(ctx)
		if err != nil {
			return err
		}
		row, err := q.InsertLane(ctx, sqlcgen.InsertLaneParams{Name: in.Name, Color: in.Color, Position: count})
		lane, err = mapRow(row, err, toLane)
		return err
	})
	return lane, err
}

// UpdateLane changes the name and color of a lane.
func (s *Store) UpdateLane(ctx context.Context, id uuid.UUID, in domain.LaneInput) (domain.Lane, error) {
	row, err := s.q.UpdateLane(ctx, sqlcgen.UpdateLaneParams{ID: id, Name: in.Name, Color: in.Color})
	return mapRow(row, err, toLane)
}

// MoveLane moves a lane to position, shifting the lanes in between. Positions
// past the end move the lane to the end.
func (s *Store) MoveLane(ctx context.Context, id uuid.UUID, position int) (domain.Lane, error) {
	var lane domain.Lane
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		row, err := q.GetLaneForUpdate(ctx, id)
		if err != nil {
			return mapErr(err)
		}
		count, err := q.CountLanes(ctx)
		if err != nil {
			return err
		}
		from, to := row.Position, int32(clamp(position, int(count)-1)) //nolint:gosec // bounded by count
		if delta, lo, hi, ok := gap(from, to); ok {
			if err := q.ShiftLanes(ctx, sqlcgen.ShiftLanesParams{Delta: delta, FromPos: lo, ToPos: hi}); err != nil {
				return err
			}
		}
		row, err = q.SetLanePosition(ctx, sqlcgen.SetLanePositionParams{ID: id, Position: to})
		lane, err = mapRow(row, err, toLane)
		return err
	})
	return lane, err
}

// DeleteLane removes an empty lane and closes the gap it leaves.
func (s *Store) DeleteLane(ctx context.Context, id uuid.UUID) error {
	return s.inTx(ctx, func(q sqlcgen.Querier) error {
		row, err := q.GetLaneForUpdate(ctx, id)
		if err != nil {
			return mapErr(err)
		}
		if err := requireDeleted(q.DeleteLane(ctx, id)); err != nil {
			return err
		}
		return q.ShiftLanes(ctx, sqlcgen.ShiftLanesParams{Delta: -1, FromPos: row.Position + 1, ToPos: last})
	})
}
