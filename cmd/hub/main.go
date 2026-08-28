// Command hub is the agent-hub-core entrypoint.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Zettaverse/agent-hub-core/internal/api"
	"github.com/Zettaverse/agent-hub-core/internal/config"
	"github.com/Zettaverse/agent-hub-core/internal/logging"
	"github.com/Zettaverse/agent-hub-core/internal/store"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx := context.Background()

	st, err := buildStore(ctx, cfg)
	if err != nil {
		logger.Error("failed to initialize store", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	srv := api.NewServer(cfg, st, logger)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("hub listening", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info("shutdown signal received", "signal", sig.String())
	case err := <-errCh:
		logger.Error("server error", "error", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("shutdown complete")
}

func buildStore(ctx context.Context, cfg config.Config) (store.Store, error) {
	if cfg.UseMemoryStore {
		return store.NewMemoryStore(), nil
	}
	pg, err := store.NewPGWithMigrations(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	return pg, nil
}
