// Package todo provides the application layer for todo management.
//
// ARCHITECTURE DECISION: Application Layer
//
// This layer contains ALL business logic and use case orchestration.
// It is protocol-agnostic - no knowledge of HTTP, CLI, or any delivery mechanism.
//
// RESPONSIBILITIES:
//   - Business logic and validation
//   - Use case orchestration (coordinating multiple domain operations)
//   - Defining repository interfaces (Dependency Inversion)
//   - Returning domain models and domain errors
//
// WHAT DOES NOT BELONG HERE:
//   - Protocol-specific code (HTTP headers, status codes, request/response formats)
//   - Database implementation details (SQL queries, connection management)
//   - Infrastructure concerns (caching, logging, metrics)
//
// LAYER DEPENDENCIES:
//   - Domain models (internal/domain) - pure business entities
//   - Repository interface (defined here, implemented in infrastructure layer)
//   - NO dependencies on HTTP, database drivers, or infrastructure
//
// This enables:
//   - Same business logic used by HTTP handlers, CLI commands, and workers
//   - Easy testing without infrastructure overhead
//   - Clean separation of concerns
package todo

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/ptr"
)

// Default configuration values.
const (
	DefaultPageSize = 25
	MaxPageSize     = 100
)

// Config holds configuration for the Service.
type Config struct {
	DefaultPageSize int
	MaxPageSize     int
}

// Service provides business logic for todo management.
// It orchestrates operations using the Repository interface.
type Service struct {
	repo   Repository
	config Config
}

// NewService creates a new todo service.
// Applies application defaults for zero or invalid config values.
// Both DefaultPageSize and MaxPageSize must be > 0.
func NewService(repo Repository, config Config) *Service {
	// Apply defaults for zero or invalid values (must be > 0)
	if config.DefaultPageSize <= 0 {
		config.DefaultPageSize = DefaultPageSize
	}
	if config.MaxPageSize <= 0 {
		config.MaxPageSize = MaxPageSize
	}

	return &Service{
		repo:   repo,
		config: config,
	}
}

// CreateList creates a new todo list.
func (s *Service) CreateList(ctx context.Context, titleStr string) (*domain.TodoList, error) {
	// Validate title using value object
	title, err := domain.NewTitle(titleStr)
	if err != nil {
		return nil, err // Returns domain error (ErrTitleRequired or ErrTitleTooLong)
	}

	idObj, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate id: %w", err)
	}

	list := &domain.TodoList{
		ID:         idObj.String(),
		Title:      title.String(),
		Items:      []domain.TodoItem{},
		CreateTime: time.Now().UTC(),
	}

	if err := s.repo.CreateList(ctx, list); err != nil {
		return nil, fmt.Errorf("failed to create list: %w", err)
	}

	return list, nil
}

// GetList retrieves a todo list by ID with all items populated.
func (s *Service) GetList(ctx context.Context, id string) (*domain.TodoList, error) {
	if id == "" {
		return nil, domain.ErrListNotFound
	}

	list, err := s.repo.FindListByID(ctx, id)
	if err != nil {
		return nil, err // Repository returns domain errors
	}

	return list, nil
}

// ListLists retrieves todo lists with filtering, sorting, and pagination.
// Returns summaries with counts only (Items field will be empty).
func (s *Service) ListLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error) {
	// Reject negative offsets to prevent database errors
	if params.Offset < 0 {
		params.Offset = 0
	}

	// Apply default page size if not specified or invalid
	if params.Limit <= 0 {
		params.Limit = s.config.DefaultPageSize
	}
	// Enforce maximum page size
	params.Limit = min(params.Limit, s.config.MaxPageSize)

	result, err := s.repo.ListLists(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list lists: %w", err)
	}

	return result, nil
}

// UpdateList updates a list using field mask.
// Only updates fields specified in UpdateMask.
func (s *Service) UpdateList(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error) {
	if params.ListID == "" {
		return nil, domain.ErrListNotFound
	}

	// Validate title if being updated
	if params.Title != nil {
		title, err := domain.NewTitle(*params.Title)
		if err != nil {
			return nil, err
		}
		params.Title = ptr.To(title.String())
	}

	return s.repo.UpdateList(ctx, params)
}

// CreateItem creates a new todo item in a list.
func (s *Service) CreateItem(ctx context.Context, listID string, item *domain.TodoItem) (*domain.TodoItem, error) {
	if listID == "" {
		return nil, domain.ErrListNotFound
	}

	// Validate title using value object
	title, err := domain.NewTitle(item.Title)
	if err != nil {
		return nil, err // Returns domain error (ErrTitleRequired or ErrTitleTooLong)
	}
	item.Title = title.String()

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

	// Ensure DueTime is UTC if provided
	if item.DueTime != nil {
		utc := item.DueTime.UTC()
		item.DueTime = &utc
	}

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
		return nil, domain.ErrItemNotFound
	}

	item, err := s.repo.FindItemByID(ctx, id)
	if err != nil {
		return nil, err // Repository returns domain errors
	}

	return item, nil
}

// UpdateItem updates an item using field mask and optional etag for OCC.
// Only updates fields specified in UpdateMask.
// If etag is provided and doesn't match, returns domain.ErrVersionConflict.
func (s *Service) UpdateItem(ctx context.Context, params domain.UpdateItemParams) (*domain.TodoItem, error) {
	if params.ItemID == "" {
		return nil, domain.ErrItemNotFound
	}
	if params.ListID == "" {
		return nil, domain.ErrListNotFound
	}

	// Validate etag format if provided
	if params.Etag != nil {
		etag := *params.Etag
		// Etag should be a numeric string (version number)
		version, err := strconv.Atoi(etag)
		if err != nil || version < 1 {
			return nil, domain.ErrInvalidEtagFormat
		}
	}

	// Build mask set for validation
	maskSet := make(map[string]bool)
	for _, field := range params.UpdateMask {
		maskSet[field] = true
	}

	// Validate title if being updated
	if params.Title != nil {
		title, err := domain.NewTitle(*params.Title)
		if err != nil {
			return nil, err
		}
		titleStr := title.String()
		params.Title = &titleStr
	}

	// Validate status - required field cannot be set to NULL
	// If status is in update_mask, a value must be provided
	if maskSet["status"] && params.Status == nil {
		return nil, domain.ErrStatusRequired
	}
	if params.Status != nil {
		if _, err := domain.NewTaskStatus(string(*params.Status)); err != nil {
			return nil, err
		}
	}

	// Validate priority if being updated
	if params.Priority != nil {
		if _, err := domain.NewTaskPriority(string(*params.Priority)); err != nil {
			return nil, err
		}
	}

	// Validate timezone if being updated
	if params.Timezone != nil && *params.Timezone != "" {
		if _, err := time.LoadLocation(*params.Timezone); err != nil {
			return nil, fmt.Errorf("invalid timezone: %w", err)
		}
	}

	return s.repo.UpdateItem(ctx, params)
}

// ListItems searches for items with filtering, sorting, and pagination.
// Filter is already validated via ItemsFilter value object.
// Applies business rules (pagination limits, default exclusions) and delegates to repository.
func (s *Service) ListItems(ctx context.Context, params domain.ListTasksParams) (*domain.PagedResult, error) {
	// Reject negative offsets to prevent database errors
	if params.Offset < 0 {
		params.Offset = 0
	}

	// Apply default limit if not specified or negative
	if params.Limit <= 0 {
		params.Limit = s.config.DefaultPageSize
	}

	// Enforce maximum page size
	params.Limit = min(params.Limit, s.config.MaxPageSize)

	// Business rule: when no explicit status filter, exclude archived and cancelled
	excludedStatuses := []domain.TaskStatus{}
	if !params.Filter.HasStatusFilter() {
		excludedStatuses = domain.DefaultExcludedStatuses()
	}

	result, err := s.repo.FindItems(ctx, params, excludedStatuses)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	return result, nil
}

// CreateRecurringTemplate creates a new recurring task template.
func (s *Service) CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) (*domain.RecurringTemplate, error) {
	if template.ListID == "" {
		return nil, domain.ErrListNotFound
	}

	// Validate title using value object
	title, err := domain.NewTitle(template.Title)
	if err != nil {
		return nil, err
	}
	template.Title = title.String()

	// Validate recurrence pattern
	pattern, err := domain.NewRecurrencePattern(string(template.RecurrencePattern))
	if err != nil {
		return nil, err
	}
	template.RecurrencePattern = pattern

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

	// Validate generation window using domain validation
	if err := domain.ValidateGenerationWindowDays(template.GenerationWindowDays); err != nil {
		return nil, err
	}

	if err := s.repo.CreateRecurringTemplate(ctx, template); err != nil {
		return nil, fmt.Errorf("failed to create template: %w", err)
	}

	return template, nil
}

// GetRecurringTemplate retrieves a recurring template by ID.
// Validates that the template belongs to the specified list.
func (s *Service) GetRecurringTemplate(ctx context.Context, listID, templateID string) (*domain.RecurringTemplate, error) {
	if templateID == "" {
		return nil, domain.ErrTemplateNotFound
	}

	template, err := s.repo.FindRecurringTemplate(ctx, templateID)
	if err != nil {
		return nil, err // Repository returns domain errors
	}

	// Verify ownership - return NotFound to avoid leaking template existence
	if template.ListID != listID {
		return nil, domain.ErrTemplateNotFound
	}

	return template, nil
}

// UpdateRecurringTemplate updates a recurring template using field mask.
// Only updates fields specified in UpdateMask.
// Validates that the template belongs to the specified list.
func (s *Service) UpdateRecurringTemplate(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error) {
	if params.TemplateID == "" {
		return nil, domain.ErrTemplateNotFound
	}

	// Verify ownership before update
	existing, err := s.repo.FindRecurringTemplate(ctx, params.TemplateID)
	if err != nil {
		return nil, err
	}
	if existing.ListID != params.ListID {
		return nil, domain.ErrTemplateNotFound
	}

	// Build mask set for validation
	maskSet := make(map[string]bool)
	for _, field := range params.UpdateMask {
		maskSet[field] = true
	}

	// Validate title - required field cannot be set to NULL
	// If title is in update_mask, a value must be provided
	if maskSet["title"] && params.Title == nil {
		return nil, domain.ErrTitleRequired
	}
	if params.Title != nil {
		title, err := domain.NewTitle(*params.Title)
		if err != nil {
			return nil, err
		}
		params.Title = ptr.To(title.String())
	}

	// Validate recurrence pattern - required field cannot be set to NULL
	// If recurrence_pattern is in update_mask, a value must be provided
	if maskSet["recurrence_pattern"] && params.RecurrencePattern == nil {
		return nil, domain.ErrRecurrencePatternRequired
	}
	if params.RecurrencePattern != nil {
		pattern, err := domain.NewRecurrencePattern(string(*params.RecurrencePattern))
		if err != nil {
			return nil, err
		}
		params.RecurrencePattern = ptr.To(pattern)
	}

	// Validate generation window if being updated using domain validation
	if params.GenerationWindowDays != nil {
		if err := domain.ValidateGenerationWindowDays(*params.GenerationWindowDays); err != nil {
			return nil, err
		}
	}

	return s.repo.UpdateRecurringTemplate(ctx, params)
}

// DeleteRecurringTemplate deletes a recurring template.
// Validates that the template belongs to the specified list.
func (s *Service) DeleteRecurringTemplate(ctx context.Context, listID, templateID string) error {
	if templateID == "" {
		return domain.ErrTemplateNotFound
	}

	// Verify ownership before delete
	existing, err := s.repo.FindRecurringTemplate(ctx, templateID)
	if err != nil {
		return err
	}
	if existing.ListID != listID {
		return domain.ErrTemplateNotFound
	}

	if err := s.repo.DeleteRecurringTemplate(ctx, templateID); err != nil {
		return err // Repository returns domain errors
	}

	return nil
}

// ListRecurringTemplates lists recurring templates for a list.
func (s *Service) ListRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*domain.RecurringTemplate, error) {
	if listID == "" {
		return nil, domain.ErrListNotFound
	}

	templates, err := s.repo.FindRecurringTemplates(ctx, listID, activeOnly)
	if err != nil {
		return nil, err // Repository returns domain errors
	}

	return templates, nil
}
