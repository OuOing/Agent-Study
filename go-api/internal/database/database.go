package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrMissingURL = errors.New("database url is required")

type Config struct {
	Driver          string
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func Open(ctx context.Context, cfg Config) (*sql.DB, error) {
	if cfg.URL == "" {
		return nil, ErrMissingURL
	}

	db, err := sql.Open(cfg.Driver, cfg.URL)
	if err != nil {
		return nil, err
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
