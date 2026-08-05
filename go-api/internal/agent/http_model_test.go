package agent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestHTTPModelDecide(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"decision":{"type":"final","content":"done"}}`)),
			Header:     make(http.Header),
		}, nil
	})}

	model := NewHTTPModel(client, "https://model.example/test", "test-key", "test-model", nil)
	decision, err := model.Decide(context.Background(), "test goal", nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Type != "final" || decision.Content != "done" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestHTTPModelReturnsAPIError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader("rate limited")),
			Header:     make(http.Header),
		}, nil
	})}

	model := NewHTTPModel(client, "https://model.example/test", "", "test-model", nil)
	_, err := model.Decide(context.Background(), "test goal", nil)
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 APIError, got %v", err)
	}
}
