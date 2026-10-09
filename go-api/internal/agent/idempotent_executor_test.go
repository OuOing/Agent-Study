package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

type countingTool struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
	err     error
}

func (*countingTool) Name() string { return "counting" }

func (t *countingTool) Execute(_ context.Context, _ map[string]any) (map[string]any, error) {
	t.calls.Add(1)
	if t.started != nil {
		close(t.started)
		<-t.release
	}
	if t.err != nil {
		return nil, t.err
	}
	return map[string]any{"count": 1}, nil
}

type failingBeginStore struct{}

func (failingBeginStore) Begin(context.Context, ToolCall) (BeginResult, error) {
	return BeginResult{}, errors.New("store unavailable")
}

func (failingBeginStore) Succeed(context.Context, string, map[string]any) error { return nil }
func (failingBeginStore) MarkUnknown(context.Context, string) error             { return nil }

type failingSucceedStore struct {
	*MemoryExecutionStore
}

func (failingSucceedStore) Succeed(context.Context, string, map[string]any) error {
	return errors.New("result storage unavailable")
}

type cancelingTool struct {
	cancel context.CancelFunc
}

func (*cancelingTool) Name() string { return "canceling" }

func (t *cancelingTool) Execute(_ context.Context, _ map[string]any) (map[string]any, error) {
	t.cancel()
	return nil, context.Canceled
}

func TestIdempotentExecutorReturnsCachedResult(t *testing.T) {
	tool := &countingTool{}
	executor := NewIdempotentExecutor(NewRegistry(tool), NewMemoryExecutionStore())
	call := ToolCall{OperationID: "op-1", ToolName: tool.Name(), Arguments: map[string]any{"query": "notes"}}

	first, err := executor.Execute(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	first["count"] = 99
	second, err := executor.Execute(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if got := tool.calls.Load(); got != 1 {
		t.Fatalf("tool executed %d times, want 1", got)
	}
	if second["count"] != float64(1) {
		t.Fatalf("cached result changed: %#v", second)
	}
}

func TestIdempotentExecutorStopsWhenStoreFails(t *testing.T) {
	tool := &countingTool{}
	executor := NewIdempotentExecutor(NewRegistry(tool), failingBeginStore{})
	_, err := executor.Execute(context.Background(), ToolCall{OperationID: "op-1", ToolName: tool.Name()})
	if err == nil {
		t.Fatal("expected storage error")
	}
	if got := tool.calls.Load(); got != 0 {
		t.Fatalf("tool executed after storage failure: %d calls", got)
	}
}

func TestIdempotentExecutorDoesNotRepeatWhenSavingResultFails(t *testing.T) {
	tool := &countingTool{}
	store := failingSucceedStore{MemoryExecutionStore: NewMemoryExecutionStore()}
	executor := NewIdempotentExecutor(NewRegistry(tool), store)
	call := ToolCall{OperationID: "op-1", ToolName: tool.Name()}

	if _, err := executor.Execute(context.Background(), call); err == nil {
		t.Fatal("expected result storage error")
	}
	if _, err := executor.Execute(context.Background(), call); !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("expected in-progress after uncertain save, got %v", err)
	}
	if got := tool.calls.Load(); got != 1 {
		t.Fatalf("tool executed %d times, want 1", got)
	}
}

func TestIdempotentExecutorDoesNotRunConcurrentDuplicate(t *testing.T) {
	tool := &countingTool{started: make(chan struct{}), release: make(chan struct{})}
	executor := NewIdempotentExecutor(NewRegistry(tool), NewMemoryExecutionStore())
	call := ToolCall{OperationID: "op-1", ToolName: tool.Name()}
	firstDone := make(chan error, 1)
	go func() {
		_, err := executor.Execute(context.Background(), call)
		firstDone <- err
	}()
	<-tool.started
	_, err := executor.Execute(context.Background(), call)
	if !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("expected in-progress error, got %v", err)
	}
	close(tool.release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if got := tool.calls.Load(); got != 1 {
		t.Fatalf("tool executed %d times, want 1", got)
	}
}

func TestIdempotentExecutorRequiresReconciliationAfterToolError(t *testing.T) {
	tool := &countingTool{err: errors.New("uncertain provider result")}
	executor := NewIdempotentExecutor(NewRegistry(tool), NewMemoryExecutionStore())
	call := ToolCall{OperationID: "op-1", ToolName: tool.Name()}
	if _, err := executor.Execute(context.Background(), call); err == nil {
		t.Fatal("expected tool error")
	}
	if _, err := executor.Execute(context.Background(), call); !errors.Is(err, ErrReconciliationRequired) {
		t.Fatalf("expected reconciliation required, got %v", err)
	}
	if got := tool.calls.Load(); got != 1 {
		t.Fatalf("tool executed %d times, want 1", got)
	}
}

func TestIdempotentExecutorRecordsUnknownAfterCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tool := &cancelingTool{cancel: cancel}
	executor := NewIdempotentExecutor(NewRegistry(tool), NewMemoryExecutionStore())
	call := ToolCall{OperationID: "op-1", ToolName: tool.Name()}
	if _, err := executor.Execute(ctx, call); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if _, err := executor.Execute(context.Background(), call); !errors.Is(err, ErrReconciliationRequired) {
		t.Fatalf("expected outcome to be recorded as unknown, got %v", err)
	}
}

func TestIdempotentExecutorRejectsChangedCall(t *testing.T) {
	tool := &countingTool{}
	executor := NewIdempotentExecutor(NewRegistry(tool), NewMemoryExecutionStore())
	if _, err := executor.Execute(context.Background(), ToolCall{
		OperationID: "op-1", ToolName: tool.Name(), Arguments: map[string]any{"amount": 100},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := executor.Execute(context.Background(), ToolCall{
		OperationID: "op-1", ToolName: tool.Name(), Arguments: map[string]any{"amount": 200},
	})
	if !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("expected operation conflict, got %v", err)
	}
}
