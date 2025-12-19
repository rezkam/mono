package main

import (
	"context"
	"log"
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
	// Use signal.NotifyContext for automatic context cancellation on signals
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Load configuration
	cfg, err := config.LoadWorkerConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to database
	store, err := postgres.NewStoreWithConfig(ctx, postgres.DBConfig{
		DSN: cfg.StorageDSN,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer store.Close()

	// Create worker with configurable operation timeout
	w := worker.New(store,
		worker.WithOperationTimeout(time.Duration(cfg.WorkerOperationTimeout)*time.Second),
	)

	// Start worker in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- w.Start(ctx)
	}()

	slog.InfoContext(ctx, "Recurring task worker started")

	// Wait for context cancellation (from signal) or worker error
	select {
	case <-ctx.Done():
		slog.InfoContext(ctx, "Received shutdown signal, stopping worker...")
		w.Stop()
	case err := <-errChan:
		if err != nil && err != context.Canceled {
			slog.ErrorContext(ctx, "Worker error", "error", err)
		}
	}

	slog.InfoContext(ctx, "Worker shut down gracefully")
}
