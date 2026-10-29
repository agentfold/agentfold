package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/store/sqlcgen"
)

func toCard(r sqlcgen.Card) domain.Card {
	return domain.Card{
		ID:           r.ID,
		Title:        r.Title,
		Description:  r.Description,
		CardTypeID:   r.CardTypeID,
		ProjectID:    r.ProjectID,
		ParentCardID: r.ParentCardID,
		LaneID:       r.LaneID,
		Position:     int(r.Position),
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}

func toCardEvent(r sqlcgen.CardEvent) domain.CardEvent {
	return domain.CardEvent{
		ID:        r.ID,
		CardID:    r.CardID,
		Type:      domain.CardEventType(r.Type),
		Payload:   r.Payload,
		CreatedAt: r.CreatedAt,
	}
}

// Timeline payloads.
type (
	cardCreatedPayload struct {
		LaneID uuid.UUID `json:"lane_id"`
	}
	cardUpdatedPayload struct{}
	cardMovedPayload   struct {
		FromLaneID uuid.UUID `json:"from_lane_id"`
		ToLaneID   uuid.UUID `json:"to_lane_id"`
		Position   int       `json:"position"`
	}
)

func addEvent(ctx context.Context, q sqlcgen.Querier, cardID uuid.UUID, typ domain.CardEventType, payload any) (domain.CardEvent, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return domain.CardEvent{}, err
	}
	row, err := q.InsertCardEvent(ctx, sqlcgen.InsertCardEventParams{CardID: cardID, Type: string(typ), Payload: data})
	return mapRow(row, err, toCardEvent)
}

// addTriggerEvent records an event that may start flows, and enqueues the
// job that starts them.
func addTriggerEvent(ctx context.Context, q sqlcgen.Querier, cardID uuid.UUID, typ domain.CardEventType, payload any) error {
	event, err := addEvent(ctx, q, cardID, typ, payload)
	if err != nil {
		return err
	}
	_, err = insertJob(ctx, q, NewJob{Kind: domain.JobFlowTrigger, Payload: domain.TriggerJob{CardEventID: event.ID}})
	return err
}

// lockLane locks a lane that a card is being placed into, so concurrent
// placements in the same lane do not pick the same position.
func lockLane(ctx context.Context, q sqlcgen.Querier, id uuid.UUID) error {
	_, err := q.GetLaneForUpdate(ctx, id)
	err = mapErr(err)
	if errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("%w: lane", domain.ErrInvalidReference)
	}
	return err
}

// ListCards returns every card, grouped by lane in board order.
func (s *Store) ListCards(ctx context.Context) ([]domain.Card, error) {
	rows, err := s.q.ListCards(ctx)
	return mapRows(rows, err, toCard)
}

// ListChildCards returns the children of a card.
func (s *Store) ListChildCards(ctx context.Context, parentID uuid.UUID) ([]domain.Card, error) {
	rows, err := s.q.ListChildCards(ctx, &parentID)
	return mapRows(rows, err, toCard)
}

// GetCard returns one card.
func (s *Store) GetCard(ctx context.Context, id uuid.UUID) (domain.Card, error) {
	row, err := s.q.GetCard(ctx, id)
	return mapRow(row, err, toCard)
}

// CreateCard appends a card to the end of its lane and records the event.
func (s *Store) CreateCard(ctx context.Context, in domain.NewCard) (domain.Card, error) {
	var card domain.Card
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		var err error
		card, err = createCard(ctx, q, in)
		return err
	})
	return card, err
}

// CreateChildCards creates the cards of a CreateCards node, all or none.
func (s *Store) CreateChildCards(ctx context.Context, cards []domain.NewCard) ([]domain.Card, error) {
	out := make([]domain.Card, len(cards))
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		for i, in := range cards {
			card, err := createCard(ctx, q, in)
			if err != nil {
				return err
			}
			out[i] = card
		}
		return nil
	})
	return out, err
}

func createCard(ctx context.Context, q sqlcgen.Querier, in domain.NewCard) (domain.Card, error) {
	if err := lockLane(ctx, q, in.LaneID); err != nil {
		return domain.Card{}, err
	}
	count, err := q.CountCardsInLane(ctx, in.LaneID)
	if err != nil {
		return domain.Card{}, err
	}
	row, err := q.InsertCard(ctx, sqlcgen.InsertCardParams{
		Title:        in.Title,
		Description:  in.Description,
		CardTypeID:   in.CardTypeID,
		ProjectID:    in.ProjectID,
		ParentCardID: in.ParentCardID,
		LaneID:       in.LaneID,
		Position:     count,
	})
	card, err := mapRow(row, err, toCard)
	if err != nil {
		return card, err
	}
	return card, addTriggerEvent(ctx, q, card.ID, domain.CardCreated, cardCreatedPayload{LaneID: card.LaneID})
}

// UpdateCard changes the editable fields of a card and records the event.
func (s *Store) UpdateCard(ctx context.Context, id uuid.UUID, in domain.CardInput) (domain.Card, error) {
	var card domain.Card
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		row, err := q.UpdateCard(ctx, sqlcgen.UpdateCardParams{
			ID:          id,
			Title:       in.Title,
			Description: in.Description,
			CardTypeID:  in.CardTypeID,
			ProjectID:   in.ProjectID,
		})
		if card, err = mapRow(row, err, toCard); err != nil {
			return err
		}
		_, err = addEvent(ctx, q, id, domain.CardUpdated, cardUpdatedPayload{})
		return err
	})
	return card, err
}

// MoveCard places a card at position in a lane, shifting the other cards, and
// records the event. Positions past the end place the card at the end.
func (s *Store) MoveCard(ctx context.Context, id, laneID uuid.UUID, position int) (domain.Card, error) {
	var card domain.Card
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		row, err := q.GetCardForUpdate(ctx, id)
		if err != nil {
			return mapErr(err)
		}
		if err := lockLane(ctx, q, laneID); err != nil {
			return err
		}
		count, err := q.CountCardsInLane(ctx, laneID)
		if err != nil {
			return err
		}
		from := row.LaneID
		sameLane := from == laneID
		if sameLane {
			count-- // the card itself is already counted
		}
		to := int32(clamp(position, int(count))) //nolint:gosec // bounded by count
		if sameLane && to == row.Position {
			card = toCard(row)
			return nil
		}
		if err := makeRoom(ctx, q, row, laneID, to); err != nil {
			return err
		}
		row, err = q.PlaceCard(ctx, sqlcgen.PlaceCardParams{ID: id, LaneID: laneID, Position: to})
		if err != nil {
			return err
		}
		card = toCard(row)
		payload := cardMovedPayload{FromLaneID: from, ToLaneID: laneID, Position: int(to)}
		if sameLane {
			_, err = addEvent(ctx, q, id, domain.CardMoved, payload)
			return err
		}
		return addTriggerEvent(ctx, q, id, domain.CardMoved, payload)
	})
	return card, err
}

// makeRoom shifts the cards around so that card can be placed at position to
// of lane laneID.
func makeRoom(ctx context.Context, q sqlcgen.Querier, card sqlcgen.Card, laneID uuid.UUID, to int32) error {
	if card.LaneID == laneID {
		delta, lo, hi, _ := gap(card.Position, to)
		return q.ShiftCards(ctx, sqlcgen.ShiftCardsParams{LaneID: laneID, Delta: delta, FromPos: lo, ToPos: hi})
	}
	err := q.ShiftCards(ctx, sqlcgen.ShiftCardsParams{LaneID: card.LaneID, Delta: -1, FromPos: card.Position + 1, ToPos: last})
	if err != nil {
		return err
	}
	return q.ShiftCards(ctx, sqlcgen.ShiftCardsParams{LaneID: laneID, Delta: 1, FromPos: to, ToPos: last})
}

// DeleteCard removes a card without children and closes the gap in its lane.
func (s *Store) DeleteCard(ctx context.Context, id uuid.UUID) error {
	return s.inTx(ctx, func(q sqlcgen.Querier) error {
		row, err := q.GetCardForUpdate(ctx, id)
		if err != nil {
			return mapErr(err)
		}
		if err := requireDeleted(q.DeleteCard(ctx, id)); err != nil {
			return err
		}
		return q.ShiftCards(ctx, sqlcgen.ShiftCardsParams{LaneID: row.LaneID, Delta: -1, FromPos: row.Position + 1, ToPos: last})
	})
}

// AddCardEvent adds an entry to the timeline of a card.
func (s *Store) AddCardEvent(ctx context.Context, cardID uuid.UUID, typ domain.CardEventType, payload any) (domain.CardEvent, error) {
	return addEvent(ctx, s.q, cardID, typ, payload)
}

// GetCardEvent returns one entry of a card timeline.
func (s *Store) GetCardEvent(ctx context.Context, id uuid.UUID) (domain.CardEvent, error) {
	row, err := s.q.GetCardEvent(ctx, id)
	return mapRow(row, err, toCardEvent)
}

// ListCardEvents returns the timeline of a card, oldest first.
func (s *Store) ListCardEvents(ctx context.Context, cardID uuid.UUID) ([]domain.CardEvent, error) {
	rows, err := s.q.ListCardEvents(ctx, cardID)
	return mapRows(rows, err, toCardEvent)
}
