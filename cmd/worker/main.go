package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/worker"
	"github.com/rezkam/mono/internal/config"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/rezkam/mono/internal/recurring"
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
		DSN:             cfg.Database.DSN,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: time.Duration(cfg.Database.ConnMaxLifetime) * time.Second,
		ConnMaxIdleTime: time.Duration(cfg.Database.ConnMaxIdleTime) * time.Second,
		AutoMigrate:     cfg.Database.AutoMigrate,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			slog.Error("Failed to close store", "error", err)
		}
	}()

	// Create coordinator for job management
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create shared task generator
	generator := recurring.NewDomainGenerator()

	// Generate unique worker ID (hostname-uuid for uniqueness across restarts)
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "worker"
	}
	workerID := fmt.Sprintf("%s-%s", hostname, uuid.New().String()[:8])

	slog.InfoContext(ctx, "Worker starting", "worker_id", workerID)

	// Create GenerationWorker with default configuration
	generationCfg := worker.DefaultWorkerConfig(workerID)
	generationWorker := worker.NewGenerationWorker(coordinator, store, generator, generationCfg)

	// Create ReconciliationWorker with default configuration
	reconciliationCfg := worker.DefaultReconciliationConfig(workerID)
	reconciliationWorker := worker.NewReconciliationWorker(coordinator, store, generator, reconciliationCfg)

	// Start both workers concurrently
	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	// Start generation worker pool
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := runGenerationWorkerPool(ctx, generationWorker, generationCfg); err != nil {
			errChan <- fmt.Errorf("generation worker error: %w", err)
		}
	}()

	// Start reconciliation worker
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := reconciliationWorker.Run(ctx); err != nil {
			// Context cancellation is expected during shutdown
			if ctx.Err() == nil {
				errChan <- fmt.Errorf("reconciliation worker error: %w", err)
			}
		}
	}()

	// Wait for shutdown signal or worker errors
	select {
	case <-ctx.Done():
		slog.InfoContext(ctx, "Shutdown signal received, waiting for workers to finish...")
	case err := <-errChan:
		slog.ErrorContext(ctx, "Worker error occurred", "error", err)
		cancel() // Trigger shutdown of other workers
	}

	// Wait for all workers to complete gracefully
	wg.Wait()

	slog.InfoContext(ctx, "All workers shut down gracefully")
	return nil
}

// runGenerationWorkerPool runs multiple concurrent generation workers.
// Each worker polls for jobs and processes them with heartbeat and retry logic.
func runGenerationWorkerPool(ctx context.Context, worker *worker.GenerationWorker, cfg worker.WorkerConfig) error {
	var wg sync.WaitGroup

	// Start N concurrent workers
	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func(workerNum int) {
			defer wg.Done()
			runGenerationWorker(ctx, worker, cfg, workerNum)
		}(i)
	}

	// Wait for all workers to finish
	wg.Wait()
	return nil
}

// runGenerationWorker runs a single generation worker in a polling loop.
func runGenerationWorker(ctx context.Context, worker *worker.GenerationWorker, cfg worker.WorkerConfig, workerNum int) {
	slog.InfoContext(ctx, "Generation worker started", "worker_num", workerNum)

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "Generation worker stopping", "worker_num", workerNum)
			return
		case <-ticker.C:
			// Process one job (or none if queue is empty)
			if err := worker.RunProcessOnce(ctx); err != nil {
				// Only log infrastructure errors - job errors are handled internally
				slog.ErrorContext(ctx, "Generation worker error",
					"worker_num", workerNum,
					"error", err)
			}
		}
	}
}
