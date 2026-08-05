package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

var ErrEmptySchema = errors.New("schema is empty")

func ApplySchema(ctx context.Context, db *sql.DB, schema string) error {
	if strings.TrimSpace(schema) == "" {
		return ErrEmptySchema
	}
	_, err := db.ExecContext(ctx, schema)
	return err
}
