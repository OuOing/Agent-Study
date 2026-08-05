package worker

import (
	"context"
	"log"
	"time"
)

type Job struct {
	ID   string
	Goal string
}

type Worker struct {
	jobs    chan Job
	handler func(context.Context, Job)
}

func New(size int, handler func(context.Context, Job)) *Worker {
	return &Worker{jobs: make(chan Job, size), handler: handler}
}

func (w *Worker) Submit(ctx context.Context, job Job) error {
	select {
	case w.jobs <- job:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Worker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("worker stopped:", ctx.Err())
			return
		case job := <-w.jobs:
			if w.handler != nil {
				w.handler(ctx, job)
			}
		}
	}
}

func DemoHandler(ctx context.Context, job Job) {
	stepCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	select {
	case <-time.After(100 * time.Millisecond):
		log.Printf("job completed: id=%s goal=%q", job.ID, job.Goal)
	case <-stepCtx.Done():
		log.Printf("job cancelled: id=%s reason=%v", job.ID, stepCtx.Err())
	}
}
