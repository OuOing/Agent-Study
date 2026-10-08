package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrOperationInProgress = errors.New("tool operation in progress")
var ErrReconciliationRequired = errors.New("tool operation requires reconciliation")
var ErrOperationConflict = errors.New("operation id reused for a different tool call")

// ToolCall identifies one business operation. Retries must reuse OperationID.
type ToolCall struct {
	OperationID string
	ToolName    string
	Arguments   map[string]any
}

type BeginState string

const (
	BeginAcquired       BeginState = "acquired"
	BeginCached         BeginState = "cached"
	BeginInProgress     BeginState = "in_progress"
	BeginNeedsReconcile BeginState = "needs_reconcile"
)

type BeginResult struct {
	State  BeginState
	Output map[string]any
}

// ExecutionStore atomically grants execution rights and remembers results.
type ExecutionStore interface {
	Begin(ctx context.Context, call ToolCall) (BeginResult, error)
	Succeed(ctx context.Context, operationID string, output map[string]any) error
	MarkUnknown(ctx context.Context, operationID string) error
}

type IdempotentExecutor struct {
	registry *Registry
	store    ExecutionStore
}

func NewIdempotentExecutor(registry *Registry, store ExecutionStore) *IdempotentExecutor {
	return &IdempotentExecutor{registry: registry, store: store}
}

func (e *IdempotentExecutor) Execute(ctx context.Context, call ToolCall) (map[string]any, error) {
	if e == nil || e.registry == nil || e.store == nil {
		return nil, errors.New("tool executor requires a registry and store")
	}
	if call.OperationID == "" || call.ToolName == "" {
		return nil, errors.New("operation id and tool name are required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	begin, err := e.store.Begin(ctx, call)
	if err != nil {
		return nil, fmt.Errorf("begin tool operation: %w", err)
	}
	switch begin.State {
	case BeginCached:
		return begin.Output, nil
	case BeginInProgress:
		return nil, ErrOperationInProgress
	case BeginNeedsReconcile:
		return nil, ErrReconciliationRequired
	case BeginAcquired:
		// Only the acquired operation may reach the tool.
	default:
		return nil, fmt.Errorf("unexpected begin state: %q", begin.State)
	}

	output, toolErr := e.registry.Execute(ctx, call.ToolName, call.Arguments)
	finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if toolErr != nil {
		// The interface cannot prove whether a tool produced an external effect.
		if err := e.store.MarkUnknown(finalizeCtx, call.OperationID); err != nil {
			return nil, errors.Join(toolErr, fmt.Errorf("mark tool outcome unknown: %w", err))
		}
		return nil, toolErr
	}
	if err := e.store.Succeed(finalizeCtx, call.OperationID, output); err != nil {
		return nil, fmt.Errorf("save tool result: %w", err)
	}
	return output, nil
}

type memoryExecution struct {
	toolName      string
	argumentsJSON string
	status        string
	outputJSON    []byte
}

// MemoryExecutionStore is a learning implementation. It does not survive restarts.
type MemoryExecutionStore struct {
	mu         sync.Mutex
	executions map[string]*memoryExecution
}

func NewMemoryExecutionStore() *MemoryExecutionStore {
	return &MemoryExecutionStore{executions: make(map[string]*memoryExecution)}
}

func (s *MemoryExecutionStore) Begin(ctx context.Context, call ToolCall) (BeginResult, error) {
	if err := ctx.Err(); err != nil {
		return BeginResult{}, err
	}
	if call.OperationID == "" || call.ToolName == "" {
		return BeginResult{}, errors.New("operation id and tool name are required")
	}
	argumentsJSON, err := json.Marshal(call.Arguments)
	if err != nil {
		return BeginResult{}, fmt.Errorf("encode tool arguments: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.executions[call.OperationID]; ok {
		if existing.toolName != call.ToolName || existing.argumentsJSON != string(argumentsJSON) {
			return BeginResult{}, ErrOperationConflict
		}
		switch existing.status {
		case "running":
			return BeginResult{State: BeginInProgress}, nil
		case "unknown":
			return BeginResult{State: BeginNeedsReconcile}, nil
		case "succeeded":
			var output map[string]any
			if err := json.Unmarshal(existing.outputJSON, &output); err != nil {
				return BeginResult{}, fmt.Errorf("decode cached tool result: %w", err)
			}
			return BeginResult{State: BeginCached, Output: output}, nil
		default:
			return BeginResult{}, fmt.Errorf("unexpected tool operation status: %q", existing.status)
		}
	}
	s.executions[call.OperationID] = &memoryExecution{
		toolName:      call.ToolName,
		argumentsJSON: string(argumentsJSON),
		status:        "running",
	}
	return BeginResult{State: BeginAcquired}, nil
}

func (s *MemoryExecutionStore) Succeed(ctx context.Context, operationID string, output map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	outputJSON, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("encode tool result: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	execution, ok := s.executions[operationID]
	if !ok || execution.status != "running" {
		return ErrOperationConflict
	}
	execution.outputJSON = outputJSON
	execution.status = "succeeded"
	return nil
}

func (s *MemoryExecutionStore) MarkUnknown(ctx context.Context, operationID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	execution, ok := s.executions[operationID]
	if !ok || execution.status != "running" {
		return ErrOperationConflict
	}
	execution.status = "unknown"
	return nil
}
