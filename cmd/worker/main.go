package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	sqlstorage "github.com/rezkam/mono/internal/storage/sql"
	"github.com/rezkam/mono/internal/worker"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
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

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start worker in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- w.Start(ctx)
	}()

	// Wait for shutdown signal or error
	select {
	case <-sigChan:
		log.Println("Received shutdown signal, stopping worker...")
		cancel()
		w.Stop()
	case err := <-errChan:
		if err != nil && err != context.Canceled {
			log.Printf("Worker error: %v", err)
		}
	}

	log.Println("Worker shut down gracefully")
}
