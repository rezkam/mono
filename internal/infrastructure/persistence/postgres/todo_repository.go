package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres/sqlcgen"
	"github.com/rezkam/mono/internal/ptr"
)

// checkRowsAffected validates that an UPDATE/DELETE operation affected exactly one row.
// Returns domain.ErrNotFound if rowsAffected == 0, indicating the record doesn't exist.
// This helper eliminates duplication of the :execrows existence check pattern.
func checkRowsAffected(rowsAffected int64, entityType, entityID string) error {
	if rowsAffected == 0 {
		return fmt.Errorf("%w: %s %s", domain.ErrNotFound, entityType, entityID)
	}
	return nil
}

// isForeignKeyViolation checks if an error is a PostgreSQL FK violation
func isForeignKeyViolation(err error, column string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == pgerrcode.ForeignKeyViolation {
			if column == "" {
				return true
			}
			// Check if the constraint name or message contains the column
			return strings.Contains(pgErr.ConstraintName, column) ||
				strings.Contains(pgErr.Message, column)
		}
	}
	return false
}

// === List Operations ===

// CreateList creates a new todo list.
// Returns the created list with version populated by persistence layer.
func (s *Store) CreateList(ctx context.Context, list *domain.TodoList) (*domain.TodoList, error) {
	id, title, createTime, err := domainTodoListToDB(list)
	if err != nil {
		return nil, fmt.Errorf("failed to convert list: %w", err)
	}

	params := sqlcgen.CreateTodoListParams{
		ID:        id,
		Title:     title,
		CreatedAt: timeToTimestamptz(createTime),
	}

	row, err := s.queries.CreateTodoList(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create list: %w", err)
	}

	return &domain.TodoList{
		ID:          row.ID,
		Title:       row.Title,
		CreatedAt:   timestamptzToTime(row.CreatedAt),
		TotalItems:  0, // New list has no items
		UndoneItems: 0,
		Version:     int(row.Version),
	}, nil
}

// FindListByID retrieves a todo list by its ID with metadata and counts.
func (s *Store) FindListByID(ctx context.Context, id string) (*domain.TodoList, error) {
	listUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	dbList, err := s.queries.GetTodoListWithCounts(ctx, sqlcgen.GetTodoListWithCountsParams{
		ID:             listUUID.String(),
		UndoneStatuses: taskStatusesToStrings(domain.UndoneStatuses()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: list %s", domain.ErrListNotFound, id)
		}
		return nil, fmt.Errorf("failed to get list: %w", err)
	}

	return &domain.TodoList{
		ID:          dbList.ID,
		Title:       dbList.Title,
		CreatedAt:   timestamptzToTime(dbList.CreatedAt),
		TotalItems:  int(dbList.TotalItems),
		UndoneItems: int(dbList.UndoneItems),
		Version:     int(dbList.Version),
	}, nil
}

// ListLists retrieves todo lists with filtering, sorting, and pagination.
func (s *Store) FindLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error) {
	// Build sqlc params from domain params
	sqlcParams := sqlcgen.FindTodoListsWithFiltersParams{
		UndoneStatuses: taskStatusesToStrings(domain.UndoneStatuses()),
		PageLimit:      int32(params.Limit),
		PageOffset:     int32(params.Offset),
	}

	// Apply optional filters
	if params.TitleContains != nil {
		sqlcParams.TitleContains = *params.TitleContains
	}
	if params.CreatedAtAfter != nil {
		sqlcParams.CreatedAtAfter = timePtrToQueryParam(params.CreatedAtAfter)
	}
	if params.CreatedAtBefore != nil {
		sqlcParams.CreatedAtBefore = timePtrToQueryParam(params.CreatedAtBefore)
	}

	// Apply validated sorting (defaults already applied by value object)
	sqlcParams.OrderBy = params.Sorting.OrderBy()
	sqlcParams.OrderDir = params.Sorting.OrderDir()

	// Execute the query
	rows, err := s.queries.FindTodoListsWithFilters(ctx, sqlcParams)
	if err != nil {
		return nil, fmt.Errorf("failed to find lists: %w", err)
	}

	// Convert to domain models
	lists := make([]*domain.TodoList, 0, len(rows))
	for _, row := range rows {
		list := &domain.TodoList{
			ID:          row.ID,
			Title:       row.Title,
			CreatedAt:   timestamptzToTime(row.CreatedAt),
			TotalItems:  int(row.TotalItems),
			UndoneItems: int(row.UndoneItems),
			Version:     int(row.Version),
		}
		lists = append(lists, list)
	}

	// Get total count for pagination
	countParams := sqlcgen.CountTodoListsWithFiltersParams{
		TitleContains:   sqlcParams.TitleContains,
		CreatedAtAfter:  sqlcParams.CreatedAtAfter,
		CreatedAtBefore: sqlcParams.CreatedAtBefore,
	}
	totalCount, err := s.queries.CountTodoListsWithFilters(ctx, countParams)
	if err != nil {
		return nil, fmt.Errorf("failed to count lists: %w", err)
	}

	// Calculate if there are more results
	hasMore := (params.Offset + params.Limit) < int(totalCount)

	return &domain.PagedListResult{
		Lists:      lists,
		TotalCount: int(totalCount),
		HasMore:    hasMore,
	}, nil
}

// UpdateList updates a list using field mask.
// Only updates fields specified in UpdateMask.
// Returns domain.ErrListNotFound if list doesn't exist.
// Returns domain.ErrVersionConflict if etag is provided and doesn't match current version.
func (s *Store) UpdateList(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error) {
	listUUID, err := uuid.Parse(params.ListID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	// Build update mask set for quick lookup
	maskSet := make(map[string]bool)
	for _, field := range params.UpdateMask {
		maskSet[field] = true
	}

	// Build sqlc params with field mask support
	// Uses CTE to atomically update and return counts in single statement
	sqlParams := sqlcgen.UpdateTodoListParams{
		ID:             listUUID.String(),
		UndoneStatuses: taskStatusesToStrings(domain.UndoneStatuses()),
	}

	// Map field mask to sqlc params
	if maskSet["title"] {
		sqlParams.SetTitle = true
		sqlParams.Title = *params.Title
	}

	// Handle optimistic locking with etag
	if params.Etag != nil {
		version, err := parseEtagToVersion(*params.Etag)
		if err != nil {
			return nil, fmt.Errorf("failed to parse etag: %w", err)
		}
		sqlParams.ExpectedVersion = int32PtrToInt4(&version)
	}

	// Execute atomic update query (returns updated list with counts)
	row, err := s.queries.UpdateTodoList(ctx, sqlParams)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Distinguish between not-found and version-conflict
			existingList, lookupErr := s.queries.GetTodoList(ctx, listUUID.String())
			if lookupErr != nil {
				if errors.Is(lookupErr, pgx.ErrNoRows) {
					return nil, fmt.Errorf("%w: list %s", domain.ErrListNotFound, params.ListID)
				}
				return nil, fmt.Errorf("failed to check list existence: %w", lookupErr)
			}

			// List exists, so update failed due to version mismatch
			if params.Etag != nil {
				return nil, fmt.Errorf("%w: expected version %s, current version %d",
					domain.ErrVersionConflict, *params.Etag, existingList.Version)
			}
			return nil, fmt.Errorf("%w: list %s", domain.ErrListNotFound, params.ListID)
		}
		return nil, fmt.Errorf("failed to update list: %w", err)
	}

	// Convert to domain model (all data returned atomically from single query)
	return &domain.TodoList{
		ID:          uuid.UUID(row.ID.Bytes).String(),
		Title:       row.Title,
		CreatedAt:   row.CreatedAt.Time.UTC(),
		TotalItems:  int(row.TotalItems),
		UndoneItems: int(row.UndoneItems),
		Version:     int(row.Version),
	}, nil
}

// === Item Operations ===

// CreateItem creates a new todo item in a list.
// Returns the created item with version populated by persistence layer.
func (s *Store) CreateItem(ctx context.Context, listID string, item *domain.TodoItem) (*domain.TodoItem, error) {
	params, err := domainTodoItemToDB(item, listID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert item: %w", err)
	}

	row, err := s.queries.CreateTodoItem(ctx, params)
	if err != nil {
		if isForeignKeyViolation(err, "list_id") {
			return nil, fmt.Errorf("%w: %w", domain.ErrListNotFound, err)
		}
		if isForeignKeyViolation(err, "recurring_template") {
			return nil, fmt.Errorf("%w: %w", domain.ErrTemplateNotFound, err)
		}
		return nil, fmt.Errorf("failed to create item: %w", err)
	}

	// Convert returned row to domain model (version comes from persistence layer)
	createdItem, err := dbTodoItemToDomain(row)
	if err != nil {
		return nil, fmt.Errorf("failed to convert created item: %w", err)
	}

	return &createdItem, nil
}

// FindItemByID retrieves a single todo item by its ID.
// O(1) lookup - more efficient than FindListByID when only the item is needed.
func (s *Store) FindItemByID(ctx context.Context, id string) (*domain.TodoItem, error) {
	itemUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	dbItem, err := s.queries.GetTodoItem(ctx, itemUUID.String())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: item %s", domain.ErrItemNotFound, id)
		}
		return nil, fmt.Errorf("failed to get item: %w", err)
	}

	item, err := dbTodoItemToDomain(dbItem)
	if err != nil {
		return nil, fmt.Errorf("failed to convert item: %w", err)
	}
	return &item, nil
}

// UpdateItem updates an item using field mask and optional etag.
// Only updates fields specified in UpdateMask without server-side read.
// If etag is provided and doesn't match, returns domain.ErrVersionConflict.
func (s *Store) UpdateItem(ctx context.Context, params domain.UpdateItemParams) (*domain.TodoItem, error) {
	itemUUID, err := uuid.Parse(params.ItemID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	listUUID, err := uuid.Parse(params.ListID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	// Build update mask set for quick lookup
	maskSet := make(map[string]bool)
	for _, field := range params.UpdateMask {
		maskSet[field] = true
	}

	// Build sqlc params - pass nil for fields not in mask to preserve existing values
	sqlcParams := sqlcgen.UpdateTodoItemParams{
		ID:     itemUUID.String(),
		ListID: listUUID.String(),
	}

	// Map field mask to sqlc params (nil = preserve existing value via COALESCE)
	if maskSet["title"] {
		sqlcParams.SetTitle = true
		sqlcParams.Title = *params.Title
	}
	if maskSet["status"] {
		sqlcParams.SetStatus = true
		sqlcParams.Status = sql.Null[string]{V: ptr.ToString(params.Status), Valid: true}
	}
	if maskSet["priority"] {
		sqlcParams.SetPriority = true
		if params.Priority != nil {
			sqlcParams.Priority = sql.Null[string]{V: ptr.ToString(params.Priority), Valid: true}
		} else {
			sqlcParams.Priority = sql.Null[string]{Valid: false}
		}
	}
	if maskSet["due_at"] {
		sqlcParams.SetDueAt = true
		sqlcParams.DueAt = timePtrToTimestamptz(params.DueAt)
	}
	if maskSet["tags"] {
		sqlcParams.SetTags = true
		if params.Tags != nil {
			sqlcParams.Tags = *params.Tags
		} else {
			sqlcParams.Tags = []string{}
		}
	}
	if maskSet["timezone"] {
		sqlcParams.SetTimezone = true
		if params.Timezone != nil {
			sqlcParams.Timezone = sql.Null[string]{V: *params.Timezone, Valid: true}
		} else {
			sqlcParams.Timezone = sql.Null[string]{Valid: false}
		}
	}
	if maskSet["estimated_duration"] {
		sqlcParams.SetEstimatedDuration = true
		sqlcParams.EstimatedDuration = durationPtrToPgtypeInterval(params.EstimatedDuration)
	}
	if maskSet["actual_duration"] {
		sqlcParams.SetActualDuration = true
		sqlcParams.ActualDuration = durationPtrToPgtypeInterval(params.ActualDuration)
	}
	if maskSet["starts_at"] {
		sqlcParams.SetStartsAt = true
		sqlcParams.StartsAt = timePtrToDate(params.StartsAt)
	}
	if maskSet["due_offset"] {
		sqlcParams.SetDueOffset = true
		sqlcParams.DueOffset = durationPtrToPgtypeInterval(params.DueOffset)
	}

	// Handle detachment from recurring template
	// Set by service layer when content/schedule fields are modified on recurring items
	sqlcParams.DetachFromTemplate = params.DetachFromTemplate

	// Handle optimistic locking with etag
	if params.Etag != nil {
		version, err := parseEtagToVersion(*params.Etag)
		if err != nil {
			return nil, fmt.Errorf("failed to parse etag: %w", err)
		}
		sqlcParams.ExpectedVersion = int32PtrToInt4(&version)
	}

	// Execute type-safe update query
	item, err := s.queries.UpdateTodoItem(ctx, sqlcParams)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Distinguish between not-found and version-conflict
			existingItem, lookupErr := s.queries.GetTodoItem(ctx, itemUUID.String())
			if lookupErr != nil {
				if errors.Is(lookupErr, pgx.ErrNoRows) {
					return nil, fmt.Errorf("%w: item %s", domain.ErrItemNotFound, params.ItemID)
				}
				return nil, fmt.Errorf("failed to check item existence: %w", lookupErr)
			}

			// Item exists - check if list ID matches
			if existingItem.ListID != params.ListID {
				return nil, fmt.Errorf("%w: item %s", domain.ErrItemNotFound, params.ItemID)
			}

			// Item exists and belongs to correct list, so update failed due to version mismatch
			if params.Etag != nil {
				return nil, fmt.Errorf("%w: expected version %s, current version %d",
					domain.ErrVersionConflict, *params.Etag, existingItem.Version)
			}
			return nil, fmt.Errorf("%w: item %s", domain.ErrItemNotFound, params.ItemID)
		}
		return nil, fmt.Errorf("failed to update item: %w", err)
	}

	// Convert to domain
	domainItem, err := dbTodoItemToDomain(item)
	if err != nil {
		return nil, fmt.Errorf("failed to convert item: %w", err)
	}

	return &domainItem, nil
}

// DeleteItem deletes a todo item by ID.
// Returns domain.ErrItemNotFound if item doesn't exist.
func (s *Store) DeleteItem(ctx context.Context, id string) error {
	rowsAffected, err := s.queries.DeleteTodoItem(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete item: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("%w: item %s", domain.ErrItemNotFound, id)
	}

	return nil
}

// parseEtagToVersion extracts the version number from an etag string.
// Assumes etag is already validated by the service layer.
// Etag format: numeric string like "1", "2", "42"
func parseEtagToVersion(etag string) (int32, error) {
	var version int32
	_, err := fmt.Sscanf(etag, "%d", &version)
	return version, err
}

// FindItems searches for items with filtering, sorting, and pagination.
// excludedStatuses is provided by service layer based on business rules.
func (s *Store) FindItems(ctx context.Context, params domain.ListTasksParams, excludedStatuses []domain.TaskStatus) (*domain.PagedResult, error) {
	// Build the query parameters for sqlc - uses empty arrays to skip filters
	var zeroUUID uuid.UUID

	// Column1: list_id (zero UUID to search all lists)
	listUUID := zeroUUID
	if params.ListID != nil {
		parsed, err := uuid.Parse(*params.ListID)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
		}
		listUUID = parsed
	}

	// Column2: statuses array (empty array to skip filter)
	statuses := taskStatusesToStrings(params.Filter.Statuses())

	// Column3: priorities array (empty array to skip filter)
	priorities := taskPrioritiesToStrings(params.Filter.Priorities())

	// Column4: tags array (empty array to skip filter)
	tags := params.Filter.Tags()
	if tags == nil {
		tags = []string{}
	}

	// Column5: due_before (zero time to skip filter)
	dueBefore := timePtrToQueryParam(params.DueBefore)

	// Column6: due_after (zero time to skip filter)
	dueAfter := timePtrToQueryParam(params.DueAfter)

	// Column7, Column8: updated_at and created_at filters (zero time to skip filter)
	updatedAt := timePtrToQueryParam(nil)
	createdAt := timePtrToQueryParam(nil)

	// Column9: order_by combined with direction (e.g., "created_at_desc", "due_time_asc")
	orderBy := params.Filter.OrderBy()
	if params.Filter.OrderDir() != "" {
		orderBy = params.Filter.OrderBy() + "_" + params.Filter.OrderDir()
	}

	// Column12: excluded_statuses (provided by service layer)
	excludedStatusStrings := taskStatusesToStrings(excludedStatuses)

	sqlcParams := sqlcgen.ListTasksWithFiltersParams{
		Column1:  uuidToQueryParam(listUUID),
		Column2:  statuses,
		Column3:  priorities,
		Column4:  tags,
		Column5:  dueBefore,
		Column6:  dueAfter,
		Column7:  updatedAt,
		Column8:  createdAt,
		Column9:  orderBy,
		Limit:    int32(params.Limit),
		Offset:   int32(params.Offset),
		Column12: excludedStatusStrings,
	}

	// Execute query - includes COUNT(*) OVER() as total_count in each row
	dbItems, err := s.queries.ListTasksWithFilters(ctx, sqlcParams)
	if err != nil {
		return nil, fmt.Errorf("failed to list items: %w", err)
	}

	// Get total count from first row (all rows have same total_count from window function)
	// If no rows returned, run separate count query to get actual total
	var totalCount int
	if len(dbItems) > 0 {
		totalCount = int(dbItems[0].TotalCount)
	} else {
		// Empty page - need separate count query to know actual total
		// This handles the case where offset >= total items
		countParams := sqlcgen.CountTasksWithFiltersParams{
			Column1: uuidToQueryParam(listUUID),
			Column2: statuses,
			Column3: priorities,
			Column4: tags,
			Column5: dueBefore,
			Column6: dueAfter,
			Column7: updatedAt,
			Column8: createdAt,
			Column9: excludedStatusStrings,
		}
		count, err := s.queries.CountTasksWithFilters(ctx, countParams)
		if err != nil {
			return nil, fmt.Errorf("failed to count items: %w", err)
		}
		totalCount = int(count)
	}

	// Convert to domain items
	items := make([]domain.TodoItem, len(dbItems))
	for i, dbItem := range dbItems {
		item, err := dbListTasksRowToDomain(dbItem)
		if err != nil {
			return nil, fmt.Errorf("failed to convert item: %w", err)
		}
		items[i] = item
	}

	// HasMore is true if there are more items beyond what we've returned
	hasMore := params.Offset+len(items) < totalCount

	return &domain.PagedResult{
		Items:      items,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}, nil
}

// === Recurring Template Operations ===

// CreateRecurringTemplate creates a new recurring task template.
// Returns the created template with version populated by persistence layer.
func (s *Store) CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) (*domain.RecurringTemplate, error) {
	params, err := domainRecurringTemplateToDB(template)
	if err != nil {
		return nil, fmt.Errorf("failed to convert template: %w", err)
	}

	row, err := s.queries.CreateRecurringTemplate(ctx, params)
	if err != nil {
		if isForeignKeyViolation(err, "list_id") {
			return nil, fmt.Errorf("%w: %w", domain.ErrListNotFound, err)
		}
		return nil, fmt.Errorf("failed to create template: %w", err)
	}

	// Convert returned row to domain model (version comes from persistence layer)
	return dbRecurringTemplateToDomain(row)
}

// FindRecurringTemplate retrieves a template by ID.
func (s *Store) FindRecurringTemplateByID(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	templateUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	dbTemplate, err := s.queries.FindRecurringTemplateByID(ctx, templateUUID.String())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: template %s", domain.ErrTemplateNotFound, id)
		}
		return nil, fmt.Errorf("failed to get template: %w", err)
	}

	template, err := dbRecurringTemplateToDomain(dbTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to convert template: %w", err)
	}

	return template, nil
}

// UpdateRecurringTemplate updates a template using field mask.
// Only updates fields specified in UpdateMask.
// Returns domain.ErrTemplateNotFound if template doesn't exist.
// Returns domain.ErrVersionConflict if etag is provided and doesn't match current version.
func (s *Store) UpdateRecurringTemplate(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error) {
	templateUUID, err := uuid.Parse(params.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	// Build update mask set for quick lookup
	maskSet := make(map[string]bool)
	for _, field := range params.UpdateMask {
		maskSet[field] = true
	}

	// Build sqlc params with field mask support
	sqlcParams := sqlcgen.UpdateRecurringTemplateParams{
		ID: templateUUID.String(),
	}

	// Map field mask to sqlc params (Set* = true means update this field)
	if maskSet["title"] {
		sqlcParams.SetTitle = true
		sqlcParams.Title = *params.Title
	}
	if maskSet["tags"] {
		sqlcParams.SetTags = true
		if params.Tags != nil {
			sqlcParams.Tags = *params.Tags
		} else {
			sqlcParams.Tags = []string{}
		}
	}
	if maskSet["priority"] {
		sqlcParams.SetPriority = true
		if params.Priority != nil {
			priorityStr := string(*params.Priority)
			sqlcParams.Priority = sql.Null[string]{V: priorityStr, Valid: true}
		} else {
			sqlcParams.Priority = sql.Null[string]{Valid: false}
		}
	}
	if maskSet["estimated_duration"] {
		sqlcParams.SetEstimatedDuration = true
		sqlcParams.EstimatedDuration = durationPtrToPgtypeInterval(params.EstimatedDuration)
	}
	if maskSet["recurrence_pattern"] {
		sqlcParams.SetRecurrencePattern = true
		sqlcParams.RecurrencePattern = sql.Null[string]{V: ptr.ToString(params.RecurrencePattern), Valid: true}
	}
	if maskSet["recurrence_config"] {
		sqlcParams.SetRecurrenceConfig = true
		if params.RecurrenceConfig != nil {
			configJSON, err := json.Marshal(params.RecurrenceConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal recurrence_config: %w", err)
			}
			sqlcParams.RecurrenceConfig = configJSON
		}
	}
	if maskSet["due_offset"] {
		sqlcParams.SetDueOffset = true
		sqlcParams.DueOffset = durationPtrToPgtypeInterval(params.DueOffset)
	}
	if maskSet["is_active"] {
		sqlcParams.SetIsActive = true
		sqlcParams.IsActive = boolPtrToBool(params.IsActive)
	}
	if maskSet["sync_horizon_days"] {
		sqlcParams.SetSyncHorizonDays = true
		if params.SyncHorizonDays != nil {
			days := int32(*params.SyncHorizonDays)
			sqlcParams.SyncHorizonDays = int32PtrToInt4(&days)
		}
	}
	if maskSet["generation_horizon_days"] {
		sqlcParams.SetGenerationHorizonDays = true
		if params.GenerationHorizonDays != nil {
			days := int32(*params.GenerationHorizonDays)
			sqlcParams.GenerationHorizonDays = int32PtrToInt4(&days)
		}
	}

	// Handle optimistic locking with etag
	if params.Etag != nil {
		version, err := parseEtagToVersion(*params.Etag)
		if err != nil {
			return nil, fmt.Errorf("failed to parse etag: %w", err)
		}
		sqlcParams.ExpectedVersion = int32PtrToInt4(&version)
	}

	// Execute type-safe update query
	tmpl, err := s.queries.UpdateRecurringTemplate(ctx, sqlcParams)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Distinguish between not-found and version-conflict
			existingTemplate, lookupErr := s.queries.FindRecurringTemplateByID(ctx, templateUUID.String())
			if lookupErr != nil {
				if errors.Is(lookupErr, pgx.ErrNoRows) {
					return nil, fmt.Errorf("%w: template %s", domain.ErrTemplateNotFound, params.TemplateID)
				}
				return nil, fmt.Errorf("failed to check template existence: %w", lookupErr)
			}

			// Template exists, so update failed due to version mismatch
			if params.Etag != nil {
				return nil, fmt.Errorf("%w: expected version %s, current version %d",
					domain.ErrVersionConflict, *params.Etag, existingTemplate.Version)
			}
			return nil, fmt.Errorf("%w: template %s", domain.ErrTemplateNotFound, params.TemplateID)
		}
		return nil, fmt.Errorf("failed to update template: %w", err)
	}

	return dbRecurringTemplateToDomain(tmpl)
}

// DeleteRecurringTemplate deletes a template.
func (s *Store) DeleteRecurringTemplate(ctx context.Context, id string) error {
	templateUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	// Single-query pattern: check rowsAffected to detect non-existent record
	rowsAffected, err := s.queries.DeleteRecurringTemplate(ctx, templateUUID.String())
	if err != nil {
		return fmt.Errorf("failed to delete template: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("%w: template %s", domain.ErrTemplateNotFound, id)
	}

	return nil
}

// FindRecurringTemplates lists all templates for a list, optionally filtered by active status.
func (s *Store) FindRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*domain.RecurringTemplate, error) {
	listUUID, err := uuid.Parse(listID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	var dbTemplates []sqlcgen.RecurringTaskTemplate
	if activeOnly {
		// Get active templates for this specific list (WHERE list_id = $1 AND is_active = true)
		dbTemplates, err = s.queries.ListRecurringTemplates(ctx, listUUID.String())
		if err != nil {
			return nil, fmt.Errorf("failed to list active templates: %w", err)
		}
	} else {
		// Get all templates (active and inactive) for this list (WHERE list_id = $1)
		dbTemplates, err = s.queries.ListAllRecurringTemplatesByList(ctx, listUUID.String())
		if err != nil {
			return nil, fmt.Errorf("failed to list templates: %w", err)
		}
	}

	templates := make([]*domain.RecurringTemplate, 0, len(dbTemplates))
	for _, dbTemplate := range dbTemplates {
		template, err := dbRecurringTemplateToDomain(dbTemplate)
		if err != nil {
			return nil, fmt.Errorf("failed to convert template: %w", err)
		}
		templates = append(templates, template)
	}

	return templates, nil
}
