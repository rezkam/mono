package worker

import (
	"context"
	"log/slog"

	"github.com/rezkam/mono/internal/domain"
)

// ErrorHandler processes job errors and panics for telemetry/alerting.
// Allows custom integration with error tracking services (Sentry, Datadog, etc.).
//
// Pattern from River (https://riverqueue.com/docs/error-handling):
// - HandleError for normal errors (can influence retry behavior)
// - HandlePanic for panics (always sent to dead letter, no retries)
type ErrorHandler interface {
	// HandleError is called when a job returns an error.
	// Return nil to follow normal retry policy (retry if transient, dead letter if permanent).
	// Return &ErrorHandlerResult{SetCancelled: true} to force permanent failure.
	HandleError(ctx context.Context, job *domain.GenerationJob, err error) *ErrorHandlerResult

	// HandlePanic is called when a job panics. Includes panic value and stack trace.
	// Panics always go to dead letter (no retries) regardless of return value.
	// This is a hook for logging/telemetry only.
	HandlePanic(ctx context.Context, job *domain.GenerationJob, panicVal any, stackTrace string) *ErrorHandlerResult
}

// ErrorHandlerResult controls job behavior after error/panic.
type ErrorHandlerResult struct {
	// SetCancelled permanently fails the job, preventing further retries.
	// Use when the error is unrecoverable (e.g., invalid template ID).
	SetCancelled bool
}

// DefaultErrorHandler logs errors and panics with structured logging.
type DefaultErrorHandler struct{}

func (h *DefaultErrorHandler) HandleError(ctx context.Context, job *domain.GenerationJob, err error) *ErrorHandlerResult {
	slog.ErrorContext(ctx, "Generation job failed",
		slog.String("job_id", job.ID),
		slog.String("template_id", job.TemplateID),
		slog.Int("retry_count", job.RetryCount),
		slog.String("error", err.Error()),
		slog.Bool("retryable", IsRetryable(err)),
	)
	return nil // Follow normal retry policy
}

func (h *DefaultErrorHandler) HandlePanic(ctx context.Context, job *domain.GenerationJob, panicVal any, stackTrace string) *ErrorHandlerResult {
	slog.ErrorContext(ctx, "Generation job panicked",
		slog.String("job_id", job.ID),
		slog.String("template_id", job.TemplateID),
		slog.Any("panic_value", panicVal),
		slog.String("stack_trace", stackTrace),
	)
	return nil // Panics always go to dead letter
}
