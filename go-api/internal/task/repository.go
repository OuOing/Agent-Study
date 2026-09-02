package task

import (
	"context"
	"errors"
	"sync"
)

var ErrNotFound = errors.New("task not found")
var ErrTransitionRejected = errors.New("task state transition rejected")

type Repository interface {
	Create(ctx context.Context, task Task) (Task, error)
	Get(ctx context.Context, id string) (Task, error)
	Start(ctx context.Context, id string) error
	Complete(ctx context.Context, id string, result TaskResult) error
	Fail(ctx context.Context, id string, from Status, errorCode string) error
}

type MemoryRepository struct {
	mu    sync.RWMutex
	tasks map[string]Task
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{tasks: make(map[string]Task)}
}

func (r *MemoryRepository) Create(_ context.Context, t Task) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[t.ID] = t
	return t, nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[id]
	if !ok {
		return Task{}, ErrNotFound
	}
	return t, nil
}

func (r *MemoryRepository) Start(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok {
		return ErrNotFound
	}
	if t.Status != StatusCreated {
		return ErrTransitionRejected
	}
	t.Status = StatusRunning
	r.tasks[id] = t
	return nil
}

func (r *MemoryRepository) Complete(_ context.Context, id string, result TaskResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok {
		return ErrNotFound
	}
	if t.Status != StatusRunning {
		return ErrTransitionRejected
	}
	t.Status = StatusCompleted
	t.Result = &result
	t.ErrorCode = ""
	r.tasks[id] = t
	return nil
}

func (r *MemoryRepository) Fail(_ context.Context, id string, from Status, errorCode string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok {
		return ErrNotFound
	}
	if t.Status != from || (from != StatusCreated && from != StatusRunning) {
		return ErrTransitionRejected
	}
	t.Status = StatusFailed
	t.Result = nil
	t.ErrorCode = errorCode
	r.tasks[id] = t
	return nil
}
