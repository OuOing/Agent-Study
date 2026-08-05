package agent

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
)

type SQLRecorder struct {
	db *sql.DB
}

func NewSQLRecorder(db *sql.DB) *SQLRecorder {
	return &SQLRecorder{db: db}
}

func (r *SQLRecorder) Record(ctx context.Context, event RunEvent) error {
	taskID := RunID(ctx)
	if taskID == "" {
		return errors.New("run_id is required to record agent run")
	}

	id, err := newRunEventID()
	if err != nil {
		return err
	}
	inputJSON, err := marshalNullableJSON(event.Input)
	if err != nil {
		return err
	}
	outputJSON, err := marshalNullableJSON(event.Output)
	if err != nil {
		return err
	}

	name := event.ToolName
	if name == "" {
		name = event.DecisionType
	}

	status := "success"
	if event.Error != "" {
		status = "failed"
		outputJSON, err = marshalNullableJSON(map[string]any{"error": event.Error})
		if err != nil {
			return err
		}
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO agent_runs (
			id, task_id, step, kind, name,
			input_json, output_json, status, duration_ms
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`,
		id,
		taskID,
		event.Step,
		event.Kind,
		name,
		inputJSON,
		outputJSON,
		status,
		event.Duration.Milliseconds(),
	)
	return err
}

func marshalNullableJSON(value map[string]any) (any, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func newRunEventID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return "run_event_" + hex.EncodeToString(bytes[:]), nil
}
