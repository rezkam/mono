package postgres

import (
	"context"
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
)

// === Todo Repository Implementation ===
// Implements application/todo.Repository interface (12 methods)

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

// FindListByID retrieves a todo list by its ID, including all items and counts.
func (s *Store) FindListByID(ctx context.Context, id string) (*domain.TodoList, error) {
	listUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	// Get the list with counts (domain defines which statuses are "undone")
	dbList, err := s.queries.GetTodoListWithCounts(ctx, sqlcgen.GetTodoListWithCountsParams{
		ID:             uuidToPgtype(listUUID),
		UndoneStatuses: domainStatusesToStrings(domain.UndoneStatuses()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: list %s", domain.ErrListNotFound, id)
		}
		return nil, fmt.Errorf("failed to get list: %w", err)
	}

	// Get all items for this list
	dbItems, err := s.queries.GetTodoItemsByListId(ctx, uuidToPgtype(listUUID))
	if err != nil {
		return nil, fmt.Errorf("failed to get items: %w", err)
	}

	// Convert to domain model
	list := &domain.TodoList{
		ID:          pgtypeToUUIDString(dbList.ID),
		Title:       dbList.Title,
		CreateTime:  pgtypeToTime(dbList.CreateTime),
		TotalItems:  int(dbList.TotalItems),
		UndoneItems: int(dbList.UndoneItems),
		Items:       make([]domain.TodoItem, 0, len(dbItems)),
	}

	for _, dbItem := range dbItems {
		item, err := dbTodoItemToDomain(dbItem)
		if err != nil {
			return nil, fmt.Errorf("failed to convert item: %w", err)
		}
		list.Items = append(list.Items, item)
	}

	return list, nil
}

// FindAllLists retrieves all todo lists with counts but without items.
func (s *Store) FindAllLists(ctx context.Context) ([]*domain.TodoList, error) {
	// Get lists with counts (domain defines which statuses are "undone")
	dbListsWithCounts, err := s.queries.ListTodoListsWithCounts(ctx, domainStatusesToStrings(domain.UndoneStatuses()))
	if err != nil {
		return nil, fmt.Errorf("failed to list lists: %w", err)
	}

	lists := make([]*domain.TodoList, 0, len(dbListsWithCounts))
	for _, row := range dbListsWithCounts {
		list := &domain.TodoList{
			ID:          pgtypeToUUIDString(row.ID),
			Title:       row.Title,
			Items:       []domain.TodoItem{}, // Empty by design
			CreateTime:  pgtypeToTime(row.CreateTime),
			TotalItems:  int(row.TotalItems),
			UndoneItems: int(row.UndoneItems),
		}
		lists = append(lists, list)
	}

	return lists, nil
}

// FindLists retrieves todo lists with filtering, sorting, and pagination.
func (s *Store) FindLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error) {
	// Build sqlc params from domain params
	sqlcParams := sqlcgen.FindTodoListsWithFiltersParams{
		UndoneStatuses: domainStatusesToStrings(domain.UndoneStatuses()),
		PageLimit:      int32(params.Limit),
		PageOffset:     int32(params.Offset),
	}

	// Apply optional filters
	if params.TitleContains != nil {
		sqlcParams.TitleContains = *params.TitleContains
	}
	if params.CreateTimeAfter != nil {
		sqlcParams.CreateTimeAfter = timeToPgtype(*params.CreateTimeAfter)
	}
	if params.CreateTimeBefore != nil {
		sqlcParams.CreateTimeBefore = timeToPgtype(*params.CreateTimeBefore)
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
			ID:          pgtypeToUUIDString(row.ID),
			Title:       row.Title,
			Items:       []domain.TodoItem{}, // Empty by design
			CreateTime:  pgtypeToTime(row.CreateTime),
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
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
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
			ID:    uuidToPgtype(id),
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
			return fmt.Errorf("%w: %v", domain.ErrListNotFound, err)
		}
		if isForeignKeyViolation(err, "recurring_template_id") {
			return fmt.Errorf("%w: %v", domain.ErrTemplateNotFound, err)
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
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	dbItem, err := s.queries.GetTodoItem(ctx, uuidToPgtype(itemUUID))
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
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	listUUID, err := uuid.Parse(params.ListID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	// Parse etag to version if provided (already validated by service layer)
	var version *int
	if params.Etag != nil {
		v, err := parseEtagToVersion(*params.Etag)
		if err != nil {
			// Should never happen if service layer validated correctly
			return nil, fmt.Errorf("failed to parse etag: %w", err)
		}
		version = &v
	}

	// Build update mask set for quick lookup
	maskSet := make(map[string]bool)
	for _, field := range params.UpdateMask {
		maskSet[field] = true
	}

	// Build SET clause dynamically with fields from update mask
	setClauses := []string{"updated_at = $1", "version = version + 1"}
	args := []any{time.Now().UTC()}
	argNum := 2

	if maskSet["title"] && params.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argNum))
		args = append(args, *params.Title)
		argNum++
	}
	if maskSet["status"] && params.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argNum))
		args = append(args, string(*params.Status))
		argNum++
	}
	if maskSet["priority"] {
		setClauses = append(setClauses, fmt.Sprintf("priority = $%d", argNum))
		if params.Priority != nil {
			args = append(args, string(*params.Priority))
		} else {
			args = append(args, nil)
		}
		argNum++
	}
	if maskSet["due_time"] {
		setClauses = append(setClauses, fmt.Sprintf("due_time = $%d", argNum))
		args = append(args, params.DueTime)
		argNum++
	}
	if maskSet["tags"] {
		setClauses = append(setClauses, fmt.Sprintf("tags = $%d", argNum))
		if params.Tags != nil {
			tagsJSON, _ := json.Marshal(*params.Tags)
			args = append(args, tagsJSON)
		} else {
			args = append(args, []byte("[]"))
		}
		argNum++
	}
	if maskSet["timezone"] {
		setClauses = append(setClauses, fmt.Sprintf("timezone = $%d", argNum))
		args = append(args, params.Timezone)
		argNum++
	}
	if maskSet["estimated_duration"] {
		setClauses = append(setClauses, fmt.Sprintf("estimated_duration = $%d", argNum))
		if params.EstimatedDuration != nil {
			args = append(args, params.EstimatedDuration.Nanoseconds())
		} else {
			args = append(args, nil)
		}
		argNum++
	}
	if maskSet["actual_duration"] {
		setClauses = append(setClauses, fmt.Sprintf("actual_duration = $%d", argNum))
		if params.ActualDuration != nil {
			args = append(args, params.ActualDuration.Nanoseconds())
		} else {
			args = append(args, nil)
		}
		argNum++
	}

	// Build WHERE clause
	whereClause := fmt.Sprintf("id = $%d AND list_id = $%d", argNum, argNum+1)
	args = append(args, itemUUID, listUUID)
	argNum += 2

	// Add version check if etag provided
	if version != nil {
		whereClause += fmt.Sprintf(" AND version = $%d", argNum)
		args = append(args, *version)
	}

	// Build full query with RETURNING
	query := fmt.Sprintf(
		`UPDATE todo_items SET %s WHERE %s
		RETURNING id, list_id, title, status, priority, estimated_duration, actual_duration,
		          create_time, updated_at, due_time, tags, recurring_template_id, instance_date, timezone, version`,
		strings.Join(setClauses, ", "),
		whereClause,
	)

	// Execute query
	row := s.pool.QueryRow(ctx, query, args...)

	// Scan result
	var item sqlcgen.TodoItem
	err = row.Scan(
		&item.ID,
		&item.ListID,
		&item.Title,
		&item.Status,
		&item.Priority,
		&item.EstimatedDuration,
		&item.ActualDuration,
		&item.CreateTime,
		&item.UpdatedAt,
		&item.DueTime,
		&item.Tags,
		&item.RecurringTemplateID,
		&item.InstanceDate,
		&item.Timezone,
		&item.Version,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Distinguish between not-found and version-conflict
			existingItem, lookupErr := s.queries.GetTodoItem(ctx, uuidToPgtype(itemUUID))
			if lookupErr != nil {
				if errors.Is(lookupErr, pgx.ErrNoRows) {
					return nil, fmt.Errorf("%w: item %s", domain.ErrItemNotFound, params.ItemID)
				}
				return nil, fmt.Errorf("failed to check item existence: %w", lookupErr)
			}

			// Item exists - check if list ID matches
			if pgtypeToUUIDString(existingItem.ListID) != params.ListID {
				return nil, fmt.Errorf("%w: item %s", domain.ErrItemNotFound, params.ItemID)
			}

			// Item exists and belongs to correct list, so update failed due to version mismatch
			if version != nil {
				return nil, fmt.Errorf("%w: expected version %d, current version %d",
					domain.ErrVersionConflict, *version, existingItem.Version)
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
func parseEtagToVersion(etag string) (int, error) {
	var version int
	_, err := fmt.Sscanf(etag, "%d", &version)
	return version, err
}

// FindItems searches for items with filtering, sorting, and pagination.
func (s *Store) FindItems(ctx context.Context, params domain.ListTasksParams) (*domain.PagedResult, error) {
	// Build the query parameters for sqlc - uses zero values to skip filters
	var zeroUUID uuid.UUID

	// Column1: list_id (zero UUID to search all lists)
	listUUID := zeroUUID
	if params.ListID != nil {
		parsed, err := uuid.Parse(*params.ListID)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
		}
		listUUID = parsed
	}

	// Column2: status (empty string to skip)
	status := ""
	if params.Status != nil {
		status = string(*params.Status)
	}

	// Column3: priority (empty string to skip)
	priority := ""
	if params.Priority != nil {
		priority = string(*params.Priority)
	}

	// Column4: tag (empty string to skip)
	tag := ""
	if params.Tag != nil {
		tag = *params.Tag
	}

	// Column5: due_before (zero time to skip filter)
	dueBefore := timePtrToPgtypeForFilter(params.DueBefore)

	// Column6: due_after (zero time to skip filter)
	dueAfter := timePtrToPgtypeForFilter(params.DueAfter)

	// Column7, Column8: updated_at and created_at filters (zero time to skip filter)
	updatedAt := timePtrToPgtypeForFilter(nil)
	createdAt := timePtrToPgtypeForFilter(nil)

	// Column9: order_by combined with direction (e.g., "created_at_desc", "due_time_asc")
	// If direction is specified, append it; otherwise use bare field name (SQL uses defaults)
	orderBy := params.OrderBy
	if params.OrderDir != "" {
		orderBy = params.OrderBy + "_" + params.OrderDir
	}

	sqlcParams := sqlcgen.ListTasksWithFiltersParams{
		Column1: uuidToPgtype(listUUID),
		Column2: status,
		Column3: priority,
		Column4: tag,
		Column5: dueBefore,
		Column6: dueAfter,
		Column7: updatedAt,
		Column8: createdAt,
		Column9: orderBy,
		Limit:   int32(params.Limit),
		Offset:  int32(params.Offset),
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
			Column1: uuidToPgtype(listUUID),
			Column2: status,
			Column3: priority,
			Column4: tag,
			Column5: dueBefore,
			Column6: dueAfter,
			Column7: updatedAt,
			Column8: createdAt,
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
			return fmt.Errorf("%w: %v", domain.ErrListNotFound, err)
		}
		return fmt.Errorf("failed to create template: %w", err)
	}

	return nil
}

// FindRecurringTemplate retrieves a template by ID.
func (s *Store) FindRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	templateUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	dbTemplate, err := s.queries.GetRecurringTemplate(ctx, uuidToPgtype(templateUUID))
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
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	// Build update mask set for quick lookup
	maskSet := make(map[string]bool)
	for _, field := range params.UpdateMask {
		maskSet[field] = true
	}

	// Build SET clause dynamically with fields from update mask
	setClauses := []string{"updated_at = $1"}
	args := []any{time.Now().UTC()}
	argNum := 2

	if maskSet["title"] && params.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argNum))
		args = append(args, *params.Title)
		argNum++
	}
	if maskSet["tags"] {
		setClauses = append(setClauses, fmt.Sprintf("tags = $%d", argNum))
		if params.Tags != nil {
			tagsJSON, _ := json.Marshal(*params.Tags)
			args = append(args, tagsJSON)
		} else {
			args = append(args, []byte("[]"))
		}
		argNum++
	}
	if maskSet["priority"] {
		setClauses = append(setClauses, fmt.Sprintf("priority = $%d", argNum))
		if params.Priority != nil {
			args = append(args, string(*params.Priority))
		} else {
			args = append(args, nil)
		}
		argNum++
	}
	if maskSet["estimated_duration"] {
		setClauses = append(setClauses, fmt.Sprintf("estimated_duration = $%d", argNum))
		if params.EstimatedDuration != nil {
			args = append(args, params.EstimatedDuration.Nanoseconds())
		} else {
			args = append(args, nil)
		}
		argNum++
	}
	if maskSet["recurrence_pattern"] && params.RecurrencePattern != nil {
		setClauses = append(setClauses, fmt.Sprintf("recurrence_pattern = $%d", argNum))
		args = append(args, string(*params.RecurrencePattern))
		argNum++
	}
	if maskSet["recurrence_config"] {
		setClauses = append(setClauses, fmt.Sprintf("recurrence_config = $%d", argNum))
		if params.RecurrenceConfig != nil {
			configJSON, _ := json.Marshal(params.RecurrenceConfig)
			args = append(args, configJSON)
		} else {
			args = append(args, nil)
		}
		argNum++
	}
	if maskSet["due_offset"] {
		setClauses = append(setClauses, fmt.Sprintf("due_offset = $%d", argNum))
		if params.DueOffset != nil {
			args = append(args, params.DueOffset.Nanoseconds())
		} else {
			args = append(args, nil)
		}
		argNum++
	}
	if maskSet["is_active"] && params.IsActive != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", argNum))
		args = append(args, *params.IsActive)
		argNum++
	}
	if maskSet["generation_window_days"] && params.GenerationWindowDays != nil {
		setClauses = append(setClauses, fmt.Sprintf("generation_window_days = $%d", argNum))
		args = append(args, *params.GenerationWindowDays)
		argNum++
	}

	// Build WHERE clause
	whereClause := fmt.Sprintf("id = $%d", argNum)
	args = append(args, templateUUID)

	// Build full query with RETURNING
	query := fmt.Sprintf(
		`UPDATE recurring_templates SET %s WHERE %s
		RETURNING id, list_id, title, tags, priority, estimated_duration, recurrence_pattern,
		          recurrence_config, due_offset, is_active, created_at, updated_at,
		          last_generated_until, generation_window_days`,
		strings.Join(setClauses, ", "),
		whereClause,
	)

	// Execute query
	row := s.pool.QueryRow(ctx, query, args...)

	// Scan result
	var tmpl sqlcgen.RecurringTaskTemplate
	err = row.Scan(
		&tmpl.ID,
		&tmpl.ListID,
		&tmpl.Title,
		&tmpl.Tags,
		&tmpl.Priority,
		&tmpl.EstimatedDuration,
		&tmpl.RecurrencePattern,
		&tmpl.RecurrenceConfig,
		&tmpl.DueOffset,
		&tmpl.IsActive,
		&tmpl.CreatedAt,
		&tmpl.UpdatedAt,
		&tmpl.LastGeneratedUntil,
		&tmpl.GenerationWindowDays,
	)
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
		return fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	// Single-query pattern: check rowsAffected to detect non-existent record
	rowsAffected, err := s.queries.DeleteRecurringTemplate(ctx, uuidToPgtype(templateUUID))
	if err != nil {
		return fmt.Errorf("failed to delete template: %w", err)
	}

	return checkRowsAffected(rowsAffected, "template", id)
}

// FindRecurringTemplates lists all templates for a list, optionally filtered by active status.
func (s *Store) FindRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*domain.RecurringTemplate, error) {
	listUUID, err := uuid.Parse(listID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	var dbTemplates []sqlcgen.RecurringTaskTemplate
	if activeOnly {
		// Get active templates for this specific list (WHERE list_id = $1 AND is_active = true)
		dbTemplates, err = s.queries.ListRecurringTemplates(ctx, uuidToPgtype(listUUID))
		if err != nil {
			return nil, fmt.Errorf("failed to list active templates: %w", err)
		}
	} else {
		// Get all templates (active and inactive) for this list (WHERE list_id = $1)
		dbTemplates, err = s.queries.ListAllRecurringTemplatesByList(ctx, uuidToPgtype(listUUID))
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
