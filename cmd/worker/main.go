package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rezkam/mono/internal/application/worker"
	"github.com/rezkam/mono/internal/config"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Worker error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Use signal.NotifyContext for automatic context cancellation on signals
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Load configuration
	cfg, err := config.LoadWorkerConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Connect to database
	store, err := postgres.NewStoreWithConfig(ctx, postgres.DBConfig{
		DSN: cfg.StorageDSN,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer store.Close()

	// Create worker with configurable operation timeout
	w := worker.New(store,
		worker.WithOperationTimeout(time.Duration(cfg.WorkerOperationTimeout)*time.Second),
	)

	slog.InfoContext(ctx, "Recurring task worker started")

	// Start worker - blocks until ctx is cancelled and in-flight work completes
	if err := w.Start(ctx); err != nil {
		return fmt.Errorf("worker error: %w", err)
	}

	slog.InfoContext(ctx, "Worker shut down gracefully")
	return nil
}
