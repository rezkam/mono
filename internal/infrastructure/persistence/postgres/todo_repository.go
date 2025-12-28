package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
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
		// 23503 is foreign_key_violation
		if pgErr.Code == "23503" {
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
func (s *Store) CreateList(ctx context.Context, list *domain.TodoList) error {
	id, title, createTime, err := domainTodoListToDB(list)
	if err != nil {
		return fmt.Errorf("failed to convert list: %w", err)
	}

	params := sqlcgen.CreateTodoListParams{
		ID:         id,
		Title:      title,
		CreateTime: createTime,
	}

	if err := s.queries.CreateTodoList(ctx, params); err != nil {
		return fmt.Errorf("failed to create list: %w", err)
	}

	return nil
}

// FindListByID retrieves a todo list by its ID with metadata and counts.
// Uses REPEATABLE READ isolation to ensure consistent snapshot.
func (s *Store) FindListByID(ctx context.Context, id string) (*domain.TodoList, error) {
	listUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	// Use REPEATABLE READ to ensure consistent snapshot for counts.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Bind queries to transaction
	qtx := sqlcgen.New(tx)

	// Get the list with counts (domain defines which statuses are "undone")
	dbList, err := qtx.GetTodoListWithCounts(ctx, sqlcgen.GetTodoListWithCountsParams{
		ID:             listUUID.String(),
		UndoneStatuses: taskStatusesToStrings(domain.UndoneStatuses()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: list %s", domain.ErrListNotFound, id)
		}
		return nil, fmt.Errorf("failed to get list: %w", err)
	}

	// Commit read-only transaction (releases snapshot)
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Convert to domain model (items fetched separately via FindItems)
	list := &domain.TodoList{
		ID:          dbList.ID,
		Title:       dbList.Title,
		CreateTime:  dbList.CreateTime.UTC(),
		TotalItems:  int(dbList.TotalItems),
		UndoneItems: int(dbList.UndoneItems),
	}

	return list, nil
}

// ListLists retrieves todo lists with filtering, sorting, and pagination.
func (s *Store) ListLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error) {
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
	if params.CreateTimeAfter != nil {
		sqlcParams.CreateTimeAfter = timePtrToQueryParam(params.CreateTimeAfter)
	}
	if params.CreateTimeBefore != nil {
		sqlcParams.CreateTimeBefore = timePtrToQueryParam(params.CreateTimeBefore)
	}

	// Apply sorting (defaults to create_time desc)
	orderBy := params.OrderBy
	if orderBy == "" {
		orderBy = "create_time"
	}
	sqlcParams.OrderBy = orderBy

	orderDir := params.OrderDir
	if orderDir == "" {
		orderDir = "desc"
	}
	sqlcParams.OrderDir = orderDir

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
			CreateTime:  row.CreateTime.UTC(),
			TotalItems:  int(row.TotalItems),
			UndoneItems: int(row.UndoneItems),
		}
		lists = append(lists, list)
	}

	// Get total count for pagination
	countParams := sqlcgen.CountTodoListsWithFiltersParams{
		TitleContains:    sqlcParams.TitleContains,
		CreateTimeAfter:  sqlcParams.CreateTimeAfter,
		CreateTimeBefore: sqlcParams.CreateTimeBefore,
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
// Returns the updated list.
func (s *Store) UpdateList(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error) {
	id, err := uuid.Parse(params.ListID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	// Check if title is in update mask
	updateTitle := false
	for _, field := range params.UpdateMask {
		if field == "title" {
			updateTitle = true
			break
		}
	}

	// Only perform update if there are fields to update
	if updateTitle && params.Title != nil {
		sqlParams := sqlcgen.UpdateTodoListParams{
			ID:    id.String(),
			Title: *params.Title,
		}

		rowsAffected, err := s.queries.UpdateTodoList(ctx, sqlParams)
		if err != nil {
			return nil, fmt.Errorf("failed to update list: %w", err)
		}

		if err := checkRowsAffected(rowsAffected, "list", params.ListID); err != nil {
			return nil, err
		}
	}

	// Fetch and return the updated list
	return s.FindListByID(ctx, params.ListID)
}

// === Item Operations ===

// CreateItem creates a new todo item in a list.
func (s *Store) CreateItem(ctx context.Context, listID string, item *domain.TodoItem) error {
	params, err := domainTodoItemToDB(item, listID)
	if err != nil {
		return fmt.Errorf("failed to convert item: %w", err)
	}

	if err := s.queries.CreateTodoItem(ctx, params); err != nil {
		if isForeignKeyViolation(err, "list_id") {
			return fmt.Errorf("%w: %w", domain.ErrListNotFound, err)
		}
		if isForeignKeyViolation(err, "recurring_template_id") {
			return fmt.Errorf("%w: %w", domain.ErrTemplateNotFound, err)
		}
		return fmt.Errorf("failed to create item: %w", err)
	}

	// Set version after successful insert (DB defaults to 1)
	item.Version = 1

	return nil
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
		sqlcParams.Status = ptr.ToString(params.Status)
	}
	if maskSet["priority"] {
		sqlcParams.SetPriority = true
		if params.Priority != nil {
			sqlcParams.Priority = sql.Null[string]{V: ptr.ToString(params.Priority), Valid: true}
		} else {
			sqlcParams.Priority = sql.Null[string]{Valid: false}
		}
	}
	if maskSet["due_time"] {
		sqlcParams.SetDueTime = true
		if params.DueTime != nil {
			sqlcParams.DueTime = sql.Null[time.Time]{V: *params.DueTime, Valid: true}
		} else {
			sqlcParams.DueTime = sql.Null[time.Time]{Valid: false}
		}
	}
	if maskSet["tags"] {
		sqlcParams.SetTags = true
		if params.Tags != nil {
			sqlcParams.Tags, _ = json.Marshal(*params.Tags)
		} else {
			sqlcParams.Tags = []byte("[]")
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
			return nil, fmt.Errorf("%w: item %s", domain.ErrNotFound, params.ItemID)
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
func (s *Store) CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) error {
	params, err := domainRecurringTemplateToDB(template)
	if err != nil {
		return fmt.Errorf("failed to convert template: %w", err)
	}

	if err := s.queries.CreateRecurringTemplate(ctx, params); err != nil {
		if isForeignKeyViolation(err, "list_id") {
			return fmt.Errorf("%w: %w", domain.ErrListNotFound, err)
		}
		return fmt.Errorf("failed to create template: %w", err)
	}

	return nil
}

// FindRecurringTemplate retrieves a template by ID.
func (s *Store) FindRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	templateUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	dbTemplate, err := s.queries.GetRecurringTemplate(ctx, templateUUID.String())
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
// Returns the updated template.
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

	// Build sqlc params - pass nil for fields not in mask to preserve existing values
	sqlcParams := sqlcgen.UpdateRecurringTemplateParams{
		ID: templateUUID.String(),
	}

	// Map field mask to sqlc params (nil = preserve existing value via COALESCE)
	if maskSet["title"] {
		sqlcParams.Title = *params.Title
	}
	if maskSet["tags"] {
		if params.Tags != nil {
			sqlcParams.Tags, _ = json.Marshal(*params.Tags)
		} else {
			sqlcParams.Tags = []byte("[]")
		}
	}
	if maskSet["priority"] {
		if params.Priority != nil {
			priorityStr := string(*params.Priority)
			sqlcParams.Priority = sql.Null[string]{V: priorityStr, Valid: true}
		} else {
			sqlcParams.Priority = sql.Null[string]{Valid: false}
		}
	}
	if maskSet["estimated_duration"] {
		sqlcParams.EstimatedDuration = durationPtrToPgtypeInterval(params.EstimatedDuration)
	}
	if maskSet["recurrence_pattern"] {
		sqlcParams.RecurrencePattern = ptr.ToString(params.RecurrencePattern)
	}
	if maskSet["recurrence_config"] {
		if params.RecurrenceConfig != nil {
			sqlcParams.RecurrenceConfig, _ = json.Marshal(params.RecurrenceConfig)
		}
	}
	if maskSet["due_offset"] {
		sqlcParams.DueOffset = durationPtrToPgtypeInterval(params.DueOffset)
	}
	if maskSet["is_active"] {
		sqlcParams.IsActive = boolPtrToBool(params.IsActive)
	}
	if maskSet["generation_window_days"] {
		if params.GenerationWindowDays != nil {
			days := int32(*params.GenerationWindowDays)
			sqlcParams.GenerationWindowDays = int32PtrToInt4(&days)
		}
	}

	// Execute type-safe update query
	tmpl, err := s.queries.UpdateRecurringTemplate(ctx, sqlcParams)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
