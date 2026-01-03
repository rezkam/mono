package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/recurring"
)

// GenerationWorker processes async generation jobs with availability timeout and heartbeat.
// Uses GenerationCoordinator for job claiming, ownership verification, and stuck job recovery.
type GenerationWorker struct {
	coordinator  GenerationCoordinator
	repo         Repository
	generator    *recurring.DomainGenerator
	cfg          WorkerConfig
	errorHandler ErrorHandler
}

// NewGenerationWorker creates a worker with the given configuration.
func NewGenerationWorker(coordinator GenerationCoordinator, repo Repository, generator *recurring.DomainGenerator, cfg WorkerConfig) *GenerationWorker {
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = &DefaultErrorHandler{}
	}
	return &GenerationWorker{
		coordinator:  coordinator,
		repo:         repo,
		generator:    generator,
		cfg:          cfg,
		errorHandler: cfg.ErrorHandler,
	}
}

// RunProcessOnce claims and processes a single job with heartbeat and panic recovery.
// Returns nil if a job was processed (successfully or not), or if no jobs available.
// Only returns error for infrastructure failures that should stop the worker.
func (w *GenerationWorker) RunProcessOnce(ctx context.Context) error {
	job, err := w.coordinator.ClaimNextJob(ctx, w.cfg.WorkerID, w.cfg.AvailabilityTimeout)
	if err != nil {
		return fmt.Errorf("failed to claim job: %w", err)
	}
	if job == nil {
		return nil // No jobs available
	}

	slog.InfoContext(ctx, "claimed job", "job_id", job.ID, "worker_id", w.cfg.WorkerID)

	// Start heartbeat goroutine to extend availability
	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()
	go w.runHeartbeat(heartbeatCtx, job.ID)

	// Process with panic recovery
	err = w.executeWithRecovery(ctx, job)
	cancelHeartbeat() // Stop heartbeat

	if err != nil {
		return w.handleJobError(ctx, job, err)
	}

	// Job completed successfully
	err = w.coordinator.CompleteJob(ctx, job.ID, w.cfg.WorkerID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to mark job as completed", "job_id", job.ID, "error", err)
		return fmt.Errorf("failed to complete job: %w", err)
	}

	slog.InfoContext(ctx, "job completed successfully", "job_id", job.ID)
	return nil
}

// runHeartbeat periodically extends job availability to prevent reclamation.
// Runs until context is cancelled (when job completes or fails).
func (w *GenerationWorker) runHeartbeat(ctx context.Context, jobID string) {
	ticker := time.NewTicker(w.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.coordinator.ExtendAvailability(ctx, jobID, w.cfg.WorkerID, w.cfg.AvailabilityTimeout); err != nil {
				slog.WarnContext(ctx, "heartbeat failed", "job_id", jobID, "error", err)
			}
		}
	}
}

// executeWithRecovery processes job with panic recovery.
// If the job panics, captures stack trace and converts to PanicError.
func (w *GenerationWorker) executeWithRecovery(ctx context.Context, job *domain.GenerationJob) (err error) {
	defer func() {
		r := recover()
		if r != nil {
			stackTrace := string(debug.Stack())
			w.errorHandler.HandlePanic(ctx, job, r, stackTrace)
			err = PanicError{Value: r, StackTrace: stackTrace}
		}
	}()
	return w.processJob(ctx, job)
}

// processJob does the actual generation work.
// Generates tasks in batches with progressive marker updates.
func (w *GenerationWorker) processJob(ctx context.Context, job *domain.GenerationJob) error {
	template, err := w.repo.GetRecurringTemplate(ctx, job.TemplateID)
	if errors.Is(err, domain.ErrTemplateNotFound) {
		// Template was deleted - cancel job permanently
		return JobCancelled{Reason: "template no longer exists"}
	}
	if err != nil {
		return Transient(err) // Database error - retry
	}

	slog.InfoContext(ctx, "processing job",
		"job_id", job.ID,
		"template_id", template.ID,
		"template_title", template.Title,
		"generate_from", job.GenerateFrom,
		"generate_until", job.GenerateUntil)

	// Fetch exceptions for the entire generation range
	exceptions, err := w.repo.ListExceptions(ctx, job.TemplateID, job.GenerateFrom, job.GenerateUntil)
	if err != nil {
		return Transient(err) // Database error - retry
	}

	// Generate in batches to handle large date ranges
	batchSize := w.cfg.GenerationBatchDays
	current := job.GenerateFrom

	for current.Before(job.GenerateUntil) {
		// Check for context cancellation between batches
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Calculate batch end date
		batchEnd := current.AddDate(0, 0, batchSize)
		if batchEnd.After(job.GenerateUntil) {
			batchEnd = job.GenerateUntil
		}

		// Generate tasks for this batch with exception filtering
		items, err := w.generator.GenerateTasksForTemplateWithExceptions(ctx, template, current, batchEnd, exceptions)
		if err != nil {
			return fmt.Errorf("failed to generate tasks: %w", err)
		}

		if len(items) > 0 {
			// Insert with duplicate detection
			inserted, err := w.repo.BatchInsertItemsIgnoreConflict(ctx, items)
			if err != nil {
				return Transient(err) // Database error - retry
			}

			slog.InfoContext(ctx, "batch generated",
				"job_id", job.ID,
				"batch_start", current,
				"batch_end", batchEnd,
				"generated_count", len(items),
				"inserted_count", inserted)
		}

		// Update generated_through marker progressively
		// This ensures partial progress is saved even if job fails mid-way
		if err := w.repo.SetGeneratedThrough(ctx, template.ID, batchEnd); err != nil {
			slog.WarnContext(ctx, "failed to update generated_through marker",
				"template_id", template.ID,
				"batch_end", batchEnd,
				"error", err)
			// Don't fail the job for marker updates - continue processing
		}

		current = batchEnd
	}

	return nil
}

// handleJobError routes errors to appropriate handling (retry, dead letter, etc.)
// Returns nil if error was handled successfully, or error if handling failed.
func (w *GenerationWorker) handleJobError(ctx context.Context, job *domain.GenerationJob, err error) error {
	// Notify error handler
	w.errorHandler.HandleError(ctx, job, err)

	// Panics go directly to dead letter (no retry)
	if IsPanic(err) {
		panicErr := err.(PanicError)
		slog.ErrorContext(ctx, "job panicked",
			"job_id", job.ID,
			"panic_value", panicErr.Value)

		if dlErr := w.coordinator.MoveToDeadLetter(ctx, job, w.cfg.WorkerID, "panic", panicErr.Error(), &panicErr.StackTrace); dlErr != nil {
			if errors.Is(dlErr, domain.ErrJobOwnershipLost) {
				slog.WarnContext(ctx, "job ownership lost during panic handling - another worker may have reclaimed",
					"job_id", job.ID)
				return nil // Another worker is handling it
			}
			return fmt.Errorf("failed to move panicked job to dead letter: %w", dlErr)
		}
		return nil // Handled
	}

	// Cancelled jobs go to dead letter
	if IsJobCancelled(err) {
		slog.WarnContext(ctx, "job cancelled",
			"job_id", job.ID,
			"reason", err.Error())

		if dlErr := w.coordinator.MoveToDeadLetter(ctx, job, w.cfg.WorkerID, "permanent", err.Error(), nil); dlErr != nil {
			if errors.Is(dlErr, domain.ErrJobOwnershipLost) {
				slog.WarnContext(ctx, "job ownership lost during cancellation handling - another worker may have reclaimed",
					"job_id", job.ID)
				return nil // Another worker is handling it
			}
			return fmt.Errorf("failed to move cancelled job to dead letter: %w", dlErr)
		}
		return nil // Handled
	}

	// Only retry transient errors
	if IsRetryable(err) {
		willRetry, failErr := w.coordinator.FailJob(ctx, job.ID, w.cfg.WorkerID, err.Error(), w.cfg.RetryConfig)
		if failErr != nil {
			return fmt.Errorf("failed to schedule retry: %w", failErr)
		}

		if !willRetry {
			// Max retries exceeded - coordinator moved to dead letter queue atomically
			slog.WarnContext(ctx, "job exhausted retries",
				"job_id", job.ID,
				"retry_count", job.RetryCount,
				"error", err.Error())
			return nil // Handled
		}

		slog.InfoContext(ctx, "job scheduled for retry",
			"job_id", job.ID,
			"retry_count", job.RetryCount+1,
			"error", err.Error())
		return nil // Will be retried
	}

	// Non-transient error (permanent) - dead letter immediately
	slog.ErrorContext(ctx, "job failed with permanent error",
		"job_id", job.ID,
		"error", err.Error())

	if dlErr := w.coordinator.MoveToDeadLetter(ctx, job, w.cfg.WorkerID, "permanent", err.Error(), nil); dlErr != nil {
		if errors.Is(dlErr, domain.ErrJobOwnershipLost) {
			slog.WarnContext(ctx, "job ownership lost during error handling - another worker may have reclaimed",
				"job_id", job.ID)
			return nil // Another worker is handling it
		}
		return fmt.Errorf("failed to move failed job to dead letter: %w", dlErr)
	}
	return nil // Handled
}
