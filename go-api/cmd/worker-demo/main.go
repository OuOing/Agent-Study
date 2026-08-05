package main

import (
	"context"
	"time"

	"agent-go-api/internal/worker"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	w := worker.New(4, worker.DemoHandler)
	go w.Run(ctx)
	_ = w.Submit(ctx, worker.Job{ID: "job-1", Goal: "分析会议记录"})
	<-ctx.Done()
}
