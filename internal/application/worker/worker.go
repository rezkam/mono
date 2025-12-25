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
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

// Start runs the worker with ticker loops for scheduling and processing.
// Runs until context is cancelled. On shutdown:
// 1. Stops accepting new work
// 2. Waits for in-flight operations to complete
// 3. Returns nil
func (w *Worker) Start(ctx context.Context) error {
	log.Println("Worker started")
	log.Printf("- Job scheduler runs every %v", w.scheduleInterval)
	log.Printf("- Job processor runs every %v", w.processInterval)

	// Schedule jobs immediately on startup
	startupCtx, startupCancel := context.WithTimeout(context.Background(), w.operationTimeout)
	if err := w.RunScheduleOnce(startupCtx); err != nil {
		log.Printf("Error scheduling jobs on startup: %v", err)
	}
	startupCancel() // releases timer resources

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
				opCtx, cancel := context.WithTimeout(context.Background(), w.operationTimeout)
				defer cancel()
				if err := w.RunScheduleOnce(opCtx); err != nil {
					log.Printf("Error scheduling jobs: %v", err)
				}
			}()
		case <-processTicker.C:
			w.wg.Add(1)
			go func() {
				defer w.wg.Done()
				opCtx, cancel := context.WithTimeout(context.Background(), w.operationTimeout)
				defer cancel()
				if _, err := w.RunProcessOnce(opCtx); err != nil {
					log.Printf("Error processing job: %v", err)
				}
			}()
		case <-ctx.Done():
			log.Println("Shutdown requested, waiting for in-flight operations...")
			w.wg.Wait()
			log.Println("Worker stopped gracefully")
			return nil
		}
	}
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

		target := time.Now().UTC().AddDate(0, 0, template.GenerationWindowDays)

		// Compare at the date level since last_generated_until stores only year/month/day.
		// We need to truncate both values to dates for a fair comparison.
		// Without this, a stored date of "2026-06-16" would be read as midnight UTC,
		// while target calculated as "2026-06-16 22:30" would appear
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
// The caller is responsible for providing a context with appropriate timeout.
func (w *Worker) RunProcessOnce(ctx context.Context) (bool, error) {
	jobID, err := w.repo.ClaimNextGenerationJob(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to claim job: %w", err)
	}

	if jobID == "" {
		return false, nil
	}

	log.Printf("Claimed job %s", jobID)

	job, err := w.repo.GetGenerationJob(ctx, jobID)
	if err != nil {
		return false, fmt.Errorf("failed to get job details: %w", err)
	}

	template, err := w.repo.GetRecurringTemplate(ctx, job.TemplateID)
	if err != nil {
		errMsg := fmt.Sprintf("template not found: %v", err)
		updateErr := w.repo.UpdateGenerationJobStatus(ctx, jobID, "failed", &errMsg)
		if updateErr != nil {
			log.Printf("ERROR: Failed to mark job %s as failed after template error: %v", jobID, updateErr)
			return false, fmt.Errorf("failed to get template: %w (additionally, failed to update job status: %v)", err, updateErr)
		}
		return false, fmt.Errorf("failed to get template: %w", err)
	}

	log.Printf("Processing job %s for template %s (%s)", jobID, template.ID, template.Title)

	tasks, err := w.generator.GenerateTasksForTemplate(ctx, template, job.GenerateFrom, job.GenerateUntil)
	if err != nil {
		errMsg := fmt.Sprintf("generation failed: %v", err)
		updateErr := w.repo.UpdateGenerationJobStatus(ctx, jobID, "failed", &errMsg)
		if updateErr != nil {
			log.Printf("ERROR: Failed to mark job %s as failed after generation error: %v", jobID, updateErr)
			return false, fmt.Errorf("failed to generate tasks: %w (additionally, failed to update job status: %v)", err, updateErr)
		}
		return false, fmt.Errorf("failed to generate tasks: %w", err)
	}

	if len(tasks) > 0 {
		for _, task := range tasks {
			err := w.repo.CreateTodoItem(ctx, template.ListID, &task)
			if err != nil {
				errMsg := fmt.Sprintf("failed to create task: %v", err)
				updateErr := w.repo.UpdateGenerationJobStatus(ctx, jobID, "failed", &errMsg)
				if updateErr != nil {
					log.Printf("ERROR: Failed to mark job %s as failed after task creation error: %v", jobID, updateErr)
					return false, fmt.Errorf("failed to create task: %w (additionally, failed to update job status: %v)", err, updateErr)
				}
				return false, fmt.Errorf("failed to create task: %w", err)
			}
		}
		log.Printf("Generated %d tasks for template %s", len(tasks), template.ID)
	}

	err = w.repo.UpdateRecurringTemplateGenerationWindow(ctx, template.ID, job.GenerateUntil)
	if err != nil {
		log.Printf("Warning: Failed to update template %s generation window: %v", template.ID, err)
	}

	err = w.repo.UpdateGenerationJobStatus(ctx, jobID, "completed", nil)
	if err != nil {
		log.Printf("ERROR: Failed to mark job %s as completed (tasks were created, job stuck in running): %v", jobID, err)
		return false, fmt.Errorf("tasks created successfully but failed to mark job as completed: %w", err)
	}

	log.Printf("Completed job %s successfully", jobID)
	return true, nil
}
