package database

import (
	"context"
	"errors"
	"testing"
)

func TestOpenRequiresDatabaseURL(t *testing.T) {
	db, err := Open(context.Background(), Config{Driver: "pgx"})
	if !errors.Is(err, ErrMissingURL) {
		t.Fatalf("expected ErrMissingURL, got %v", err)
	}
	if db != nil {
		t.Fatal("expected nil db")
	}
}
