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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/storage/sql/sqlcgen"
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

// FindListByID retrieves a todo list by its ID, including all items.
func (s *Store) FindListByID(ctx context.Context, id string) (*domain.TodoList, error) {
	listUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	// Get the list
	dbList, err := s.queries.GetTodoList(ctx, listUUID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: list %s", domain.ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get list: %w", err)
	}

	// Get all items for this list
	dbItems, err := s.queries.GetTodoItemsByListId(ctx, listUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get items: %w", err)
	}

	// Convert to domain model
	list := dbTodoListToDomain(dbList)
	list.Items = make([]domain.TodoItem, 0, len(dbItems))

	for _, dbItem := range dbItems {
		item := dbTodoItemToDomain(dbItem)
		list.Items = append(list.Items, item)
	}

	return list, nil
}

// FindAllLists retrieves all todo lists with counts but without items.
func (s *Store) FindAllLists(ctx context.Context) ([]*domain.TodoList, error) {
	// Get lists with counts using the SQL query that includes aggregation
	dbListsWithCounts, err := s.queries.ListTodoListsWithCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list lists: %w", err)
	}

	lists := make([]*domain.TodoList, 0, len(dbListsWithCounts))
	for _, row := range dbListsWithCounts {
		list := &domain.TodoList{
			ID:          row.ID.String(),
			Title:       row.Title,
			Items:       []domain.TodoItem{}, // Empty by design
			CreateTime:  row.CreateTime,
			TotalItems:  int(row.TotalItems),
			UndoneItems: int(row.UndoneItems),
		}
		lists = append(lists, list)
	}

	return lists, nil
}

// UpdateList updates an existing todo list metadata (title, etc).
func (s *Store) UpdateList(ctx context.Context, list *domain.TodoList) error {
	id, err := uuid.Parse(list.ID)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	params := sqlcgen.UpdateTodoListParams{
		ID:    id,
		Title: list.Title,
	}

	// Single-query pattern: check rowsAffected to detect non-existent record
	rowsAffected, err := s.queries.UpdateTodoList(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to update list: %w", err)
	}

	return checkRowsAffected(rowsAffected, "list", list.ID)
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
		return fmt.Errorf("failed to create item: %w", err)
	}

	return nil
}

// UpdateItem updates an existing todo item.
func (s *Store) UpdateItem(ctx context.Context, item *domain.TodoItem) error {
	params, err := domainTodoItemToUpdateParams(item)
	if err != nil {
		return fmt.Errorf("failed to convert item: %w", err)
	}

	// Single-query pattern: check rowsAffected to detect non-existent record
	rowsAffected, err := s.queries.UpdateTodoItem(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to update item: %w", err)
	}

	return checkRowsAffected(rowsAffected, "item", item.ID)
}

// FindItems searches for items with filtering, sorting, and pagination.
func (s *Store) FindItems(ctx context.Context, params domain.ListTasksParams) (*domain.PagedResult, error) {
	// Build the query parameters for sqlc - uses zero values to skip filters
	var zeroUUID uuid.UUID
	var zeroTime time.Time

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

	// Column5: due_before (zero time to skip)
	dueBefore := zeroTime
	if params.DueBefore != nil {
		dueBefore = *params.DueBefore
	}

	// Column6: due_after (zero time to skip)
	dueAfter := zeroTime
	if params.DueAfter != nil {
		dueAfter = *params.DueAfter
	}

	// Column7, Column8: updated_at and created_at filters (zero time to skip)
	updatedAt := zeroTime
	createdAt := zeroTime

	// Column9: order_by (empty string for default)
	orderBy := params.OrderBy

	sqlcParams := sqlcgen.ListTasksWithFiltersParams{
		Column1: listUUID,
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

	// Execute query
	dbItems, err := s.queries.ListTasksWithFilters(ctx, sqlcParams)
	if err != nil {
		return nil, fmt.Errorf("failed to list items: %w", err)
	}

	// Convert to domain items
	items := make([]domain.TodoItem, len(dbItems))
	for i, dbItem := range dbItems {
		items[i] = dbTodoItemToDomain(dbItem)
	}

	// Get total count (would need a separate count query in real implementation)
	// For now, we'll use a simple heuristic: if we got fewer items than limit, we've reached the end
	totalCount := params.Offset + len(items)
	hasMore := len(items) == params.Limit

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

	dbTemplate, err := s.queries.GetRecurringTemplate(ctx, templateUUID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: template %s", domain.ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get template: %w", err)
	}

	template, err := dbRecurringTemplateToDomain(dbTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to convert template: %w", err)
	}

	return template, nil
}

// UpdateRecurringTemplate updates an existing template.
func (s *Store) UpdateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) error {
	templateID, err := uuid.Parse(template.ID)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	// Build update parameters
	params := sqlcgen.UpdateRecurringTemplateParams{
		ID:                templateID,
		Title:             template.Title,
		RecurrencePattern: string(template.RecurrencePattern),
		UpdatedAt:         template.UpdatedAt,
	}

	// Tags
	if len(template.Tags) > 0 {
		tagsJSON, err := json.Marshal(template.Tags)
		if err != nil {
			return fmt.Errorf("failed to marshal tags: %w", err)
		}
		params.Tags.RawMessage = tagsJSON
		params.Tags.Valid = true
	}

	// Priority
	if template.Priority != nil {
		params.Priority = sql.NullString{
			String: string(*template.Priority),
			Valid:  true,
		}
	}

	// Estimated Duration
	if template.EstimatedDuration != nil {
		params.EstimatedDuration = durationToInterval(*template.EstimatedDuration)
	}

	// Recurrence Config
	if template.RecurrenceConfig != nil {
		configJSON, err := json.Marshal(template.RecurrenceConfig)
		if err != nil {
			return fmt.Errorf("failed to marshal recurrence config: %w", err)
		}
		params.RecurrenceConfig = configJSON
	}

	// Due Offset
	if template.DueOffset != nil {
		params.DueOffset = durationToInterval(*template.DueOffset)
	}

	// Single-query pattern: check rowsAffected to detect non-existent record
	rowsAffected, err := s.queries.UpdateRecurringTemplate(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to update template: %w", err)
	}

	return checkRowsAffected(rowsAffected, "template", template.ID)
}

// DeleteRecurringTemplate deletes a template.
func (s *Store) DeleteRecurringTemplate(ctx context.Context, id string) error {
	templateUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	// Single-query pattern: check rowsAffected to detect non-existent record
	rowsAffected, err := s.queries.DeleteRecurringTemplate(ctx, templateUUID)
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
		// Get all active templates across all lists, then filter by listID
		allActive, err := s.queries.ListAllActiveRecurringTemplates(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list active templates: %w", err)
		}
		// Filter for this specific list
		for _, tmpl := range allActive {
			if tmpl.ListID == listUUID {
				dbTemplates = append(dbTemplates, tmpl)
			}
		}
	} else {
		var err error
		dbTemplates, err = s.queries.ListRecurringTemplates(ctx, listUUID)
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
