package todo

import (
	"context"

	"github.com/rezkam/mono/internal/domain"
)

// Repository defines storage operations for todo management.
type Repository interface {
	// === List Operations ===

	// CreateList creates a new todo list.
	CreateList(ctx context.Context, list *domain.TodoList) error

	// FindListByID retrieves a todo list by its ID, including all items.
	// Returns error if list not found.
	FindListByID(ctx context.Context, id string) (*domain.TodoList, error)

	// FindAllLists retrieves all todo lists with counts but without items.
	FindAllLists(ctx context.Context) ([]*domain.TodoList, error)

	// FindLists retrieves todo lists with filtering, sorting, and pagination.
	FindLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error)

	// UpdateList updates an existing todo list metadata (title, etc).
	// Returns error if list not found.
	UpdateList(ctx context.Context, list *domain.TodoList) error

	// === Item Operations ===

	// CreateItem creates a new todo item in a list.
	// Returns error if list not found.
	CreateItem(ctx context.Context, listID string, item *domain.TodoItem) error

	// FindItemByID retrieves a single todo item by its ID.
	// Returns domain.ErrNotFound if item doesn't exist.
	FindItemByID(ctx context.Context, id string) (*domain.TodoItem, error)

	// UpdateItem updates an existing todo item.
	// Validates that the item belongs to the specified list (prevents cross-list updates).
	// Returns domain.ErrNotFound if item doesn't exist or doesn't belong to the list.
	UpdateItem(ctx context.Context, listID string, item *domain.TodoItem) error

	// FindItems searches for items with filtering, sorting, and pagination.
	FindItems(ctx context.Context, params domain.ListTasksParams) (*domain.PagedResult, error)

	// === Recurring Template Operations ===

	// CreateRecurringTemplate creates a new recurring task template.
	// Returns error if list not found or invalid config.
	CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) error

	// FindRecurringTemplate retrieves a template by ID.
	// Returns error if template not found.
	FindRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error)

	// UpdateRecurringTemplate updates an existing template.
	// Returns error if template not found or invalid config.
	UpdateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) error

	// DeleteRecurringTemplate deletes a template.
	// Returns error if template not found.
	DeleteRecurringTemplate(ctx context.Context, id string) error

	// FindRecurringTemplates lists all templates for a list, optionally filtered by active status.
	FindRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*domain.RecurringTemplate, error)
}
