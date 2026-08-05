package agent

import "context"

type contextKey string

const runIDKey contextKey = "run_id"

func WithRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, runIDKey, runID)
}

func RunID(ctx context.Context) string {
	runID, _ := ctx.Value(runIDKey).(string)
	return runID
}
