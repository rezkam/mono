package worker

import (
	"context"
	"time"

	"github.com/rezkam/mono/internal/domain"
)

// Repository defines storage operations for recurring task worker.
type Repository interface {
	// === Template Operations ===

	// GetActiveTemplatesNeedingGeneration retrieves all active templates that need task generation.
	GetActiveTemplatesNeedingGeneration(ctx context.Context) ([]*domain.RecurringTemplate, error)

	// GetRecurringTemplate retrieves a single recurring template by ID.
	// Returns error if template not found.
	GetRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error)

	// UpdateRecurringTemplateGenerationWindow updates the last_generated_until timestamp.
	// Returns error if template not found.
	UpdateRecurringTemplateGenerationWindow(ctx context.Context, id string, until time.Time) error

	// === Job Operations ===

	// CreateGenerationJob creates a new background job for generating recurring tasks.
	// scheduledFor: when to run the job (zero value = immediate)
	// from/until: date range for task generation
	// Returns the created job ID or error if creation fails.
	CreateGenerationJob(ctx context.Context, templateID string, scheduledFor, from, until time.Time) (string, error)

	// ClaimNextGenerationJob atomically claims the next pending job.
	// Returns empty string if no jobs available, job ID if claimed successfully.
	// Returns error only on failure (not for empty queue).
	ClaimNextGenerationJob(ctx context.Context) (string, error)

	// GetGenerationJob retrieves job details by ID.
	// Returns error if job not found.
	GetGenerationJob(ctx context.Context, id string) (*domain.GenerationJob, error)

	// UpdateGenerationJobStatus updates job status and optionally records an error.
	// status: "PENDING", "RUNNING", "COMPLETED", "FAILED"
	// errorMessage: nil for successful completion, non-nil for failures
	// Returns error if job not found.
	UpdateGenerationJobStatus(ctx context.Context, id, status string, errorMessage *string) error

	// HasPendingOrRunningJob checks if a template already has a pending or running job.
	// Returns true if such a job exists, false otherwise.
	HasPendingOrRunningJob(ctx context.Context, templateID string) (bool, error)

	// === Item Creation ===

	// CreateTodoItem creates a new todo item in a list.
	// Returns error if list not found.
	CreateTodoItem(ctx context.Context, listID string, item *domain.TodoItem) error
}
