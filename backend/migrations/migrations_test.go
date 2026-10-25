package migrations

import (
	"context"
	"testing"
)

func TestUpRejectsNilDB(t *testing.T) {
	if err := Up(context.Background(), nil); err == nil {
		t.Fatal("expected an error for a nil db")
	}
}
