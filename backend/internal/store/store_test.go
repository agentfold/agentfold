package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"orquestrator/internal/domain"
	"orquestrator/internal/store/sqlcgen"
)

func TestInTxCommitFailure(t *testing.T) {
	s := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	err := s.inTx(ctx, func(sqlcgen.Querier) error {
		cancel()
		return nil
	})
	if err == nil {
		t.Fatal("expected the commit to fail")
	}
}

func TestInTxBeginFailure(t *testing.T) {
	s := newStore(t)
	err := s.inTx(cancelled(), func(sqlcgen.Querier) error {
		t.Fatal("fn must not run")
		return nil
	})
	if err == nil {
		t.Fatal("expected begin to fail")
	}
}

func TestMapErr(t *testing.T) {
	other := &pgconn.PgError{Code: "23514"}
	if err := mapErr(other); !errors.Is(err, other) {
		t.Fatalf("unmapped code: %v", err)
	}
	restrict := &pgconn.PgError{Code: pgRestrictViolation, ConstraintName: "c"}
	if err := mapErr(restrict); !errors.Is(err, domain.ErrInUse) || err.Error() != "in use: c" {
		t.Fatalf("restrict: %v", err)
	}
}
