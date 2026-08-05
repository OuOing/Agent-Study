package agent

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"net/http"
	"time"
)

type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

type RetryModel struct {
	model  Model
	policy RetryPolicy
}

func NewRetryModel(model Model, policy RetryPolicy) *RetryModel {
	return &RetryModel{model: model, policy: policy}
}

func (m *RetryModel) Decide(ctx context.Context, goal string, observation map[string]any) (Decision, error) {
	attempts := m.policy.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		decision, err := m.model.Decide(ctx, goal, observation)
		if err == nil {
			return decision, nil
		}
		lastErr = err
		if attempt == attempts || !isRetryable(err) {
			return Decision{}, err
		}
		if err := waitForRetry(ctx, m.delay(attempt)); err != nil {
			return Decision{}, err
		}
	}
	return Decision{}, lastErr
}

func (m *RetryModel) delay(attempt int) time.Duration {
	delay := m.policy.BaseDelay * time.Duration(1<<(attempt-1))
	if m.policy.MaxDelay > 0 && delay > m.policy.MaxDelay {
		delay = m.policy.MaxDelay
	}
	if delay <= 0 {
		return 0
	}
	return delay/2 + time.Duration(rand.Int63n(int64(delay/2)+1))
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func isRetryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= 500
	}
	var netErr net.Error
	return errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary())
}
