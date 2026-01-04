package worker

import (
	"context"
	"time"

	"github.com/rezkam/mono/internal/domain"
)

// Repository defines storage operations for recurring task worker.
type Repository interface {
	// === Template Operations ===

	// FindActiveTemplatesNeedingGeneration retrieves all active templates that need task generation.
	FindActiveTemplatesNeedingGeneration(ctx context.Context) ([]*domain.RecurringTemplate, error)

	// FindRecurringTemplateByID retrieves a single recurring template by ID.
	// Returns error if template not found.
	FindRecurringTemplateByID(ctx context.Context, id string) (*domain.RecurringTemplate, error)

	// UpdateRecurringTemplateGenerationWindow updates the last_generated_until timestamp.
	// Returns error if template not found.
	UpdateRecurringTemplateGenerationWindow(ctx context.Context, id string, until time.Time) error

	// === Job Operations ===

	// ScheduleGenerationJob schedules a new background job for generating recurring tasks.
	// scheduledFor: when to run the job (zero value = immediate)
	// from/until: date range for task generation
	// Returns the created job ID or error if creation fails.
	ScheduleGenerationJob(ctx context.Context, templateID string, scheduledFor, from, until time.Time) (string, error)

	// FindGenerationJobByID retrieves job details by ID.
	// Returns error if job not found.
	FindGenerationJobByID(ctx context.Context, id string) (*domain.GenerationJob, error)

	// UpdateGenerationJobStatus updates job status and optionally records an error.
	// status: "pending", "running", "completed", "failed"
	// errorMessage: nil for successful completion, non-nil for failures
	// Returns error if job not found.
	UpdateGenerationJobStatus(ctx context.Context, id, status string, errorMessage *string) error

	// HasPendingOrRunningJob checks if a template already has a pending or running job.
	// Returns true if such a job exists, false otherwise.
	HasPendingOrRunningJob(ctx context.Context, templateID string) (bool, error)

	// === Item Creation ===

	// CreateTodoItem creates a new todo item in a list.
	CreateTodoItem(ctx context.Context, listID string, item *domain.TodoItem) error

	// BatchCreateTodoItems creates multiple todo items in a single operation.
	// Returns the number of items created.
	BatchCreateTodoItems(ctx context.Context, listID string, items []domain.TodoItem) (int64, error)

	// BatchInsertItemsIgnoreConflict inserts items in batch with conflict handling.
	// Duplicates based on (recurring_template_id, occurs_at) are silently ignored.
	// Returns count of successfully inserted items.
	BatchInsertItemsIgnoreConflict(ctx context.Context, items []*domain.TodoItem) (int, error)

	// SetGeneratedThrough updates the generated_through marker after generation.
	SetGeneratedThrough(ctx context.Context, templateID string, generatedThrough time.Time) error

	// === Exception Operations ===

	// FindExceptions retrieves exceptions for a template in date range.
	// Used during generation to filter out deleted/edited instances.
	FindExceptions(ctx context.Context, templateID string, from, until time.Time) ([]*domain.RecurringTemplateException, error)

	// === Reconciliation Operations ===

	// FindStaleTemplatesForReconciliation retrieves templates needing reconciliation.
	// Excludes templates with pending/running jobs if ExcludePending is true.
	// Excludes templates updated after UpdatedBefore (grace period).
	// Returns templates where generated_through < TargetDate.
	FindStaleTemplatesForReconciliation(ctx context.Context, params FindStaleParams) ([]*domain.RecurringTemplate, error)

	// DeleteFuturePendingItems deletes all pending items for a template that occur after the specified date.
	// Used during reconciliation to remove items that shouldn't exist beyond the template's generated_through marker.
	// Returns count of deleted items.
	DeleteFuturePendingItems(ctx context.Context, templateID string, after time.Time) (int64, error)
}
