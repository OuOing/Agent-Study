package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestApplySchemaRejectsEmptySchema(t *testing.T) {
	err := ApplySchema(context.Background(), &sql.DB{}, "  \n\t")
	if !errors.Is(err, ErrEmptySchema) {
		t.Fatalf("expected ErrEmptySchema, got %v", err)
	}
}
