// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for the hub.
type Config struct {
	Port        int
	Host        string
	DatabaseURL string
	JWTSecret   string
	JWTExpiry   time.Duration
	LogLevel    string

	// SeedOwner configures the default owner account created on first boot
	// when the user store is empty.
	SeedOwnerUsername string
	SeedOwnerPassword string

	// UseMemoryStore forces the in-memory store (handy for demos/tests).
	UseMemoryStore bool

	// Task distribution tuning.
	TaskWorkerPool int
	TaskTimeout    time.Duration

	// Dashboard history collector tuning.
	DashboardHistoryCapacity int
	DashboardHistoryInterval time.Duration
}

// Load reads configuration from the process environment, applying defaults
// for any unset value.
func Load() Config {
	return Config{
		Port:                     envInt("PORT", 8080),
		Host:                     envStr("HOST", "0.0.0.0"),
		DatabaseURL:              envStr("DATABASE_URL", ""),
		JWTSecret:                envStr("JWT_SECRET", ""),
		JWTExpiry:                envDuration("JWT_EXPIRY", 24*time.Hour),
		LogLevel:                 envStr("LOG_LEVEL", "info"),
		SeedOwnerUsername:        envStr("SEED_OWNER_USERNAME", "admin"),
		SeedOwnerPassword:        envStr("SEED_OWNER_PASSWORD", "admin"),
		UseMemoryStore:           envBool("USE_MEMORY_STORE", false),
		TaskWorkerPool:           envInt("TASK_WORKER_POOL", 4),
		TaskTimeout:              envDuration("TASK_TIMEOUT", 30*time.Second),
		DashboardHistoryCapacity: envInt("DASHBOARD_HISTORY_CAPACITY", 120),
		DashboardHistoryInterval: envDuration("DASHBOARD_HISTORY_INTERVAL", 5*time.Second),
	}
}

// Validate performs basic sanity checks on the loaded configuration.
func (c Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("config: PORT must be 1-65535, got %d", c.Port)
	}
	if c.UseMemoryStore {
		return nil
	}
	if c.DatabaseURL == "" {
		return fmt.Errorf("config: DATABASE_URL is required unless USE_MEMORY_STORE=true")
	}
	if c.JWTSecret == "" {
		return fmt.Errorf("config: JWT_SECRET is required")
	}
	return nil
}

func envStr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
