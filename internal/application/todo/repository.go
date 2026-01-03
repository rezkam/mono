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

	// ListLists retrieves todo lists with filtering, sorting, and pagination.
	// Items are fetched separately via FindItems to support pagination.
	ListLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error)

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

	// FindRecurringTemplate retrieves a template by ID.
	// Returns domain.ErrTemplateNotFound if template doesn't exist.
	FindRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error)

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

	// === Recurring Instance Operations ===

	// BatchInsertItemsIgnoreConflict inserts items in batch with conflict handling.
	// Duplicates based on (recurring_template_id, occurs_at) are silently ignored.
	// Returns count of successfully inserted items.
	BatchInsertItemsIgnoreConflict(ctx context.Context, items []*domain.TodoItem) (int, error)

	// DeleteFuturePendingItems deletes future pending items for a template.
	// Used before regeneration when pattern changes.
	// Returns count of deleted items.
	DeleteFuturePendingItems(ctx context.Context, templateID string, fromDate time.Time) (int64, error)

	// === Template Generation Tracking ===

	// FindStaleTemplates finds templates needing generation.
	// Returns templates where generated_through < target date.
	FindStaleTemplates(ctx context.Context, listID string, untilDate time.Time) ([]*domain.RecurringTemplate, error)

	// SetGeneratedThrough updates the generated_through marker after generation.
	SetGeneratedThrough(ctx context.Context, templateID string, generatedThrough time.Time) error

	// === Generation Job Operations ===

	// CreateGenerationJob creates a background generation job.
	// Used for async generation of future recurring instances.
	CreateGenerationJob(ctx context.Context, job *domain.GenerationJob) error

	// === Recurring Template Exception Operations ===

	// CreateException creates an exception to prevent regeneration or mark customization.
	// Returns domain.ErrExceptionAlreadyExists if exception for this occurrence exists.
	CreateException(ctx context.Context, exception *domain.RecurringTemplateException) (*domain.RecurringTemplateException, error)

	// ListExceptions retrieves exceptions for a template in date range.
	// Returns exceptions ordered by occurs_at.
	ListExceptions(ctx context.Context, templateID string, from, until time.Time) ([]*domain.RecurringTemplateException, error)

	// FindExceptionByOccurrence checks if exception exists for specific occurrence.
	// Returns domain.ErrExceptionNotFound if not exists.
	FindExceptionByOccurrence(ctx context.Context, templateID string, occursAt time.Time) (*domain.RecurringTemplateException, error)

	// DeleteException removes an exception (allows regeneration or template updates).
	// Returns domain.ErrExceptionNotFound if not exists.
	DeleteException(ctx context.Context, templateID string, occursAt time.Time) error

	// ListAllExceptionsByTemplate retrieves all exceptions for a template.
	ListAllExceptionsByTemplate(ctx context.Context, templateID string) ([]*domain.RecurringTemplateException, error)

	// === Transaction Support ===

	// Transaction executes multiple repository operations atomically.
	// All operations within fn succeed together or fail together.
	// If fn returns an error, all operations are rolled back.
	Transaction(ctx context.Context, fn func(tx Repository) error) error
}
