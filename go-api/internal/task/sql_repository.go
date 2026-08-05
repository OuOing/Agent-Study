package task

import (
	"context"
	"database/sql"
	"errors"
)

type SQLRepository struct {
	db *sql.DB
}

func NewSQLRepository(db *sql.DB) *SQLRepository {
	return &SQLRepository{db: db}
}

func (r *SQLRepository) Create(ctx context.Context, t Task) (Task, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tasks (id, user_id, goal, status)
		VALUES ($1, $2, $3, $4)
	`, t.ID, t.UserID, t.Goal, t.Status)
	return t, err
}

func (r *SQLRepository) Get(ctx context.Context, id string) (Task, error) {
	var t Task
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, goal, status
		FROM tasks
		WHERE id = $1
	`, id).Scan(&t.ID, &t.UserID, &t.Goal, &t.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	return t, err
}

func (r *SQLRepository) UpdateStatus(ctx context.Context, id, status string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = $2, updated_at = NOW()
		WHERE id = $1
	`, id, status)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
