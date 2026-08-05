package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"agent-go-api/internal/agent"
	"agent-go-api/internal/config"
	"agent-go-api/internal/database"
	"agent-go-api/internal/httpapi"
	"agent-go-api/internal/task"
	"agent-go-api/internal/worker"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	cfg := config.Load()

	repo := task.Repository(task.NewMemoryRepository())
	recorder := agent.RunRecorder(agent.LogRecorder{})
	if cfg.Database.URL != "" {
		dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		db, err := database.Open(dbCtx, database.Config{
			Driver:          cfg.Database.Driver,
			URL:             cfg.Database.URL,
			MaxOpenConns:    cfg.Database.MaxOpenConns,
			MaxIdleConns:    cfg.Database.MaxIdleConns,
			ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
		})
		if err != nil {
			log.Fatal("open database:", err)
		}
		defer db.Close()

		repo = task.NewSQLRepository(db)
		recorder = agent.NewSQLRecorder(db)
	}

	service := task.NewService(repo)
	w := worker.New(16, func(ctx context.Context, job worker.Job) {
		runCtx := agent.WithRunID(ctx, job.ID)

		_ = service.UpdateStatus(runCtx, job.ID, "running")
		model := agent.DemoModel{}
		registry := agent.NewRegistry(agent.SearchNotesTool{})
		orchestrator := agent.NewOrchestratorWithRecorder(model, registry, cfg.Agent.MaxSteps, recorder)
		_, err := orchestrator.Run(runCtx, job.Goal)
		if err != nil {
			_ = service.UpdateStatus(runCtx, job.ID, "failed")
			return
		}
		_ = service.UpdateStatus(runCtx, job.ID, "completed")
	})
	handler := httpapi.NewHandler(service, w)
	ctx := context.Background()
	go w.Run(ctx)

	server := &http.Server{Addr: cfg.Addr, Handler: handler.Routes()}
	log.Println("Go API listening on", cfg.Addr)
	log.Fatal(server.ListenAndServe())
}
