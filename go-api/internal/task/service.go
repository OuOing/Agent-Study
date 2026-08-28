package task

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidGoal = errors.New("goal is required")

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Task, error) {
	goal := strings.TrimSpace(input.Goal)
	if goal == "" {
		return Task{}, ErrInvalidGoal
	}
	t := Task{
		ID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		UserID: "demo-user",
		Goal:   goal,
		Status: "created",
	}
	return s.repo.Create(ctx, t)
}

func (s *Service) Get(ctx context.Context, id string) (Task, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) UpdateStatus(ctx context.Context, id, status string) error {
	return s.repo.UpdateStatus(ctx, id, status)
}

func (s *Service) Complete(ctx context.Context, id, content string) error {
	return s.repo.Complete(ctx, id, TaskResult{Content: content})
}

func (s *Service) Fail(ctx context.Context, id, errorCode string) error {
	return s.repo.Fail(ctx, id, errorCode)
}
