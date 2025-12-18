package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rezkam/mono/internal/core"
	"github.com/rezkam/mono/internal/recurring"
	sqlstorage "github.com/rezkam/mono/internal/storage/sql"
)

func main() {
	ctx := context.Background()

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

	// Create generator
	generator := recurring.NewGenerator(store)

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create tickers for scheduling and processing
	scheduleTicker := time.NewTicker(1 * time.Hour)   // Schedule new jobs hourly
	processTicker := time.NewTicker(30 * time.Second) // Process jobs every 30 seconds
	defer scheduleTicker.Stop()
	defer processTicker.Stop()

	slog.InfoContext(ctx, "Recurring task worker started")
	slog.InfoContext(ctx, "Job scheduler runs every hour")
	slog.InfoContext(ctx, "Job processor runs every 30 seconds")

	// Schedule jobs immediately on startup
	if err := scheduleJobs(ctx, store); err != nil {
		slog.ErrorContext(ctx, "Error scheduling jobs on startup", "error", err)
	}

	for {
		select {
		case <-scheduleTicker.C:
			slog.InfoContext(ctx, "Scheduling generation jobs")
			if err := scheduleJobs(ctx, store); err != nil {
				slog.ErrorContext(ctx, "Error scheduling jobs", "error", err)
			}
		case <-processTicker.C:
			// Process jobs (non-blocking - just claims one job at a time)
			if err := processNextJob(ctx, generator, store); err != nil {
				slog.ErrorContext(ctx, "Error processing job", "error", err)
			}
		case <-sigChan:
			slog.InfoContext(ctx, "Received shutdown signal, exiting")
			return
		}
	}
}

// scheduleJobs creates generation jobs for templates that need them.
func scheduleJobs(ctx context.Context, store core.Storage) error {
	templates, err := store.GetActiveTemplatesNeedingGeneration(ctx)
	if err != nil {
		return fmt.Errorf("failed to get active templates: %w", err)
	}

	slog.InfoContext(ctx, "Found templates needing generation", "count", len(templates))

	for _, template := range templates {
		// Calculate generation window
		start := template.LastGeneratedUntil
		if start.IsZero() {
			start = template.CreatedAt
		}

		target := time.Now().AddDate(0, 0, template.GenerationWindowDays)

		if start.After(target) || start.Equal(target) {
			continue // Already covered
		}

		// Create a job for this template
		jobID, err := store.CreateGenerationJob(ctx, template.ID, time.Now(), start, target)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to create job for template",
				"template_id", template.ID,
				"template_title", template.Title,
				"error", err)
			continue
		}

		slog.InfoContext(ctx, "Created generation job",
			"job_id", jobID,
			"template_id", template.ID,
			"template_title", template.Title)
	}

	return nil
}

// processNextJob claims and processes a single generation job.
func processNextJob(ctx context.Context, generator *recurring.Generator, store core.Storage) error {
	// Claim next job using SKIP LOCKED
	jobID, err := store.ClaimNextGenerationJob(ctx)
	if err != nil {
		return fmt.Errorf("failed to claim job: %w", err)
	}

	if jobID == "" {
		// No jobs available
		return nil
	}

	slog.InfoContext(ctx, "Claimed generation job", "job_id", jobID)

	// Get job details
	job, err := store.GetGenerationJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job details: %w", err)
	}

	// Get template
	template, err := store.GetRecurringTemplate(ctx, job.TemplateID)
	if err != nil {
		errMsg := fmt.Sprintf("template not found: %v", err)
		store.UpdateGenerationJobStatus(ctx, jobID, "FAILED", &errMsg)
		return fmt.Errorf("failed to get template: %w", err)
	}

	slog.InfoContext(ctx, "Processing generation job",
		"job_id", jobID,
		"template_id", template.ID,
		"template_title", template.Title)

	// Generate tasks
	tasks, err := generator.GenerateTasksForTemplate(ctx, template, job.GenerateFrom, job.GenerateUntil)
	if err != nil {
		errMsg := fmt.Sprintf("generation failed: %v", err)
		store.UpdateGenerationJobStatus(ctx, jobID, "FAILED", &errMsg)
		return fmt.Errorf("failed to generate tasks: %w", err)
	}

	// Add tasks to list if any were generated
	if len(tasks) > 0 {
		list, err := store.GetList(ctx, template.ListID)
		if err != nil {
			errMsg := fmt.Sprintf("list not found: %v", err)
			store.UpdateGenerationJobStatus(ctx, jobID, "FAILED", &errMsg)
			return fmt.Errorf("failed to get list: %w", err)
		}

		for _, task := range tasks {
			list.AddItem(task)
		}

		if err := store.UpdateList(ctx, list); err != nil {
			errMsg := fmt.Sprintf("failed to save tasks: %v", err)
			store.UpdateGenerationJobStatus(ctx, jobID, "FAILED", &errMsg)
			return fmt.Errorf("failed to update list: %w", err)
		}

		slog.InfoContext(ctx, "Generated tasks from template",
			"count", len(tasks),
			"template_id", template.ID,
			"template_title", template.Title)
	}

	// Update template's last_generated_until
	if err := store.UpdateRecurringTemplateGenerationWindow(ctx, template.ID, job.GenerateUntil); err != nil {
		slog.WarnContext(ctx, "Failed to update template generation window",
			"template_id", template.ID,
			"error", err)
		// Don't fail the job for this - tasks were created successfully
	}

	// Mark job as completed
	if err := store.UpdateGenerationJobStatus(ctx, jobID, "COMPLETED", nil); err != nil {
		slog.WarnContext(ctx, "Failed to mark job as completed",
			"job_id", jobID,
			"error", err)
	}

	slog.InfoContext(ctx, "Completed generation job successfully", "job_id", jobID)
	return nil
}
