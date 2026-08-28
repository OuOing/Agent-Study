package task

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryRepositoryCompleteStoresResult(t *testing.T) {
	repo := NewMemoryRepository()
	created, err := repo.Create(context.Background(), Task{ID: "task-1", Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Complete(context.Background(), created.ID, TaskResult{Content: "done"}); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "completed" || got.Result == nil || got.Result.Content != "done" || got.ErrorCode != "" {
		t.Fatalf("unexpected completed task: %#v", got)
	}
}

func TestMemoryRepositoryFailStoresErrorCode(t *testing.T) {
	repo := NewMemoryRepository()
	created, err := repo.Create(context.Background(), Task{ID: "task-1", Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Fail(context.Background(), created.ID, "agent_run_failed"); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || got.Result != nil || got.ErrorCode != "agent_run_failed" {
		t.Fatalf("unexpected failed task: %#v", got)
	}
}

func TestMemoryRepositoryCompleteMissingTask(t *testing.T) {
	repo := NewMemoryRepository()
	err := repo.Complete(context.Background(), "missing", TaskResult{Content: "done"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
