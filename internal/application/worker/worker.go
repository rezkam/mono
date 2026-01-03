package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/rezkam/mono/internal/recurring"
)

// Worker handles scheduling recurring task generation jobs.
// Job processing is handled by GenerationWorker using the coordinator pattern.
type Worker struct {
	repo             Repository
	generator        *recurring.DomainGenerator
	scheduleInterval time.Duration
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
		generator:        recurring.NewDomainGenerator(),
		scheduleInterval: 1 * time.Hour,    // Default: schedule hourly
		operationTimeout: 30 * time.Second, // Default: 30s timeout for storage operations
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

// Start runs the worker with a ticker loop for scheduling.
// Job processing is handled separately by GenerationWorker.
// Runs until context is cancelled. On shutdown:
// 1. Stops accepting new work
// 2. Waits for in-flight operations to complete
// 3. Returns nil
func (w *Worker) Start(ctx context.Context) error {
	slog.InfoContext(ctx, "Job scheduler started", "interval", w.scheduleInterval)

	// Schedule jobs immediately on startup
	startupCtx, startupCancel := context.WithTimeout(context.Background(), w.operationTimeout)
	if err := w.RunScheduleOnce(startupCtx); err != nil {
		slog.ErrorContext(startupCtx, "Error scheduling jobs on startup", "error", err)
	}
	startupCancel() // releases timer resources

	scheduleTicker := time.NewTicker(w.scheduleInterval)
	defer scheduleTicker.Stop()

	for {
		select {
		case <-scheduleTicker.C:
			w.wg.Go(func() {
				opCtx, cancel := context.WithTimeout(context.Background(), w.operationTimeout)
				defer cancel()
				if err := w.RunScheduleOnce(opCtx); err != nil {
					slog.ErrorContext(opCtx, "Error scheduling jobs", "error", err)
				}
			})
		case <-ctx.Done():
			slog.InfoContext(ctx, "Shutdown requested, waiting for in-flight operations...")
			w.wg.Wait()
			slog.InfoContext(ctx, "Job scheduler stopped gracefully")
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
		start := template.GeneratedThrough
		if start.IsZero() {
			start = template.CreatedAt
		}

		target := time.Now().UTC().AddDate(0, 0, template.GenerationHorizonDays)

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
		jobID, err := w.repo.ScheduleGenerationJob(ctx, template.ID, time.Time{}, start, target)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to create job for template", "template_id", template.ID, "error", err)
			continue
		}

		slog.InfoContext(ctx, "Created generation job", "job_id", jobID, "template_id", template.ID, "template_title", template.Title)
	}

	return nil
}
