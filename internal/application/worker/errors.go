package worker

import (
	"errors"
	"fmt"
)

// === Retry Classification ===

// RetryableError wraps transient errors that should be retried.
// Only errors wrapped with Transient() will be retried; all other errors
// are treated as permanent and go directly to dead letter queue.
//
// Use for: network timeouts, database connection lost, temporary locks, rate limits.
// Don't use for: validation errors, not found errors, business logic failures.
type RetryableError struct {
	Err error
}

func (e RetryableError) Error() string { return e.Err.Error() }
func (e RetryableError) Unwrap() error { return e.Err }

// Transient wraps an error to signal it should be retried.
// Use for: network timeouts, DB connection lost, temporary locks.
//
// Example:
//
//	if err := db.Query(...); err != nil {
//	    return worker.Transient(err)  // Will retry with exponential backoff
//	}
func Transient(err error) error {
	return RetryableError{Err: err}
}

// IsRetryable returns true if the error should be retried.
func IsRetryable(err error) bool {
	var retryable RetryableError
	return errors.As(err, &retryable)
}

// === Panic Handling ===

// PanicError indicates a panic occurred during job processing.
// Jobs that panic are sent directly to dead letter queue (no retries).
// Panics indicate programming errors, not transient issues.
type PanicError struct {
	Value      any
	StackTrace string
}

func (e PanicError) Error() string {
	return fmt.Sprintf("panic: %v", e.Value)
}

// IsPanic returns true if the error indicates a panic occurred.
func IsPanic(err error) bool {
	var panicErr PanicError
	return errors.As(err, &panicErr)
}

// === Worker-Initiated Cancellation ===

// JobCancelled indicates the job should be permanently cancelled (no retries).
// Return this from a worker to stop processing without retry attempts.
// Use when the job is determined to be unrecoverable (e.g., template deleted).
//
// Example:
//
//	template, err := repo.FindTemplate(ctx, job.TemplateID)
//	if errors.Is(err, domain.ErrTemplateNotFound) {
//	    return worker.JobCancelled{Reason: "template no longer exists"}
//	}
type JobCancelled struct {
	Reason string
}

func (e JobCancelled) Error() string {
	return fmt.Sprintf("job cancelled: %s", e.Reason)
}

// IsJobCancelled returns true if the error indicates intentional cancellation.
func IsJobCancelled(err error) bool {
	var cancelled JobCancelled
	return errors.As(err, &cancelled)
}
