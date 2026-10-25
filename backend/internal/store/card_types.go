package store

import (
	"context"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/store/sqlcgen"
)

func toCardType(r sqlcgen.CardType) domain.CardType {
	return domain.CardType{ID: r.ID, Name: r.Name, Color: r.Color, CreatedAt: r.CreatedAt}
}

// ListCardTypes returns the card types ordered by name.
func (s *Store) ListCardTypes(ctx context.Context) ([]domain.CardType, error) {
	rows, err := s.q.ListCardTypes(ctx)
	return mapRows(rows, err, toCardType)
}

// GetCardType returns one card type.
func (s *Store) GetCardType(ctx context.Context, id uuid.UUID) (domain.CardType, error) {
	row, err := s.q.GetCardType(ctx, id)
	return mapRow(row, err, toCardType)
}

// CreateCardType stores a card type.
func (s *Store) CreateCardType(ctx context.Context, in domain.CardTypeInput) (domain.CardType, error) {
	row, err := s.q.InsertCardType(ctx, sqlcgen.InsertCardTypeParams{Name: in.Name, Color: in.Color})
	return mapRow(row, err, toCardType)
}

// UpdateCardType changes the name and color of a card type.
func (s *Store) UpdateCardType(ctx context.Context, id uuid.UUID, in domain.CardTypeInput) (domain.CardType, error) {
	row, err := s.q.UpdateCardType(ctx, sqlcgen.UpdateCardTypeParams{ID: id, Name: in.Name, Color: in.Color})
	return mapRow(row, err, toCardType)
}

// DeleteCardType removes a card type no card uses.
func (s *Store) DeleteCardType(ctx context.Context, id uuid.UUID) error {
	return requireDeleted(s.q.DeleteCardType(ctx, id))
}
