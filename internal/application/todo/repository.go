package todo

import (
	"context"
	"time"

	"github.com/rezkam/mono/internal/domain"
)

// Repository defines storage operations for todo management.
// All create/update operations return the entity as persisted, including version.
type Repository interface {
	// === List Operations ===

	// CreateList creates a new todo list.
	// Returns the created list with version populated by persistence layer.
	CreateList(ctx context.Context, list *domain.TodoList) (*domain.TodoList, error)

	// FindListByID retrieves a todo list by its ID with metadata and counts.
	// Items are fetched separately via FindItems to support pagination.
	// Returns domain.ErrListNotFound if list doesn't exist.
	FindListByID(ctx context.Context, id string) (*domain.TodoList, error)

	// FindLists retrieves todo lists with filtering, sorting, and pagination.
	// Items are fetched separately via FindItems to support pagination.
	FindLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error)

	// UpdateList updates a list using field mask.
	// Only updates fields specified in UpdateMask.
	// Returns the updated list with new version.
	// Returns domain.ErrListNotFound if list doesn't exist.
	// Returns domain.ErrVersionConflict if etag is provided and doesn't match current version.
	UpdateList(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error)

	// === Item Operations ===

	// CreateItem creates a new todo item in a list.
	// Returns the created item with version populated by persistence layer.
	// Returns domain.ErrListNotFound if list doesn't exist.
	CreateItem(ctx context.Context, listID string, item *domain.TodoItem) (*domain.TodoItem, error)

	// FindItemByID retrieves a single todo item by its ID.
	// Returns domain.ErrItemNotFound if item doesn't exist.
	FindItemByID(ctx context.Context, id string) (*domain.TodoItem, error)

	// UpdateItem updates an item using field mask and optional etag.
	// Only updates fields specified in UpdateMask.
	// Returns the updated item with new version.
	// Returns domain.ErrItemNotFound if item doesn't exist.
	// Returns domain.ErrVersionConflict if etag is provided and doesn't match current version.
	UpdateItem(ctx context.Context, params domain.UpdateItemParams) (*domain.TodoItem, error)

	// FindItems searches for items with filtering, sorting, and pagination.
	// excludedStatuses is provided by service layer based on business rules.
	FindItems(ctx context.Context, params domain.ListTasksParams, excludedStatuses []domain.TaskStatus) (*domain.PagedResult, error)

	// === Recurring Template Operations ===

	// CreateRecurringTemplate creates a new recurring task template.
	// Returns the created template with version populated by persistence layer.
	// Returns domain.ErrListNotFound if list doesn't exist.
	CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) (*domain.RecurringTemplate, error)

	// FindRecurringTemplateByID retrieves a template by ID.
	// Returns domain.ErrTemplateNotFound if template doesn't exist.
	FindRecurringTemplateByID(ctx context.Context, id string) (*domain.RecurringTemplate, error)

	// UpdateRecurringTemplate updates a template using field mask.
	// Only updates fields specified in UpdateMask.
	// Returns the updated template with new version.
	// Returns domain.ErrTemplateNotFound if template doesn't exist.
	// Returns domain.ErrVersionConflict if etag is provided and doesn't match current version.
	UpdateRecurringTemplate(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error)

	// DeleteRecurringTemplate deletes a template.
	// Returns domain.ErrTemplateNotFound if template doesn't exist.
	DeleteRecurringTemplate(ctx context.Context, id string) error

	// FindRecurringTemplates lists all templates for a list, optionally filtered by active status.
	FindRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*domain.RecurringTemplate, error)

	// FindExceptions retrieves exceptions for a template in a date range.
	// Used for displaying edited/deleted occurrences to users.
	// Returns domain.ErrTemplateNotFound if template doesn't exist.
	FindExceptions(ctx context.Context, templateID string, from, until time.Time) ([]*domain.RecurringTemplateException, error)

	// CreateException creates a new recurring template exception.
	// Returns domain.ErrExceptionAlreadyExists if exception already exists for this occurrence.
	CreateException(ctx context.Context, exception *domain.RecurringTemplateException) (*domain.RecurringTemplateException, error)

	// DeleteFuturePendingItems deletes all pending items for a template that occur after the specified date.
	// Used during template updates to remove items that shouldn't exist beyond the template's generated_through marker.
	// Returns count of deleted items.
	DeleteFuturePendingItems(ctx context.Context, templateID string, after time.Time) (int64, error)

	// BatchInsertItemsIgnoreConflict inserts items in batch with conflict handling.
	// Duplicates based on (recurring_template_id, occurs_at) are silently ignored.
	// Returns count of successfully inserted items.
	BatchInsertItemsIgnoreConflict(ctx context.Context, items []*domain.TodoItem) (int, error)

	// SetGeneratedThrough updates the generated_through marker after generation.
	SetGeneratedThrough(ctx context.Context, templateID string, generatedThrough time.Time) error

	// CreateGenerationJob creates a new background job for generating recurring tasks.
	// Must be called within an Atomic transaction to ensure atomicity with data operations.
	CreateGenerationJob(ctx context.Context, job *domain.GenerationJob) error

	// === Atomic Operations ===

	// Atomic executes a callback function within a database transaction.
	// All operations inside the callback succeed together or fail together.
	// The callback receives a Repository instance that operates within the transaction.
	// Commits the transaction if callback returns nil, rolls back if callback returns an error.
	Atomic(ctx context.Context, fn func(repo Repository) error) error
}
