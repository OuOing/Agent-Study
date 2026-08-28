package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	var resultJSON []byte
	var errorCode sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, goal, status, result_json, error_code
		FROM tasks
		WHERE id = $1
	`, id).Scan(&t.ID, &t.UserID, &t.Goal, &t.Status, &resultJSON, &errorCode)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	if len(resultJSON) > 0 {
		var result TaskResult
		if err := json.Unmarshal(resultJSON, &result); err != nil {
			return Task{}, fmt.Errorf("decode task result: %w", err)
		}
		t.Result = &result
	}
	if errorCode.Valid {
		t.ErrorCode = errorCode.String
	}
	return t, nil
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

func (r *SQLRepository) Complete(ctx context.Context, id string, taskResult TaskResult) error {
	resultJSON, err := json.Marshal(taskResult)
	if err != nil {
		return fmt.Errorf("encode task result: %w", err)
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'completed', result_json = $2, error_code = NULL, updated_at = NOW()
		WHERE id = $1
	`, id, string(resultJSON))
	return checkTaskUpdated(result, err)
}

func (r *SQLRepository) Fail(ctx context.Context, id, errorCode string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'failed', result_json = NULL, error_code = $2, updated_at = NOW()
		WHERE id = $1
	`, id, errorCode)
	return checkTaskUpdated(result, err)
}

func checkTaskUpdated(result sql.Result, err error) error {
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
