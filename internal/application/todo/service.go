package todo

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/ptr"
)

// Field groups for update mask validation.
// Defined at package level for maintainability - adding new fields only requires updating these slices.
var (
	// patternChangeFields are fields that affect scheduling behavior and require regeneration.
	patternChangeFields = []string{
		domain.FieldRecurrencePattern,
		domain.FieldRecurrenceConfig,
		domain.FieldSyncHorizonDays,
		domain.FieldGenerationHorizonDays,
	}

	// exceptionFields are fields that require creating an exception for recurring items.
	// Exceptions prevent the template from regenerating this occurrence.
	exceptionFields = []string{
		domain.FieldItemTitle,
		domain.FieldItemTags,
		domain.FieldItemPriority,
		domain.FieldItemEstimatedDuration,
		domain.FieldDueAt,
		domain.FieldStartsAt,
		domain.FieldOccursAt,
	}
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

// TaskGenerator generates recurring task instances from templates.
type TaskGenerator interface {
	GenerateTasksForTemplateWithExceptions(ctx context.Context, template *domain.RecurringTemplate, start, end time.Time, exceptions []*domain.RecurringTemplateException) ([]*domain.TodoItem, error)
}

// Service provides business logic for todo management.
// It orchestrates operations using the Repository interface.
type Service struct {
	repo      Repository
	generator TaskGenerator
	config    Config
}

// NewService creates a new todo service.
// Applies application defaults for zero or invalid config values.
// Both DefaultPageSize and MaxPageSize must be > 0.
func NewService(repo Repository, generator TaskGenerator, config Config) *Service {
	// Apply defaults for zero or invalid values (must be > 0)
	if config.DefaultPageSize <= 0 {
		config.DefaultPageSize = DefaultPageSize
	}
	if config.MaxPageSize <= 0 {
		config.MaxPageSize = MaxPageSize
	}

	return &Service{
		repo:      repo,
		generator: generator,
		config:    config,
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
		ID:          idObj.String(),
		Title:       title.String(),
		CreatedAt:   time.Now().UTC(),
		TotalItems:  0,
		UndoneItems: 0,
	}

	// Return the persisted entity from repository (includes version from persistence layer)
	createdList, err := s.repo.CreateList(ctx, list)
	if err != nil {
		return nil, fmt.Errorf("failed to create list: %w", err)
	}

	return createdList, nil
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

	// Validate update mask and required fields
	if err := params.Validate(); err != nil {
		return nil, err
	}

	// Validate title value if being updated
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
	item.CreatedAt = now
	item.UpdatedAt = now

	// Ensure DueAt is UTC if provided
	if item.DueAt != nil {
		utc := item.DueAt.UTC()
		item.DueAt = &utc
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

	// Return the persisted entity from repository (includes version from persistence layer)
	createdItem, err := s.repo.CreateItem(ctx, listID, item)
	if err != nil {
		return nil, fmt.Errorf("failed to create item: %w", err)
	}

	return createdItem, nil
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

	// Validate update mask and required fields
	if err := params.Validate(); err != nil {
		return nil, err
	}

	// Validate title value if being updated
	if params.Title != nil {
		title, err := domain.NewTitle(*params.Title)
		if err != nil {
			return nil, err
		}
		params.Title = ptr.To(title.String())
	}

	// Validate status value if being updated
	if params.Status != nil {
		if _, err := domain.NewTaskStatus(string(*params.Status)); err != nil {
			return nil, err
		}
	}

	// Validate priority value if being updated
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

	// Fetch existing item to check if it's a recurring item
	existingItem, err := s.repo.FindItemByID(ctx, params.ItemID)
	if err != nil {
		return nil, err
	}

	// Verify ownership
	if existingItem.ListID != params.ListID {
		return nil, domain.ErrItemNotFound
	}

	// Check if this is a recurring item that needs exception handling
	if existingItem.RecurringTemplateID != nil && existingItem.OccursAt != nil {
		if shouldCreateException(params.UpdateMask) {
			// Create exception to prevent template regeneration
			// Keep template link for reference - exception prevents regeneration
			excID, err := uuid.NewV7()
			if err != nil {
				return nil, fmt.Errorf("failed to generate exception id: %w", err)
			}

			exception := &domain.RecurringTemplateException{
				ID:            excID.String(),
				TemplateID:    *existingItem.RecurringTemplateID,
				OccursAt:      *existingItem.OccursAt,
				ExceptionType: domain.ExceptionTypeEdited,
				ItemID:        &existingItem.ID,
				CreatedAt:     time.Now().UTC(),
			}

			// Use atomic operation to update item and create exception together
			var updatedItem *domain.TodoItem
			err = s.repo.Atomic(ctx, func(repo Repository) error {
				item, err := repo.UpdateItem(ctx, params)
				if err != nil {
					return err
				}
				updatedItem = item

				_, err = repo.CreateException(ctx, exception)
				return err
			})
			if err != nil {
				return nil, err
			}
			return updatedItem, nil
		}
	}

	// No exception needed - standard update
	return s.repo.UpdateItem(ctx, params)
}

// DeleteItem deletes a todo item.
// For recurring items: creates exception and archives item (soft delete).
// For non-recurring items: hard delete (not implemented yet).
func (s *Service) DeleteItem(ctx context.Context, listID, itemID string) error {
	// Find item
	item, err := s.repo.FindItemByID(ctx, itemID)
	if err != nil {
		return err
	}

	// Verify ownership
	if item.ListID != listID {
		return domain.ErrItemNotFound
	}

	// Check if recurring item
	if item.RecurringTemplateID != nil && item.OccursAt != nil {
		// Recurring item - soft delete with exception
		// Generate exception ID
		excID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("failed to generate exception id: %w", err)
		}

		// Create exception
		exception := &domain.RecurringTemplateException{
			ID:            excID.String(),
			TemplateID:    *item.RecurringTemplateID,
			OccursAt:      *item.OccursAt,
			ExceptionType: domain.ExceptionTypeDeleted,
			ItemID:        &item.ID,
			CreatedAt:     time.Now().UTC(),
		}

		// Use atomic operation to create exception and archive item together
		return s.repo.Atomic(ctx, func(repo Repository) error {
			// Create exception first
			if _, err := repo.CreateException(ctx, exception); err != nil {
				return err
			}

			// Archive item (soft delete)
			archived := domain.TaskStatusArchived
			updateParams := domain.UpdateItemParams{
				ItemID:     itemID,
				ListID:     listID,
				UpdateMask: []string{"status"},
				Status:     &archived,
			}
			_, err := repo.UpdateItem(ctx, updateParams)
			return err
		})
	}

	// Non-recurring item - TODO: implement hard delete
	return domain.ErrHardDeleteNotImplemented
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
	template.GeneratedThrough = now
	template.IsActive = true

	// Apply default horizons if not set
	if template.SyncHorizonDays == 0 {
		template.SyncHorizonDays = 14 // Default: 2 weeks immediate
	}
	if template.GenerationHorizonDays == 0 {
		template.GenerationHorizonDays = 365 // Default: 1 year total
	}

	// Validate generation window
	if err := domain.ValidateGenerationWindowDays(template.GenerationHorizonDays); err != nil {
		return nil, err
	}

	// Prepare SYNC items: generate next N days immediately
	syncEnd := now.AddDate(0, 0, template.SyncHorizonDays)
	// No exceptions for newly created template
	syncItems, err := s.generator.GenerateTasksForTemplateWithExceptions(ctx, template, now, syncEnd, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate sync items: %w", err)
	}

	// Prepare ASYNC job for remaining days (if needed)
	var asyncJob *domain.GenerationJob
	asyncEnd := now.AddDate(0, 0, template.GenerationHorizonDays)
	if syncEnd.Before(asyncEnd) {
		// Generate job ID
		jobIDObj, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("failed to generate job ID: %w", err)
		}

		asyncJob = &domain.GenerationJob{
			ID:            jobIDObj.String(),
			TemplateID:    template.ID,
			GenerateFrom:  syncEnd,
			GenerateUntil: asyncEnd,
			ScheduledFor:  now,
			CreatedAt:     now,
			RetryCount:    0,
		}
	}

	// Use atomic operation: template + items + generation marker + job all succeed/fail together
	err = s.repo.Atomic(ctx, func(repo Repository) error {
		// 1. Create template
		if _, err := repo.CreateRecurringTemplate(ctx, template); err != nil {
			return fmt.Errorf("failed to create template: %w", err)
		}

		// 2. Insert sync horizon items
		if len(syncItems) > 0 {
			if _, err = repo.BatchInsertItemsIgnoreConflict(ctx, syncItems); err != nil {
				return fmt.Errorf("failed to insert sync items: %w", err)
			}
		}

		// 3. Update generation marker
		if err := repo.SetGeneratedThrough(ctx, template.ID, syncEnd); err != nil {
			return fmt.Errorf("failed to update generation marker: %w", err)
		}

		// 4. Create async generation job (if needed)
		if asyncJob != nil {
			if err := repo.CreateGenerationJob(ctx, asyncJob); err != nil {
				return fmt.Errorf("failed to create generation job: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Refetch template to get updated generated_through
	return s.repo.FindRecurringTemplate(ctx, template.ID)
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
// Pattern changes (recurrence_pattern, recurrence_config, horizons) trigger regeneration of future items.
// Content changes (title, tags, priority) only update the template.
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

	// Validate update mask and required fields
	if err := params.Validate(); err != nil {
		return nil, err
	}

	// Validate title value if being updated
	if params.Title != nil {
		title, err := domain.NewTitle(*params.Title)
		if err != nil {
			return nil, err
		}
		params.Title = ptr.To(title.String())
	}

	// Validate recurrence pattern value if being updated
	if params.RecurrencePattern != nil {
		pattern, err := domain.NewRecurrencePattern(string(*params.RecurrencePattern))
		if err != nil {
			return nil, err
		}
		params.RecurrencePattern = ptr.To(pattern)
	}

	// Validate generation horizons if being updated
	if params.SyncHorizonDays != nil && *params.SyncHorizonDays <= 0 {
		return nil, domain.ErrSyncHorizonMustBePositive
	}
	if params.GenerationHorizonDays != nil {
		if err := domain.ValidateGenerationWindowDays(*params.GenerationHorizonDays); err != nil {
			return nil, err
		}
	}

	// Check if this is a pattern change (requires regeneration)
	isPatternChange := s.isPatternChange(params)

	if isPatternChange {
		// Pattern change: delete future items and regenerate
		return s.updateTemplateWithRegeneration(ctx, existing, params)
	}

	// Content-only change: just update the template
	return s.repo.UpdateRecurringTemplate(ctx, params)
}

// isPatternChange detects if the update changes scheduling behavior.
func (s *Service) isPatternChange(params domain.UpdateRecurringTemplateParams) bool {
	for _, field := range params.UpdateMask {
		if slices.Contains(patternChangeFields, field) {
			return true
		}
	}
	return false
}

// containsAnyField checks if updateMask contains any field from the given set.
func containsAnyField(updateMask []string, fields []string) bool {
	for _, field := range updateMask {
		if slices.Contains(fields, field) {
			return true
		}
	}
	return false
}

// shouldCreateException checks if the update mask contains content fields that require
// creating an exception for recurring items (but keeping the template link).
func shouldCreateException(updateMask []string) bool {
	return containsAnyField(updateMask, exceptionFields)
}

// updateTemplateWithRegeneration handles pattern changes by deleting future items and regenerating.
func (s *Service) updateTemplateWithRegeneration(ctx context.Context, existing *domain.RecurringTemplate, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error) {
	now := time.Now().UTC()

	syncHorizon := existing.SyncHorizonDays
	if syncHorizon == 0 {
		syncHorizon = 14
	}
	syncEnd := now.AddDate(0, 0, syncHorizon)

	syncItems, err := s.generator.GenerateTasksForTemplateWithExceptions(ctx, existing, now, syncEnd, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate sync items: %w", err)
	}

	// Prepare async job for remaining generation horizon (if needed)
	var asyncJob *domain.GenerationJob
	generationHorizon := existing.GenerationHorizonDays
	if generationHorizon == 0 {
		generationHorizon = 365
	}
	asyncEnd := now.AddDate(0, 0, generationHorizon)

	if syncEnd.Before(asyncEnd) {
		jobIDObj, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("failed to generate job ID: %w", err)
		}

		asyncJob = &domain.GenerationJob{
			ID:            jobIDObj.String(),
			TemplateID:    params.TemplateID,
			GenerateFrom:  syncEnd,
			GenerateUntil: asyncEnd,
			ScheduledFor:  now,
			CreatedAt:     now,
			RetryCount:    0,
		}
	}

	// Use atomic operation: update template + regenerate items + generation marker + job all succeed/fail together
	err = s.repo.Atomic(ctx, func(repo Repository) error {
		// 1. Update template
		if _, err := repo.UpdateRecurringTemplate(ctx, params); err != nil {
			return fmt.Errorf("failed to update template: %w", err)
		}

		// 2. Delete future pending items
		if _, err := repo.DeleteFuturePendingItems(ctx, params.TemplateID, now); err != nil {
			return fmt.Errorf("failed to delete future items: %w", err)
		}

		// 3. Insert regenerated items
		if len(syncItems) > 0 {
			if _, err := repo.BatchInsertItemsIgnoreConflict(ctx, syncItems); err != nil {
				return fmt.Errorf("failed to insert regenerated items: %w", err)
			}
		}

		// 4. Update generation marker
		if err := repo.SetGeneratedThrough(ctx, params.TemplateID, syncEnd); err != nil {
			return fmt.Errorf("failed to update generation marker: %w", err)
		}

		// 5. Create async generation job (if needed)
		if asyncJob != nil {
			if err := repo.CreateGenerationJob(ctx, asyncJob); err != nil {
				return fmt.Errorf("failed to create generation job: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Refetch template to get updated generated_through
	return s.repo.FindRecurringTemplate(ctx, params.TemplateID)
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

// ListExceptions retrieves exceptions for a recurring template.
// Validates that the template belongs to the specified list.
func (s *Service) ListExceptions(ctx context.Context, listID, templateID string, from, until time.Time) ([]*domain.RecurringTemplateException, error) {
	if listID == "" {
		return nil, domain.ErrListNotFound
	}
	if templateID == "" {
		return nil, domain.ErrTemplateNotFound
	}

	// Verify template belongs to list
	template, err := s.repo.FindRecurringTemplate(ctx, templateID)
	if err != nil {
		return nil, err
	}
	if template.ListID != listID {
		return nil, domain.ErrTemplateNotFound
	}

	return s.repo.ListExceptions(ctx, templateID, from, until)
}
