package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr     string
	Database DatabaseConfig
	Model    ModelConfig
	Agent    AgentConfig
}

type DatabaseConfig struct {
	Driver          string
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type ModelConfig struct {
	Endpoint string
	APIKey   string
	Name     string
}

type AgentConfig struct {
	MaxSteps int
	Timeout  time.Duration
}

func Load() Config {
	return Config{
		Addr: envString("ADDR", ":8080"),
		Database: DatabaseConfig{
			Driver:          envString("DATABASE_DRIVER", "pgx"),
			URL:             envString("DATABASE_URL", ""),
			MaxOpenConns:    envInt("DATABASE_MAX_OPEN_CONNS", 10),
			MaxIdleConns:    envInt("DATABASE_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: envDuration("DATABASE_CONN_MAX_LIFETIME", 30*time.Minute),
		},
		Model: ModelConfig{
			Endpoint: envString("MODEL_API_ENDPOINT", ""),
			APIKey:   envString("MODEL_API_KEY", ""),
			Name:     envString("MODEL_NAME", "demo-model"),
		},
		Agent: AgentConfig{
			MaxSteps: envInt("AGENT_MAX_STEPS", 4),
			Timeout:  envDuration("AGENT_TIMEOUT", 60*time.Second),
		},
	}
}

func envString(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
