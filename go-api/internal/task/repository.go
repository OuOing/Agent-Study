package task

import (
	"context"
	"errors"
	"sync"
)

var ErrNotFound = errors.New("task not found")

type Repository interface {
	Create(ctx context.Context, task Task) (Task, error)
	Get(ctx context.Context, id string) (Task, error)
	UpdateStatus(ctx context.Context, id, status string) error
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

func (r *MemoryRepository) UpdateStatus(_ context.Context, id, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok {
		return ErrNotFound
	}
	t.Status = status
	r.tasks[id] = t
	return nil
}
