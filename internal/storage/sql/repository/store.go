package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/rezkam/mono/internal/core"
	"github.com/rezkam/mono/internal/storage/sql/sqlcgen"
)

// Store implements core.Storage using SQL databases (PostgreSQL or SQLite).
type Store struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// NewStore creates a new SQL-backed store.
func NewStore(db *sql.DB) *Store {
	return &Store{
		db:      db,
		queries: sqlcgen.New(db),
	}
}

// CreateList creates a new TodoList in the database.
func (s *Store) CreateList(ctx context.Context, list *core.TodoList) error {
	// Start a transaction to ensure atomicity
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	// Create the list
	err = qtx.CreateTodoList(ctx, sqlcgen.CreateTodoListParams{
		ID:         list.ID,
		Title:      list.Title,
		CreateTime: list.CreateTime,
	})
	if err != nil {
		return fmt.Errorf("failed to create list: %w", err)
	}

	// Create all items
	for _, item := range list.Items {
		if err := s.createItem(ctx, qtx, list.ID, item); err != nil {
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
	// Get the list
	dbList, err := s.queries.GetTodoList(ctx, id)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("list not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get list: %w", err)
	}

	// Get all items for this list
	dbItems, err := s.queries.GetTodoItemsByListId(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get items: %w", err)
	}

	// Convert to core domain models
	list := &core.TodoList{
		ID:         dbList.ID,
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

// UpdateList updates an existing TodoList.
func (s *Store) UpdateList(ctx context.Context, list *core.TodoList) error {
	// Start a transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	// Check if list exists
	_, err = qtx.GetTodoList(ctx, list.ID)
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
		ID:         list.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to update list: %w", err)
	}

	// Delete all existing items
	err = qtx.DeleteTodoItemsByListId(ctx, list.ID)
	if err != nil {
		return fmt.Errorf("failed to delete existing items: %w", err)
	}

	// Re-create all items
	for _, item := range list.Items {
		if err := s.createItem(ctx, qtx, list.ID, item); err != nil {
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
func (s *Store) ListLists(ctx context.Context) ([]*core.TodoList, error) {
	dbLists, err := s.queries.ListTodoLists(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list lists: %w", err)
	}

	if len(dbLists) == 0 {
		return []*core.TodoList{}, nil
	}

	// Build a map of list IDs to lists for efficient lookup
	listMap := make(map[string]*core.TodoList, len(dbLists))
	lists := make([]*core.TodoList, 0, len(dbLists))
	
	for _, dbList := range dbLists {
		list := &core.TodoList{
			ID:         dbList.ID,
			Title:      dbList.Title,
			CreateTime: dbList.CreateTime,
			Items:      []core.TodoItem{}, // Initialize empty slice
		}
		listMap[dbList.ID] = list
		lists = append(lists, list)
	}

	// Fetch all items for all lists in a single query (avoids N+1)
	allItems, err := s.queries.GetAllTodoItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all items: %w", err)
	}

	// Group items by list_id
	for _, dbItem := range allItems {
		if list, exists := listMap[dbItem.ListID]; exists {
			item, err := dbItemToCore(dbItem)
			if err != nil {
				return nil, fmt.Errorf("failed to convert item: %w", err)
			}
			list.Items = append(list.Items, item)
		}
	}

	return lists, nil
}

// createItem creates a single todo item (must be called within a transaction).
func (s *Store) createItem(ctx context.Context, q *sqlcgen.Queries, listID string, item core.TodoItem) error {
	// Serialize tags to JSON
	tagsJSON := sql.NullString{Valid: false}
	if len(item.Tags) > 0 {
		tagsBytes, err := json.Marshal(item.Tags)
		if err != nil {
			return fmt.Errorf("failed to marshal tags: %w", err)
		}
		tagsJSON = sql.NullString{String: string(tagsBytes), Valid: true}
	}

	dueTime := sql.NullTime{Valid: false}
	if !item.DueTime.IsZero() {
		dueTime = sql.NullTime{Time: item.DueTime, Valid: true}
	}

	completed := int32(0)
	if item.Completed {
		completed = 1
	}

	return q.CreateTodoItem(ctx, sqlcgen.CreateTodoItemParams{
		ID:         item.ID,
		ListID:     listID,
		Title:      item.Title,
		Completed:  completed,
		CreateTime: item.CreateTime,
		DueTime:    dueTime,
		Tags:       tagsJSON,
	})
}

// dbItemToCore converts a database TodoItem to a core TodoItem.
func dbItemToCore(dbItem sqlcgen.TodoItem) (core.TodoItem, error) {
	item := core.TodoItem{
		ID:         dbItem.ID,
		Title:      dbItem.Title,
		Completed:  dbItem.Completed != 0,
		CreateTime: dbItem.CreateTime,
	}

	if dbItem.DueTime.Valid {
		item.DueTime = dbItem.DueTime.Time
	}

	if dbItem.Tags.Valid && dbItem.Tags.String != "" {
		if err := json.Unmarshal([]byte(dbItem.Tags.String), &item.Tags); err != nil {
			return item, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}

	return item, nil
}
