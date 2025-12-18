package main

import (
	"context"
	"fmt"
	"log"
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

	log.Println("Recurring task worker started")
	log.Println("- Job scheduler runs every hour")
	log.Println("- Job processor runs every 30 seconds")

	// Schedule jobs immediately on startup
	if err := scheduleJobs(ctx, store); err != nil {
		log.Printf("Error scheduling jobs: %v", err)
	}

	for {
		select {
		case <-scheduleTicker.C:
			log.Println("Scheduling generation jobs...")
			if err := scheduleJobs(ctx, store); err != nil {
				log.Printf("Error scheduling jobs: %v", err)
			}
		case <-processTicker.C:
			// Process jobs (non-blocking - just claims one job at a time)
			if err := processNextJob(ctx, generator, store); err != nil {
				log.Printf("Error processing job: %v", err)
			}
		case <-sigChan:
			log.Println("Received shutdown signal, exiting...")
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

	log.Printf("Found %d templates needing generation", len(templates))

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
			log.Printf("Failed to create job for template %s: %v", template.ID, err)
			continue
		}

		log.Printf("Created job %s for template %s (%s)", jobID, template.ID, template.Title)
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

	log.Printf("Claimed job %s", jobID)

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

	log.Printf("Processing job %s for template %s (%s)", jobID, template.ID, template.Title)

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

		log.Printf("Generated %d tasks for template %s", len(tasks), template.ID)
	}

	// Update template's last_generated_until
	if err := store.UpdateRecurringTemplateGenerationWindow(ctx, template.ID, job.GenerateUntil); err != nil {
		log.Printf("Warning: Failed to update template %s generation window: %v", template.ID, err)
		// Don't fail the job for this - tasks were created successfully
	}

	// Mark job as completed
	if err := store.UpdateGenerationJobStatus(ctx, jobID, "COMPLETED", nil); err != nil {
		log.Printf("Warning: Failed to mark job %s as completed: %v", jobID, err)
	}

	log.Printf("Completed job %s successfully", jobID)
	return nil
}
