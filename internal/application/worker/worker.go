package worker

import (
	"context"
	"fmt"
	"log/slog"
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
	slog.InfoContext(ctx, "Worker started")
	slog.InfoContext(ctx, "Job scheduler configuration", "interval", w.scheduleInterval)
	slog.InfoContext(ctx, "Job processor configuration", "interval", w.processInterval)

	// Schedule jobs immediately on startup
	startupCtx, startupCancel := context.WithTimeout(context.Background(), w.operationTimeout)
	if err := w.RunScheduleOnce(startupCtx); err != nil {
		slog.ErrorContext(startupCtx, "Error scheduling jobs on startup", "error", err)
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
					slog.ErrorContext(opCtx, "Error scheduling jobs", "error", err)
				}
			}()
		case <-processTicker.C:
			w.wg.Add(1)
			go func() {
				defer w.wg.Done()
				opCtx, cancel := context.WithTimeout(context.Background(), w.operationTimeout)
				defer cancel()
				if _, err := w.RunProcessOnce(opCtx); err != nil {
					slog.ErrorContext(opCtx, "Error processing job", "error", err)
				}
			}()
		case <-ctx.Done():
			slog.InfoContext(ctx, "Shutdown requested, waiting for in-flight operations...")
			w.wg.Wait()
			slog.InfoContext(ctx, "Worker stopped gracefully")
			return nil
		}
	}
}

// RunScheduleOnce executes a single scheduling cycle.
// Creates generation jobs for templates that need them.
func (w *Worker) RunScheduleOnce(ctx context.Context) error {
	slog.InfoContext(ctx, "Scheduling generation jobs...")

	templates, err := w.repo.GetActiveTemplatesNeedingGeneration(ctx)
	if err != nil {
		return fmt.Errorf("failed to get active templates: %w", err)
	}

	slog.InfoContext(ctx, "Found templates needing generation", "count", len(templates))

	for _, template := range templates {
		// Check for existing pending/running job to prevent duplicates
		hasJob, err := w.repo.HasPendingOrRunningJob(ctx, template.ID)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to check for existing job", "template_id", template.ID, "error", err)
			continue
		}
		if hasJob {
			slog.InfoContext(ctx, "Template already has pending/running job, skipping", "template_id", template.ID)
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
			slog.ErrorContext(ctx, "Failed to create job for template", "template_id", template.ID, "error", err)
			continue
		}

		slog.InfoContext(ctx, "Created generation job", "job_id", jobID, "template_id", template.ID, "template_title", template.Title)
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

	slog.InfoContext(ctx, "Claimed job", "job_id", jobID)

	job, err := w.repo.GetGenerationJob(ctx, jobID)
	if err != nil {
		return false, fmt.Errorf("failed to get job details: %w", err)
	}

	template, err := w.repo.GetRecurringTemplate(ctx, job.TemplateID)
	if err != nil {
		errMsg := fmt.Sprintf("template not found: %v", err)
		updateErr := w.repo.UpdateGenerationJobStatus(ctx, jobID, "failed", &errMsg)
		if updateErr != nil {
			slog.ErrorContext(ctx, "Failed to mark job as failed after template error", "job_id", jobID, "error", updateErr)
			return false, fmt.Errorf("failed to get template: %w (additionally, failed to update job status: %v)", err, updateErr)
		}
		return false, fmt.Errorf("failed to get template: %w", err)
	}

	slog.InfoContext(ctx, "Processing job", "job_id", jobID, "template_id", template.ID, "template_title", template.Title)

	tasks, err := w.generator.GenerateTasksForTemplate(ctx, template, job.GenerateFrom, job.GenerateUntil)
	if err != nil {
		errMsg := fmt.Sprintf("generation failed: %v", err)
		updateErr := w.repo.UpdateGenerationJobStatus(ctx, jobID, "failed", &errMsg)
		if updateErr != nil {
			slog.ErrorContext(ctx, "Failed to mark job as failed after generation error", "job_id", jobID, "error", updateErr)
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
					slog.ErrorContext(ctx, "Failed to mark job as failed after task creation error", "job_id", jobID, "error", updateErr)
					return false, fmt.Errorf("failed to create task: %w (additionally, failed to update job status: %v)", err, updateErr)
				}
				return false, fmt.Errorf("failed to create task: %w", err)
			}
		}
		slog.InfoContext(ctx, "Generated tasks for template", "count", len(tasks), "template_id", template.ID)
	}

	err = w.repo.UpdateRecurringTemplateGenerationWindow(ctx, template.ID, job.GenerateUntil)
	if err != nil {
		slog.WarnContext(ctx, "Failed to update template generation window", "template_id", template.ID, "error", err)
	}

	err = w.repo.UpdateGenerationJobStatus(ctx, jobID, "completed", nil)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to mark job as completed (tasks were created, job stuck in running)", "job_id", jobID, "error", err)
		return false, fmt.Errorf("tasks created successfully but failed to mark job as completed: %w", err)
	}

	slog.InfoContext(ctx, "Completed job successfully", "job_id", jobID)
	return true, nil
}
