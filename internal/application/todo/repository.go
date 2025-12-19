package todo

import (
	"context"

	"github.com/rezkam/mono/internal/domain"
)

// Repository defines storage operations for todo management.
//
// This interface is owned by the todo package (consumer), not by the storage package (provider).
// Following the Dependency Inversion Principle and Interface Segregation Principle.
//
// Interface Segregation: Only ~9 methods needed by todo service for list and item operations.
type Repository interface {
	// === List Operations ===

	// CreateList creates a new todo list.
	// Returns error if creation fails (e.g., database error).
	CreateList(ctx context.Context, list *domain.TodoList) error

	// FindListByID retrieves a todo list by its ID, including all items.
	// Returns error if list not found or database error occurs.
	FindListByID(ctx context.Context, id string) (*domain.TodoList, error)

	// FindAllLists retrieves all todo lists with counts but without items.
	// Use this for dashboard/list views where item details aren't needed.
	// Returns error if database error occurs.
	FindAllLists(ctx context.Context) ([]*domain.TodoList, error)

	// UpdateList updates an existing todo list metadata (title, etc).
	// Returns error if list not found or database error occurs.
	UpdateList(ctx context.Context, list *domain.TodoList) error

	// === Item Operations ===

	// CreateItem creates a new todo item in a list.
	// Preserves status history via database triggers.
	// Returns error if list not found or database error occurs.
	CreateItem(ctx context.Context, listID string, item *domain.TodoItem) error

	// FindItemByID retrieves a single todo item by its ID.
	// O(1) lookup - use this instead of FindListByID when only the item is needed.
	// Returns domain.ErrNotFound if item doesn't exist.
	FindItemByID(ctx context.Context, id string) (*domain.TodoItem, error)

	// UpdateItem updates an existing todo item.
	// Validates that the item belongs to the specified list (prevents cross-list updates).
	// Preserves status history via database triggers.
	// Returns domain.ErrNotFound if item doesn't exist or doesn't belong to the list.
	UpdateItem(ctx context.Context, listID string, item *domain.TodoItem) error

	// FindItems searches for items with filtering, sorting, and pagination.
	// All filtering happens at the database level for performance.
	// Returns error if database error occurs.
	FindItems(ctx context.Context, params domain.ListTasksParams) (*domain.PagedResult, error)

	// === Recurring Template Operations ===

	// CreateRecurringTemplate creates a new recurring task template.
	// Returns error if list not found, invalid config, or database error occurs.
	CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) error

	// FindRecurringTemplate retrieves a template by ID.
	// Returns error if template not found or database error occurs.
	FindRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error)

	// UpdateRecurringTemplate updates an existing template.
	// Returns error if template not found, invalid config, or database error occurs.
	UpdateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) error

	// DeleteRecurringTemplate deletes a template.
	// Returns error if template not found or database error occurs.
	DeleteRecurringTemplate(ctx context.Context, id string) error

	// FindRecurringTemplates lists all templates for a list, optionally filtered by active status.
	// Returns error if database error occurs.
	FindRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*domain.RecurringTemplate, error)
}
