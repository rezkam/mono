package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rezkam/mono/internal/core"
	"github.com/rezkam/mono/internal/recurring"
)

// Worker handles recurring task generation by scheduling and processing jobs.
type Worker struct {
	store            core.Storage
	generator        *recurring.Generator
	scheduleInterval time.Duration
	processInterval  time.Duration
	done             chan struct{}
	wg               sync.WaitGroup
}

// Option is a functional option for configuring Worker.
type Option func(*Worker)

// WithScheduleInterval sets how often the worker schedules new jobs.
func WithScheduleInterval(d time.Duration) Option {
	return func(w *Worker) {
		w.scheduleInterval = d
	}
}

// WithProcessInterval sets how often the worker processes jobs from the queue.
func WithProcessInterval(d time.Duration) Option {
	return func(w *Worker) {
		w.processInterval = d
	}
}

// New creates a new Worker with the given storage and options.
func New(store core.Storage, opts ...Option) *Worker {
	w := &Worker{
		store:            store,
		generator:        recurring.NewGenerator(store),
		scheduleInterval: 1 * time.Hour,    // Default: schedule hourly
		processInterval:  30 * time.Second, // Default: process every 30s
		done:             make(chan struct{}),
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

// Start runs the worker with ticker loops for scheduling and processing.
// This is the production mode - runs until context is cancelled or Stop is called.
func (w *Worker) Start(ctx context.Context) error {
	log.Println("Worker started")
	log.Printf("- Job scheduler runs every %v", w.scheduleInterval)
	log.Printf("- Job processor runs every %v", w.processInterval)

	// Schedule jobs immediately on startup
	if err := w.RunScheduleOnce(ctx); err != nil {
		log.Printf("Error scheduling jobs on startup: %v", err)
	}

	scheduleTicker := time.NewTicker(w.scheduleInterval)
	processTicker := time.NewTicker(w.processInterval)
	defer scheduleTicker.Stop()
	defer processTicker.Stop()

	for {
		select {
		case <-scheduleTicker.C:
			w.wg.Add(1)
			go func() {
				defer w.wg.Done()
				if err := w.RunScheduleOnce(ctx); err != nil {
					log.Printf("Error scheduling jobs: %v", err)
				}
			}()
		case <-processTicker.C:
			w.wg.Add(1)
			go func() {
				defer w.wg.Done()
				if _, err := w.RunProcessOnce(ctx); err != nil {
					log.Printf("Error processing job: %v", err)
				}
			}()
		case <-ctx.Done():
			log.Println("Worker context cancelled, shutting down...")
			w.wg.Wait()
			return ctx.Err()
		case <-w.done:
			log.Println("Worker stopped")
			w.wg.Wait()
			return nil
		}
	}
}

// Stop gracefully stops the worker.
func (w *Worker) Stop() error {
	close(w.done)
	return nil
}

// RunScheduleOnce executes a single scheduling cycle.
// Creates generation jobs for templates that need them.
func (w *Worker) RunScheduleOnce(ctx context.Context) error {
	log.Println("Scheduling generation jobs...")

	templates, err := w.store.GetActiveTemplatesNeedingGeneration(ctx)
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
		// Pass time.Time{} for immediate scheduling (zero value means "schedule now").
		jobID, err := w.store.CreateGenerationJob(ctx, template.ID, time.Time{}, start, target)
		if err != nil {
			log.Printf("Failed to create job for template %s: %v", template.ID, err)
			continue
		}

		log.Printf("Created job %s for template %s (%s)", jobID, template.ID, template.Title)
	}

	return nil
}

// RunProcessOnce claims and processes a single generation job.
// Returns true if a job was processed, false if queue was empty.
func (w *Worker) RunProcessOnce(ctx context.Context) (bool, error) {
	// Claim next job using SKIP LOCKED
	jobID, err := w.store.ClaimNextGenerationJob(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to claim job: %w", err)
	}

	if jobID == "" {
		// No jobs available
		return false, nil
	}

	log.Printf("Claimed job %s", jobID)

	// Get job details
	job, err := w.store.GetGenerationJob(ctx, jobID)
	if err != nil {
		return false, fmt.Errorf("failed to get job details: %w", err)
	}

	// Get template
	template, err := w.store.GetRecurringTemplate(ctx, job.TemplateID)
	if err != nil {
		errMsg := fmt.Sprintf("template not found: %v", err)
		w.store.UpdateGenerationJobStatus(ctx, jobID, "FAILED", &errMsg)
		return false, fmt.Errorf("failed to get template: %w", err)
	}

	log.Printf("Processing job %s for template %s (%s)", jobID, template.ID, template.Title)

	// Generate tasks
	tasks, err := w.generator.GenerateTasksForTemplate(ctx, template, job.GenerateFrom, job.GenerateUntil)
	if err != nil {
		errMsg := fmt.Sprintf("generation failed: %v", err)
		w.store.UpdateGenerationJobStatus(ctx, jobID, "FAILED", &errMsg)
		return false, fmt.Errorf("failed to generate tasks: %w", err)
	}

	// Add tasks to list if any were generated
	// IMPORTANT: Use CreateTodoItem to preserve existing items and their status history
	if len(tasks) > 0 {
		for _, task := range tasks {
			if err := w.store.CreateTodoItem(ctx, template.ListID, task); err != nil {
				errMsg := fmt.Sprintf("failed to create task: %v", err)
				w.store.UpdateGenerationJobStatus(ctx, jobID, "FAILED", &errMsg)
				return false, fmt.Errorf("failed to create task: %w", err)
			}
		}

		log.Printf("Generated %d tasks for template %s", len(tasks), template.ID)
	}

	// Update template's last_generated_until
	if err := w.store.UpdateRecurringTemplateGenerationWindow(ctx, template.ID, job.GenerateUntil); err != nil {
		log.Printf("Warning: Failed to update template %s generation window: %v", template.ID, err)
		// Don't fail the job for this - tasks were created successfully
	}

	// Mark job as completed
	if err := w.store.UpdateGenerationJobStatus(ctx, jobID, "COMPLETED", nil); err != nil {
		log.Printf("Warning: Failed to mark job %s as completed: %v", jobID, err)
	}

	log.Printf("Completed job %s successfully", jobID)
	return true, nil
}
