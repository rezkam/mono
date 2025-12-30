package todo

import (
	"context"

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
}
