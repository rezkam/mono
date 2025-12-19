package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rezkam/mono/internal/recurring"
)

// Worker handles recurring task generation by scheduling and processing jobs.
type Worker struct {
	repo             Repository
	generator        *recurring.DomainGenerator
	scheduleInterval time.Duration
	processInterval  time.Duration
	operationTimeout time.Duration // Timeout for individual storage operations
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

// WithOperationTimeout sets the timeout for individual storage operations.
func WithOperationTimeout(d time.Duration) Option {
	return func(w *Worker) {
		w.operationTimeout = d
	}
}

// New creates a new Worker with the given repository and options.
func New(repo Repository, opts ...Option) *Worker {
	w := &Worker{
		repo:             repo,
		generator:        recurring.NewDomainGenerator(repo),
		scheduleInterval: 1 * time.Hour,    // Default: schedule hourly
		processInterval:  30 * time.Second, // Default: process every 30s
		operationTimeout: 30 * time.Second, // Default: 30s timeout for storage operations
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

	templates, err := w.repo.GetActiveTemplatesNeedingGeneration(ctx)
	if err != nil {
		return fmt.Errorf("failed to get active templates: %w", err)
	}

	log.Printf("Found %d templates needing generation", len(templates))

	for _, template := range templates {
		// Check for existing pending/running job to prevent duplicates
		hasJob, err := w.repo.HasPendingOrRunningJob(ctx, template.ID)
		if err != nil {
			log.Printf("Failed to check for existing job for template %s: %v", template.ID, err)
			continue
		}
		if hasJob {
			log.Printf("Template %s already has a pending/running job, skipping", template.ID)
			continue
		}

		// Calculate generation window
		start := template.LastGeneratedUntil
		if start.IsZero() {
			start = template.CreatedAt
		}

		target := time.Now().AddDate(0, 0, template.GenerationWindowDays)

		// Compare at the date level since last_generated_until is stored as DATE in the database.
		// The database column only stores year/month/day, so we need to truncate both values
		// to dates for a fair comparison. Without this, a stored date of "2026-06-16" would
		// be read as midnight UTC, while target calculated as "2026-06-16 22:30" would appear
		// to need generation.
		startDate := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		targetDate := time.Date(target.Year(), target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)

		if !startDate.Before(targetDate) {
			continue // Already covered (same day or later)
		}

		// Create a job for this template
		// Pass time.Time{} for immediate scheduling (zero value means "schedule now").
		jobID, err := w.repo.CreateGenerationJob(ctx, template.ID, time.Time{}, start, target)
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
	claimCtx, cancel := context.WithTimeout(ctx, w.operationTimeout)
	jobID, err := w.repo.ClaimNextGenerationJob(claimCtx)
	cancel()
	if err != nil {
		return false, fmt.Errorf("failed to claim job: %w", err)
	}

	if jobID == "" {
		// No jobs available
		return false, nil
	}

	log.Printf("Claimed job %s", jobID)

	// Get job details
	getJobCtx, cancel := context.WithTimeout(ctx, w.operationTimeout)
	job, err := w.repo.GetGenerationJob(getJobCtx, jobID)
	cancel()
	if err != nil {
		return false, fmt.Errorf("failed to get job details: %w", err)
	}

	// Get template
	getTemplateCtx, cancel := context.WithTimeout(ctx, w.operationTimeout)
	template, err := w.repo.GetRecurringTemplate(getTemplateCtx, job.TemplateID)
	cancel()
	if err != nil {
		errMsg := fmt.Sprintf("template not found: %v", err)
		updateCtx, cancel := context.WithTimeout(ctx, w.operationTimeout)
		updateErr := w.repo.UpdateGenerationJobStatus(updateCtx, jobID, "FAILED", &errMsg)
		cancel()
		if updateErr != nil {
			log.Printf("ERROR: Failed to mark job %s as FAILED after template error: %v", jobID, updateErr)
			return false, fmt.Errorf("failed to get template: %w (additionally, failed to update job status: %v)", err, updateErr)
		}
		return false, fmt.Errorf("failed to get template: %w", err)
	}

	log.Printf("Processing job %s for template %s (%s)", jobID, template.ID, template.Title)

	// Generate tasks (may involve storage operations, so apply timeout)
	genCtx, cancel := context.WithTimeout(ctx, w.operationTimeout)
	tasks, err := w.generator.GenerateTasksForTemplate(genCtx, template, job.GenerateFrom, job.GenerateUntil)
	cancel()
	if err != nil {
		errMsg := fmt.Sprintf("generation failed: %v", err)
		updateCtx, cancel := context.WithTimeout(ctx, w.operationTimeout)
		updateErr := w.repo.UpdateGenerationJobStatus(updateCtx, jobID, "FAILED", &errMsg)
		cancel()
		if updateErr != nil {
			log.Printf("ERROR: Failed to mark job %s as FAILED after generation error: %v", jobID, updateErr)
			return false, fmt.Errorf("failed to generate tasks: %w (additionally, failed to update job status: %v)", err, updateErr)
		}
		return false, fmt.Errorf("failed to generate tasks: %w", err)
	}

	// Add tasks to list if any were generated
	// IMPORTANT: Use CreateTodoItem to preserve existing items and their status history
	if len(tasks) > 0 {
		for _, task := range tasks {
			createCtx, cancel := context.WithTimeout(ctx, w.operationTimeout)
			err := w.repo.CreateTodoItem(createCtx, template.ListID, &task)
			cancel()
			if err != nil {
				errMsg := fmt.Sprintf("failed to create task: %v", err)
				updateCtx, cancel := context.WithTimeout(ctx, w.operationTimeout)
				updateErr := w.repo.UpdateGenerationJobStatus(updateCtx, jobID, "FAILED", &errMsg)
				cancel()
				if updateErr != nil {
					log.Printf("ERROR: Failed to mark job %s as FAILED after task creation error: %v", jobID, updateErr)
					return false, fmt.Errorf("failed to create task: %w (additionally, failed to update job status: %v)", err, updateErr)
				}
				return false, fmt.Errorf("failed to create task: %w", err)
			}
		}

		log.Printf("Generated %d tasks for template %s", len(tasks), template.ID)
	}

	// Update template's last_generated_until
	updateWindowCtx, cancel := context.WithTimeout(ctx, w.operationTimeout)
	err = w.repo.UpdateRecurringTemplateGenerationWindow(updateWindowCtx, template.ID, job.GenerateUntil)
	cancel()
	if err != nil {
		log.Printf("Warning: Failed to update template %s generation window: %v", template.ID, err)
		// Don't fail the job for this - tasks were created successfully
	}

	// Mark job as completed
	completeCtx, cancel := context.WithTimeout(ctx, w.operationTimeout)
	err = w.repo.UpdateGenerationJobStatus(completeCtx, jobID, "COMPLETED", nil)
	cancel()
	if err != nil {
		// Tasks were created but job status update failed - job will be stuck in RUNNING state
		log.Printf("ERROR: Failed to mark job %s as COMPLETED (tasks were created, job stuck in RUNNING): %v", jobID, err)
		return false, fmt.Errorf("tasks created successfully but failed to mark job as completed: %w", err)
	}

	log.Printf("Completed job %s successfully", jobID)
	return true, nil
}
