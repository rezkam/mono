package worker

import (
	"context"
	"time"

	"github.com/rezkam/mono/internal/domain"
)

// Repository defines storage operations for recurring task worker.
//
// This interface is owned by the worker package (consumer), not by the storage package (provider).
// Following the Dependency Inversion Principle and Interface Segregation Principle.
//
// Interface Segregation: Only ~9 methods needed by worker for job scheduling and processing.
type Repository interface {
	// === Template Operations ===

	// GetActiveTemplatesNeedingGeneration retrieves all active templates that need task generation.
	// This is used during the scheduling phase to create generation jobs.
	// Returns error if database error occurs.
	GetActiveTemplatesNeedingGeneration(ctx context.Context) ([]*domain.RecurringTemplate, error)

	// GetRecurringTemplate retrieves a single recurring template by ID.
	// Used during job processing to get template details for task generation.
	// Returns error if template not found or database error occurs.
	GetRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error)

	// UpdateRecurringTemplateGenerationWindow updates the last_generated_until timestamp.
	// Called after successfully generating tasks to track progress.
	// Returns error if template not found or database error occurs.
	UpdateRecurringTemplateGenerationWindow(ctx context.Context, id string, until time.Time) error

	// === Job Operations ===

	// CreateGenerationJob creates a new background job for generating recurring tasks.
	// scheduledFor: when to run the job (zero value = immediate)
	// from/until: date range for task generation
	// Returns the created job ID or error if creation fails.
	CreateGenerationJob(ctx context.Context, templateID string, scheduledFor, from, until time.Time) (string, error)

	// ClaimNextGenerationJob atomically claims the next pending job using SKIP LOCKED.
	// Returns empty string if no jobs available, job ID if claimed successfully.
	// Returns error only for database errors (not for empty queue).
	ClaimNextGenerationJob(ctx context.Context) (string, error)

	// GetGenerationJob retrieves job details by ID.
	// Used after claiming a job to get the generation parameters.
	// Returns error if job not found or database error occurs.
	GetGenerationJob(ctx context.Context, id string) (*domain.GenerationJob, error)

	// UpdateGenerationJobStatus updates job status and optionally records an error.
	// status: "PENDING", "RUNNING", "COMPLETED", "FAILED"
	// errorMessage: nil for successful completion, non-nil for failures
	// Returns error if job not found or database error occurs.
	UpdateGenerationJobStatus(ctx context.Context, id, status string, errorMessage *string) error

	// HasPendingOrRunningJob checks if a template already has a pending or running job.
	// Used during scheduling to prevent creating duplicate jobs for the same template.
	// Returns true if such a job exists, false otherwise.
	HasPendingOrRunningJob(ctx context.Context, templateID string) (bool, error)

	// === Item Creation ===

	// CreateTodoItem creates a new todo item in a list.
	// Used by worker to create task instances generated from recurring templates.
	// Preserves status history via database triggers.
	// Returns error if list not found or database error occurs.
	CreateTodoItem(ctx context.Context, listID string, item *domain.TodoItem) error
}
