package main

import (
	"context"
	"log"
	"os"
	"time"

	"agent-go-api/internal/config"
	"agent-go-api/internal/database"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	cfg := config.Load()
	if cfg.Database.URL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	schemaPath := "db/schema.sql"
	if len(os.Args) > 1 {
		schemaPath = os.Args[1]
	}

	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		log.Fatal("read schema:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := database.Open(ctx, database.Config{
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

	if err := database.ApplySchema(ctx, db, string(schema)); err != nil {
		log.Fatal("apply schema:", err)
	}
	log.Println("schema applied:", schemaPath)
}
