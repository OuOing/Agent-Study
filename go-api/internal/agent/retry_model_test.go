package agent

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

type sequenceModel struct {
	errors []error
	calls  int
}

func (m *sequenceModel) Decide(_ context.Context, _ string, _ map[string]any) (Decision, error) {
	m.calls++
	if m.calls <= len(m.errors) {
		return Decision{}, m.errors[m.calls-1]
	}
	return Decision{Type: "final", Content: "done"}, nil
}

func TestRetryModelRetriesServerError(t *testing.T) {
	base := &sequenceModel{errors: []error{
		&APIError{StatusCode: http.StatusServiceUnavailable},
		&APIError{StatusCode: http.StatusTooManyRequests},
	}}
	model := NewRetryModel(base, RetryPolicy{MaxAttempts: 3})

	decision, err := model.Decide(context.Background(), "goal", nil)
	if err != nil {
		t.Fatal(err)
	}
	if base.calls != 3 || decision.Content != "done" {
		t.Fatalf("calls=%d decision=%#v", base.calls, decision)
	}
}

func TestRetryModelStopsOnClientError(t *testing.T) {
	base := &sequenceModel{errors: []error{
		&APIError{StatusCode: http.StatusBadRequest},
	}}
	model := NewRetryModel(base, RetryPolicy{MaxAttempts: 3})

	_, err := model.Decide(context.Background(), "goal", nil)
	if err == nil || base.calls != 1 {
		t.Fatalf("expected one failed call, calls=%d err=%v", base.calls, err)
	}
}

func TestRetryModelHonorsCancelledContext(t *testing.T) {
	base := &sequenceModel{errors: []error{
		&APIError{StatusCode: http.StatusServiceUnavailable},
	}}
	model := NewRetryModel(base, RetryPolicy{MaxAttempts: 3})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := model.Decide(ctx, "goal", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
