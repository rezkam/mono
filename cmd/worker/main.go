package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	sqlstorage "github.com/rezkam/mono/internal/storage/sql"
	"github.com/rezkam/mono/internal/worker"
)

func main() {
	// Use signal.NotifyContext for automatic context cancellation on signals
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Get PostgreSQL connection string from environment
	pgURL := os.Getenv("POSTGRES_URL")
	if pgURL == "" {
		log.Fatal("POSTGRES_URL environment variable is required")
	}

	// Connect to database
	store, err := sqlstorage.NewStore(ctx, sqlstorage.DBConfig{
		DSN: pgURL,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer store.Close()

	// Create worker
	w := worker.New(store)

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
