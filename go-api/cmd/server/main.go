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
	registry := agent.NewRegistry(agent.SearchNotesTool{})
	var model agent.Model = agent.DemoModel{}
	if cfg.Model.Endpoint != "" {
		httpClient := &http.Client{Timeout: cfg.Agent.Timeout}
		model = agent.NewHTTPModel(
			httpClient,
			cfg.Model.Endpoint,
			cfg.Model.APIKey,
			cfg.Model.Name,
			[]agent.ToolDefinition{agent.SearchNotesDefinition()},
		)
		model = agent.NewRetryModel(model, agent.RetryPolicy{
			MaxAttempts: 3,
			BaseDelay:   200 * time.Millisecond,
			MaxDelay:    2 * time.Second,
		})
	}

	w := worker.New(16, func(ctx context.Context, job worker.Job) {
		jobCtx, cancel := context.WithTimeout(ctx, cfg.Agent.Timeout)
		defer cancel()
		runCtx := agent.WithRunID(jobCtx, job.ID)

		if err := service.Start(runCtx, job.ID); err != nil {
			log.Printf("start task %s: %v", job.ID, err)
			return
		}
		orchestrator := agent.NewOrchestratorWithRecorder(model, registry, cfg.Agent.MaxSteps, recorder)
		content, err := orchestrator.Run(runCtx, job.Goal)
		finalizeCtx, finalizeCancel := context.WithTimeout(ctx, 5*time.Second)
		defer finalizeCancel()
		if err != nil {
			_ = service.Fail(finalizeCtx, job.ID, task.StatusRunning, "agent_run_failed")
			return
		}
		_ = service.Complete(finalizeCtx, job.ID, content)
	})
	handler := httpapi.NewHandler(service, w)
	ctx := context.Background()
	go w.Run(ctx)

	server := &http.Server{Addr: cfg.Addr, Handler: handler.Routes()}
	log.Println("Go API listening on", cfg.Addr)
	log.Fatal(server.ListenAndServe())
}
