// Package store persists the board in Postgres using sqlc-generated queries.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"orquestrator/internal/domain"
	"orquestrator/internal/store/sqlcgen"
)

// Postgres error codes mapped to domain errors.
const (
	pgRestrictViolation   = "23001"
	pgForeignKeyViolation = "23503"
	pgUniqueViolation     = "23505"
)

// last is an upper bound for position ranges ("until the end of the lane").
const last = 1<<31 - 1

// Beginner starts transactions and hands out connections; *pgxpool.Pool
// satisfies it.
type Beginner interface {
	sqlcgen.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
}

// Store implements the board persistence.
type Store struct {
	db Beginner
	q  sqlcgen.Querier
	// txQuerier builds the querier used inside a transaction. Tests replace it
	// to inject failures in the middle of multi-step operations.
	txQuerier func(pgx.Tx) sqlcgen.Querier
}

// New returns a Store backed by db.
func New(db Beginner) *Store {
	return &Store{
		db:        db,
		q:         sqlcgen.New(db),
		txQuerier: func(tx pgx.Tx) sqlcgen.Querier { return sqlcgen.New(tx) },
	}
}

// inTx runs fn inside a transaction, committing when fn succeeds.
func (s *Store) inTx(ctx context.Context, fn func(q sqlcgen.Querier) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(s.txQuerier(tx)); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// mapErr translates database errors into domain errors.
func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgUniqueViolation:
			return fmt.Errorf("%w: %s", domain.ErrConflict, pgErr.ConstraintName)
		case pgForeignKeyViolation:
			return fmt.Errorf("%w: %s", domain.ErrInvalidReference, pgErr.ConstraintName)
		case pgRestrictViolation:
			return fmt.Errorf("%w: %s", domain.ErrInUse, pgErr.ConstraintName)
		}
	}
	return err
}

// mapRows converts sqlc rows to domain values.
func mapRows[R, D any](rows []R, err error, conv func(R) D) ([]D, error) {
	if err != nil {
		return nil, err
	}
	out := make([]D, len(rows))
	for i, r := range rows {
		out[i] = conv(r)
	}
	return out, nil
}

// mapRow converts one sqlc row to a domain value, translating errors.
func mapRow[R, D any](row R, err error, conv func(R) D) (D, error) {
	if err != nil {
		var zero D
		return zero, mapErr(err)
	}
	return conv(row), nil
}

// requireDeleted turns "no rows affected" into ErrNotFound.
func requireDeleted(n int64, err error) error {
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// clamp limits position to [0, upper].
func clamp(position, upper int) int {
	return max(0, min(position, upper))
}

// gap returns the shift that makes room for an item moving from one position
// to another within the same list: the items in [lo, hi] move by delta.
func gap(from, to int32) (delta, lo, hi int32, ok bool) {
	switch {
	case to < from:
		return 1, to, from - 1, true
	case to > from:
		return -1, from + 1, to, true
	}
	return 0, 0, 0, false
}
