package agent

import (
	"context"
	"log"
	"time"
)

type RunEvent struct {
	Step         int
	Kind         string
	ToolName     string
	DecisionType string
	Input        map[string]any
	Output       map[string]any
	Duration     time.Duration
	Error        string
}

type RunRecorder interface {
	Record(ctx context.Context, event RunEvent) error
}

type NoopRecorder struct{}

func (NoopRecorder) Record(context.Context, RunEvent) error {
	return nil
}

type LogRecorder struct{}

func (LogRecorder) Record(ctx context.Context, event RunEvent) error {
	log.Printf(
		"run_id=%s step=%d kind=%s tool_name=%s decision_type=%s duration_ms=%d error=%q",
		RunID(ctx),
		event.Step,
		event.Kind,
		event.ToolName,
		event.DecisionType,
		event.Duration.Milliseconds(),
		event.Error,
	)
	return nil
}
