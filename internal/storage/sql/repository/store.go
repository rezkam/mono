package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lib/pq"
	"github.com/rezkam/mono/internal/core"
	"github.com/rezkam/mono/internal/storage/sql/sqlcgen"
	"github.com/sqlc-dev/pqtype"
)

// Common errors returned by storage layer
var (
	ErrListNotFound = errors.New("list not found")
)

// isForeignKeyViolation checks if an error is a PostgreSQL FK violation
func isForeignKeyViolation(err error, column string) bool {
	if pqErr, ok := err.(*pq.Error); ok {
		// 23503 is foreign_key_violation
		if pqErr.Code == "23503" {
			if column == "" {
				return true
			}
			// Check if the constraint name or message contains the column
			return strings.Contains(pqErr.Constraint, column) ||
				strings.Contains(pqErr.Message, column)
		}
	}
	return false
}

// Store implements core.Storage using PostgreSQL.
type Store struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// DB returns the underlying database connection.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Queries returns the sqlc-generated queries.
func (s *Store) Queries() *sqlcgen.Queries {
	return s.queries
}

// NewStore creates a new PostgreSQL-backed store.
func NewStore(db *sql.DB) *Store {
	return &Store{
		db:      db,
		queries: sqlcgen.New(db),
	}
}

// Close closes the database connection.
// This should be called when shutting down the application.
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateList creates a new TodoList in the database.
// CreateList creates a new TodoList in the database.
// CreateList creates a new TodoList in the database.
func (s *Store) CreateList(ctx context.Context, list *core.TodoList) error {
	// Start a transaction to ensure atomicity
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	listID, err := uuid.Parse(list.ID)
	if err != nil {
		return fmt.Errorf("invalid list id: %w", err)
	}

	// Create the list
	err = qtx.CreateTodoList(ctx, sqlcgen.CreateTodoListParams{
		ID:         listID,
		Title:      list.Title,
		CreateTime: list.CreateTime,
	})
	if err != nil {
		return fmt.Errorf("failed to create list: %w", err)
	}

	// Create all items
	for _, item := range list.Items {
		if err := s.createItem(ctx, qtx, listID, item); err != nil {
			return fmt.Errorf("failed to create item: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetList retrieves a TodoList by its ID.
func (s *Store) GetList(ctx context.Context, id string) (*core.TodoList, error) {
	listUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid list id: %w", err)
	}

	// Get the list
	dbList, err := s.queries.GetTodoList(ctx, listUUID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("list not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get list: %w", err)
	}

	// Get all items for this list
	dbItems, err := s.queries.GetTodoItemsByListId(ctx, listUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get items: %w", err)
	}

	// Convert to core domain models
	list := &core.TodoList{
		ID:         dbList.ID.String(),
		Title:      dbList.Title,
		CreateTime: dbList.CreateTime,
		Items:      make([]core.TodoItem, 0, len(dbItems)),
	}

	for _, dbItem := range dbItems {
		item, err := dbItemToCore(dbItem)
		if err != nil {
			return nil, fmt.Errorf("failed to convert item: %w", err)
		}
		list.Items = append(list.Items, item)
	}

	return list, nil
}

// UpdateList updates an existing TodoList including all its items.
//
// WARNING: This method deletes all existing items and recreates them,
// which WILL CASCADE DELETE status history and other related data.
//
// For item-level operations, use CreateTodoItem or UpdateTodoItem instead
// to preserve audit trails and status history.
//
// This method is primarily intended for bulk operations where history loss
// is acceptable (e.g., test cleanup).
func (s *Store) UpdateList(ctx context.Context, list *core.TodoList) error {
	// Start a transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	listID, err := uuid.Parse(list.ID)
	if err != nil {
		return fmt.Errorf("invalid list id: %w", err)
	}

	// Check if list exists
	_, err = qtx.GetTodoList(ctx, listID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("list not found: %s", list.ID)
	}
	if err != nil {
		return fmt.Errorf("failed to check list existence: %w", err)
	}

	// Update the list
	err = qtx.UpdateTodoList(ctx, sqlcgen.UpdateTodoListParams{
		Title:      list.Title,
		CreateTime: list.CreateTime,
		ID:         listID,
	})
	if err != nil {
		return fmt.Errorf("failed to update list: %w", err)
	}

	// Delete all existing items
	err = qtx.DeleteTodoItemsByListId(ctx, listID)
	if err != nil {
		return fmt.Errorf("failed to delete existing items: %w", err)
	}

	// Re-create all items
	for _, item := range list.Items {
		if err := s.createItem(ctx, qtx, listID, item); err != nil {
			return fmt.Errorf("failed to create item: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ListLists returns all TodoLists.
// This method is optimized to avoid N+1 queries by fetching all items in a single batch query.
// ListLists returns all lists with item counts, optimized for dashboard/list views.
//
// ACCESS PATTERN: LIST VIEW (dashboard showing list summaries)
// Performance: Single SQL query with aggregation vs N+1 queries loading all items.
//
// Returns TodoList with:
//   - Items: Empty slice (not loaded for performance)
//   - TotalItems: Count of all items
//   - UndoneItems: Count of active items (TODO, IN_PROGRESS, BLOCKED)
//
// Use GetList() instead if you need the actual items.
func (s *Store) ListLists(ctx context.Context) ([]*core.TodoList, error) {
	// Use the optimized query that returns counts without loading items
	dbLists, err := s.queries.ListTodoListsWithCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list lists: %w", err)
	}

	if len(dbLists) == 0 {
		return []*core.TodoList{}, nil
	}

	lists := make([]*core.TodoList, 0, len(dbLists))
	for _, dbList := range dbLists {
		list := &core.TodoList{
			ID:          dbList.ID.String(),
			Title:       dbList.Title,
			CreateTime:  dbList.CreateTime,
			Items:       []core.TodoItem{}, // Empty slice - items not loaded for list view
			TotalItems:  int(dbList.TotalItems),
			UndoneItems: int(dbList.UndoneItems),
		}
		lists = append(lists, list)
	}

	return lists, nil
}

// CreateTodoItem creates a new item in a list without affecting existing status history.
// This preserves the audit trail when adding items to a list.
func (s *Store) CreateTodoItem(ctx context.Context, listID string, item core.TodoItem) error {
	listUUID, err := uuid.Parse(listID)
	if err != nil {
		return fmt.Errorf("invalid list id: %w", err)
	}

	// Use a transaction to ensure atomicity
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	// Create the item
	if err := s.createItem(ctx, qtx, listUUID, item); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// UpdateTodoItem updates an existing item's fields without deleting and recreating it.
// This preserves the status history and audit trail.
func (s *Store) UpdateTodoItem(ctx context.Context, item core.TodoItem) error {
	itemID, err := uuid.Parse(item.ID)
	if err != nil {
		return fmt.Errorf("invalid item id: %w", err)
	}

	// Use a transaction to ensure atomicity
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	// Serialize tags to JSON
	tagsJSON := pqtype.NullRawMessage{Valid: false}
	if len(item.Tags) > 0 {
		tagsBytes, err := json.Marshal(item.Tags)
		if err != nil {
			return fmt.Errorf("failed to marshal tags: %w", err)
		}
		tagsJSON = pqtype.NullRawMessage{RawMessage: tagsBytes, Valid: true}
	}

	dueTime := sql.NullTime{Valid: false}
	if item.DueTime != nil {
		dueTime = sql.NullTime{Time: *item.DueTime, Valid: true}
	}

	priority := sql.NullString{Valid: false}
	if item.Priority != nil {
		priority = sql.NullString{String: string(*item.Priority), Valid: true}
	}

	estDuration := pgtype.Interval{}
	if item.EstimatedDuration != nil {
		estDuration = pgtype.Interval{Microseconds: int64(*item.EstimatedDuration / time.Microsecond), Valid: true}
	}

	actDuration := pgtype.Interval{}
	if item.ActualDuration != nil {
		actDuration = pgtype.Interval{Microseconds: int64(*item.ActualDuration / time.Microsecond), Valid: true}
	}

	timezone := sql.NullString{Valid: false}
	if item.Timezone != nil && *item.Timezone != "" {
		timezone = sql.NullString{String: *item.Timezone, Valid: true}
	}

	// Update the item directly without deleting it
	err = qtx.UpdateTodoItem(ctx, sqlcgen.UpdateTodoItemParams{
		ID:                itemID,
		Title:             item.Title,
		Status:            string(item.Status),
		Priority:          priority,
		EstimatedDuration: estDuration,
		ActualDuration:    actDuration,
		UpdatedAt:         item.UpdatedAt,
		DueTime:           dueTime,
		Tags:              tagsJSON,
		Timezone:          timezone,
	})
	if err != nil {
		return fmt.Errorf("failed to update item: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// createItem creates a single todo item (must be called within a transaction).
func (s *Store) createItem(ctx context.Context, q *sqlcgen.Queries, listID uuid.UUID, item core.TodoItem) error {
	itemID, err := uuid.Parse(item.ID)
	if err != nil {
		return fmt.Errorf("invalid item id: %w", err)
	}

	// Serialize tags to JSON
	tagsJSON := pqtype.NullRawMessage{Valid: false}
	if len(item.Tags) > 0 {
		tagsBytes, err := json.Marshal(item.Tags)
		if err != nil {
			return fmt.Errorf("failed to marshal tags: %w", err)
		}
		tagsJSON = pqtype.NullRawMessage{RawMessage: tagsBytes, Valid: true}
	}

	dueTime := sql.NullTime{Valid: false}
	if item.DueTime != nil {
		dueTime = sql.NullTime{Time: *item.DueTime, Valid: true}
	}

	priority := sql.NullString{Valid: false}
	if item.Priority != nil {
		priority = sql.NullString{String: string(*item.Priority), Valid: true}
	}

	estDuration := pgtype.Interval{}
	if item.EstimatedDuration != nil {
		estDuration = pgtype.Interval{Microseconds: int64(*item.EstimatedDuration / time.Microsecond), Valid: true}
	}

	actDuration := pgtype.Interval{}
	if item.ActualDuration != nil {
		actDuration = pgtype.Interval{Microseconds: int64(*item.ActualDuration / time.Microsecond), Valid: true}
	}

	recurringTemplateID := uuid.NullUUID{Valid: false}
	if item.RecurringTemplateID != nil {
		id, err := uuid.Parse(*item.RecurringTemplateID)
		if err != nil {
			return fmt.Errorf("invalid recurring template id: %w", err)
		}
		recurringTemplateID = uuid.NullUUID{UUID: id, Valid: true}
	}

	instanceDate := sql.NullTime{Valid: false}
	if item.InstanceDate != nil {
		instanceDate = sql.NullTime{Time: *item.InstanceDate, Valid: true}
	}

	timezone := sql.NullString{Valid: false}
	if item.Timezone != nil && *item.Timezone != "" {
		timezone = sql.NullString{String: *item.Timezone, Valid: true}
	}

	err = q.CreateTodoItem(ctx, sqlcgen.CreateTodoItemParams{
		ID:                  itemID,
		ListID:              listID,
		Title:               item.Title,
		Status:              string(item.Status),
		Priority:            priority,
		EstimatedDuration:   estDuration,
		ActualDuration:      actDuration,
		CreateTime:          item.CreateTime,
		UpdatedAt:           item.UpdatedAt,
		DueTime:             dueTime,
		Tags:                tagsJSON,
		RecurringTemplateID: recurringTemplateID,
		InstanceDate:        instanceDate,
		Timezone:            timezone,
	})
	if err != nil {
		if isForeignKeyViolation(err, "list_id") {
			return fmt.Errorf("%w: %v", ErrListNotFound, err)
		}
		return fmt.Errorf("failed to create item: %w", err)
	}
	return nil
}

// dbItemToCore converts a database TodoItem to a core TodoItem.
func dbItemToCore(dbItem sqlcgen.TodoItem) (core.TodoItem, error) {
	item := core.TodoItem{
		ID:         dbItem.ID.String(),
		Title:      dbItem.Title,
		Status:     core.TaskStatus(dbItem.Status),
		CreateTime: dbItem.CreateTime,
		UpdatedAt:  dbItem.UpdatedAt,
	}

	if dbItem.DueTime.Valid {
		t := dbItem.DueTime.Time
		item.DueTime = &t
	}

	if dbItem.Priority.Valid {
		p := core.TaskPriority(dbItem.Priority.String)
		item.Priority = &p
	}

	if dbItem.EstimatedDuration.Valid {
		d := time.Duration(dbItem.EstimatedDuration.Microseconds) * time.Microsecond
		item.EstimatedDuration = &d
	}

	if dbItem.ActualDuration.Valid {
		d := time.Duration(dbItem.ActualDuration.Microseconds) * time.Microsecond
		item.ActualDuration = &d
	}

	if dbItem.Tags.Valid && len(dbItem.Tags.RawMessage) > 0 {
		if err := json.Unmarshal(dbItem.Tags.RawMessage, &item.Tags); err != nil {
			return item, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}

	if dbItem.RecurringTemplateID.Valid {
		id := dbItem.RecurringTemplateID.UUID.String()
		item.RecurringTemplateID = &id
	}

	if dbItem.InstanceDate.Valid {
		t := dbItem.InstanceDate.Time
		item.InstanceDate = &t
	}

	if dbItem.Timezone.Valid && dbItem.Timezone.String != "" {
		tz := dbItem.Timezone.String
		item.Timezone = &tz
	}

	return item, nil
}

// ListTasks returns tasks with filtering, sorting, and pagination.
//
// ACCESS PATTERN: SEARCH/FILTER VIEW (task searches, filtered lists, pagination)
// Performance: All operations (filter, sort, paginate) happen in PostgreSQL.
//
// Common use cases with benchmarks:
//   - Search across all lists: params.ListID = nil
//   - Filter by list: params.ListID = "list-uuid"
//   - Overdue tasks: params.DueBefore = time.Now()
//   - High priority: params.Priority = HIGH, params.Status = TODO
//
// The SQL query uses indexes on (list_id, status, priority, due_time) for optimal performance.
// Pagination is cursor-based using offset (can be optimized to keyset pagination if needed).
func (s *Store) ListTasks(ctx context.Context, params core.ListTasksParams) (*core.ListTasksResult, error) {
	// Convert core params to SQL params - use zero values for NULL
	var listID uuid.UUID
	if params.ListID != nil {
		parsedID, err := uuid.Parse(*params.ListID)
		if err != nil {
			return nil, fmt.Errorf("invalid list id: %w", err)
		}
		listID = parsedID
	}

	var status string
	if params.Status != nil {
		status = string(*params.Status)
	}

	var priority string
	if params.Priority != nil {
		priority = string(*params.Priority)
	}

	var tag string
	if params.Tag != nil {
		tag = *params.Tag
	}

	var dueBefore time.Time
	if params.DueBefore != nil {
		dueBefore = *params.DueBefore
	}

	var dueAfter time.Time
	if params.DueAfter != nil {
		dueAfter = *params.DueAfter
	}

	// Default orderBy if not specified
	orderBy := params.OrderBy
	if orderBy == "" {
		orderBy = "created_at"
	}

	// Execute the query - fetch limit+1 to detect if there are more items
	// (Column4 is tag string, Column5-8 are timestamps, Column9 is orderBy string)
	queryParams := sqlcgen.ListTasksWithFiltersParams{
		Column1: listID,
		Column2: status,
		Column3: priority,
		Column4: tag,
		Column5: dueBefore,
		Column6: dueAfter,
		Column7: time.Time{}, // updated_at filter (not used yet)
		Column8: time.Time{}, // create_time filter (not used yet)
		Column9: orderBy,
		Limit:   int32(params.Limit + 1), // Fetch one extra to detect if there are more
		Offset:  int32(params.Offset),
	}
	dbItems, err := s.queries.ListTasksWithFilters(ctx, queryParams)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	// Check if there are more items beyond the requested limit
	hasMore := len(dbItems) > params.Limit
	if hasMore {
		// Trim to requested limit
		dbItems = dbItems[:params.Limit]
	}

	// Convert to core items
	items := make([]core.TodoItem, 0, len(dbItems))
	for _, dbItem := range dbItems {
		item, err := dbItemToCore(dbItem)
		if err != nil {
			return nil, fmt.Errorf("failed to convert item: %w", err)
		}
		items = append(items, item)
	}

	return &core.ListTasksResult{
		Items:      items,
		TotalCount: len(items), // This is the current page count, not total - we can enhance later
		HasMore:    hasMore,
	}, nil
}
