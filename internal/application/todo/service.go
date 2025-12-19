// Package todo provides the application layer for todo management.
//
// ARCHITECTURE DECISION: Application Layer
//
// This layer contains ALL business logic and use case orchestration.
// It is protocol-agnostic - no knowledge of gRPC, HTTP, CLI, or any delivery mechanism.
//
// RESPONSIBILITIES:
//   - Business logic and validation
//   - Use case orchestration (coordinating multiple domain operations)
//   - Defining repository interfaces (Dependency Inversion)
//   - Returning domain models and domain errors
//
// WHAT DOES NOT BELONG HERE:
//   - Protocol-specific code (protobuf, HTTP headers, gRPC status codes)
//   - Database implementation details (SQL queries, connection management)
//   - Infrastructure concerns (caching, logging, metrics)
//
// LAYER DEPENDENCIES:
//   - Domain models (internal/domain) - pure business entities
//   - Repository interface (defined here, implemented in infrastructure layer)
//   - NO dependencies on gRPC, HTTP, database drivers, or infrastructure
//
// This enables:
//   - Same business logic used by gRPC handlers, REST endpoints, CLI commands, and workers
//   - Easy testing without infrastructure overhead
//   - Clean separation of concerns
package todo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
)

// Service provides business logic for todo management.
// It orchestrates operations using the Repository interface.
type Service struct {
	repo Repository
}

// NewService creates a new todo service.
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
	}
}

// CreateList creates a new todo list.
func (s *Service) CreateList(ctx context.Context, title string) (*domain.TodoList, error) {
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	idObj, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate id: %w", err)
	}

	list := &domain.TodoList{
		ID:         idObj.String(),
		Title:      title,
		Items:      []domain.TodoItem{},
		CreateTime: time.Now().UTC(),
	}

	if err := s.repo.CreateList(ctx, list); err != nil {
		return nil, fmt.Errorf("failed to create list: %w", err)
	}

	return list, nil
}

// GetList retrieves a todo list by ID.
func (s *Service) GetList(ctx context.Context, id string) (*domain.TodoList, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	list, err := s.repo.FindListByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get list: %w", err)
	}

	return list, nil
}

// ListLists retrieves all todo lists.
func (s *Service) ListLists(ctx context.Context) ([]*domain.TodoList, error) {
	lists, err := s.repo.FindAllLists(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list lists: %w", err)
	}

	return lists, nil
}

// UpdateList updates an existing todo list.
func (s *Service) UpdateList(ctx context.Context, list *domain.TodoList) error {
	if list.ID == "" {
		return fmt.Errorf("list ID is required")
	}

	if err := s.repo.UpdateList(ctx, list); err != nil {
		return fmt.Errorf("failed to update list: %w", err)
	}

	return nil
}

// CreateItem creates a new todo item in a list.
func (s *Service) CreateItem(ctx context.Context, listID string, item *domain.TodoItem) (*domain.TodoItem, error) {
	if listID == "" {
		return nil, fmt.Errorf("list_id is required")
	}
	if item.Title == "" {
		return nil, fmt.Errorf("title is required")
	}

	// Generate ID if not provided
	if item.ID == "" {
		idObj, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("failed to generate id: %w", err)
		}
		item.ID = idObj.String()
	}

	// Set timestamps
	now := time.Now().UTC()
	item.CreateTime = now
	item.UpdatedAt = now

	// Set default status if not provided
	if item.Status == "" {
		item.Status = domain.TaskStatusTodo
	}

	// Validate timezone if provided
	if item.Timezone != nil && *item.Timezone != "" {
		if _, err := time.LoadLocation(*item.Timezone); err != nil {
			return nil, fmt.Errorf("invalid timezone: %w", err)
		}
	}

	if err := s.repo.CreateItem(ctx, listID, item); err != nil {
		return nil, fmt.Errorf("failed to create item: %w", err)
	}

	return item, nil
}

// GetItem retrieves a single todo item by ID.
func (s *Service) GetItem(ctx context.Context, id string) (*domain.TodoItem, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	item, err := s.repo.FindItemByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get item: %w", err)
	}

	return item, nil
}

// UpdateItem updates an existing todo item.
// Validates that the item belongs to the specified list.
func (s *Service) UpdateItem(ctx context.Context, listID string, item *domain.TodoItem) error {
	if item.ID == "" {
		return fmt.Errorf("item ID is required")
	}
	if listID == "" {
		return fmt.Errorf("list ID is required")
	}

	// Validate timezone if provided
	if item.Timezone != nil && *item.Timezone != "" {
		if _, err := time.LoadLocation(*item.Timezone); err != nil {
			return fmt.Errorf("invalid timezone: %w", err)
		}
	}

	// Update timestamp
	item.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateItem(ctx, listID, item); err != nil {
		return fmt.Errorf("failed to update item: %w", err)
	}

	return nil
}

// ListTasks searches for tasks with filtering, sorting, and pagination.
func (s *Service) ListTasks(ctx context.Context, params domain.ListTasksParams) (*domain.PagedResult, error) {
	// Validate order_by field
	if params.OrderBy != "" {
		validFields := map[string]bool{
			"due_time":   true,
			"priority":   true,
			"created_at": true,
			"updated_at": true,
		}
		if !validFields[params.OrderBy] {
			return nil, fmt.Errorf("invalid order_by field: %s (supported: due_time, priority, created_at, updated_at)", params.OrderBy)
		}
	}

	result, err := s.repo.FindItems(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	return result, nil
}

// CreateRecurringTemplate creates a new recurring task template.
func (s *Service) CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) (*domain.RecurringTemplate, error) {
	if template.ListID == "" {
		return nil, fmt.Errorf("list_id is required")
	}
	if template.Title == "" {
		return nil, fmt.Errorf("title is required")
	}

	// Generate ID if not provided
	if template.ID == "" {
		idObj, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("failed to generate id: %w", err)
		}
		template.ID = idObj.String()
	}

	// Set timestamps and defaults
	now := time.Now().UTC()
	template.CreatedAt = now
	template.UpdatedAt = now
	template.LastGeneratedUntil = now
	template.IsActive = true

	// Default generation window
	if template.GenerationWindowDays == 0 {
		template.GenerationWindowDays = 30
	}

	if err := s.repo.CreateRecurringTemplate(ctx, template); err != nil {
		return nil, fmt.Errorf("failed to create template: %w", err)
	}

	return template, nil
}

// GetRecurringTemplate retrieves a recurring template by ID.
func (s *Service) GetRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	template, err := s.repo.FindRecurringTemplate(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}

	return template, nil
}

// UpdateRecurringTemplate updates an existing recurring template.
func (s *Service) UpdateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) error {
	if template.ID == "" {
		return fmt.Errorf("template ID is required")
	}

	// Update timestamp
	template.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateRecurringTemplate(ctx, template); err != nil {
		return fmt.Errorf("failed to update template: %w", err)
	}

	return nil
}

// DeleteRecurringTemplate deletes a recurring template.
func (s *Service) DeleteRecurringTemplate(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}

	if err := s.repo.DeleteRecurringTemplate(ctx, id); err != nil {
		return fmt.Errorf("failed to delete template: %w", err)
	}

	return nil
}

// ListRecurringTemplates lists recurring templates for a list.
func (s *Service) ListRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*domain.RecurringTemplate, error) {
	if listID == "" {
		return nil, fmt.Errorf("list_id is required")
	}

	templates, err := s.repo.FindRecurringTemplates(ctx, listID, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("failed to list templates: %w", err)
	}

	return templates, nil
}
