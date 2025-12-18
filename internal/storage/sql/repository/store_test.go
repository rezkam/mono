package repository_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/core"
	sqlstorage "github.com/rezkam/mono/internal/storage/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostgresStorage tests basic CRUD operations with PostgreSQL.
func TestPostgresStorage(t *testing.T) {
	pgURL := os.Getenv("TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("TEST_POSTGRES_URL not set, skipping PostgreSQL tests")
	}

	ctx := context.Background()
	store, err := sqlstorage.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	// Cleanup: truncate tables after test
	defer func() {
		db, err := sql.Open("pgx", pgURL)
		if err == nil {
			db.Exec("TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
			db.Close()
		}
	}()

	// Test Create and Get List
	listUUID := "550e8400-e29b-41d4-a716-446655440099"
	list := &core.TodoList{
		ID:         listUUID,
		Title:      "Test List",
		CreateTime: time.Now(),
		Items:      []core.TodoItem{},
	}

	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	retrieved, err := store.GetList(ctx, list.ID)
	require.NoError(t, err)
	assert.Equal(t, list.Title, retrieved.Title)
	assert.Equal(t, list.ID, retrieved.ID)

	// Test ListLists
	lists, err := store.ListLists(ctx)
	require.NoError(t, err)
	assert.Len(t, lists, 1)
	assert.Equal(t, list.ID, lists[0].ID)

	// Test Create Item with Duration
	duration := 90 * time.Minute
	p := core.TaskPriorityMedium
	itemUUID := "550e8400-e29b-41d4-a716-446655440100"
	item := core.TodoItem{
		ID:                itemUUID,
		Title:             "Duration Test",
		Status:            core.TaskStatusTodo,
		CreateTime:        time.Now(),
		EstimatedDuration: &duration,
		Priority:          &p,
		UpdatedAt:         time.Now(),
	}
	list.AddItem(item)
	err = store.UpdateList(ctx, list)
	require.NoError(t, err)

	// Retrieve and check duration
	retrievedList, err := store.GetList(ctx, list.ID)
	require.NoError(t, err)
	require.Len(t, retrievedList.Items, 1)
	retrievedItem := retrievedList.Items[0]

	require.NotNil(t, retrievedItem.EstimatedDuration)
	// Check with some tolerance for float checks if needed, but exact Match for Int storage
	assert.Equal(t, duration, *retrievedItem.EstimatedDuration, "Duration should match exactly")
}

// TestListTasksPaginationExactMultiple tests pagination when total items is exact multiple of page size.
// Bug: When total items = limit (e.g., 10 items, limit=10), HasMore incorrectly returns true,
// causing clients to request an empty next page.
func TestListTasksPaginationExactMultiple(t *testing.T) {
	pgURL := os.Getenv("TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("TEST_POSTGRES_URL not set, skipping PostgreSQL tests")
	}

	ctx := context.Background()
	store, err := sqlstorage.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	// Cleanup
	defer func() {
		db, err := sql.Open("pgx", pgURL)
		if err == nil {
			db.Exec("TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
			db.Close()
		}
	}()

	// Create a list
	listUUID := "550e8400-e29b-41d4-a716-446655440201"
	list := &core.TodoList{
		ID:         listUUID,
		Title:      "Pagination Test List",
		CreateTime: time.Now(),
		Items:      []core.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create exactly 10 items (same as our page size)
	for i := 0; i < 10; i++ {
		itemID, err := uuid.NewV7()
		require.NoError(t, err)
		item := core.TodoItem{
			ID:         itemID.String(),
			Title:      "Task " + string(rune('A'+i)),
			Status:     core.TaskStatusTodo,
			CreateTime: time.Now(),
			UpdatedAt:  time.Now(),
		}
		list.AddItem(item)
	}
	err = store.UpdateList(ctx, list)
	require.NoError(t, err)

	// Test: Request first page with limit=10
	// Expected: Should get 10 items and HasMore=false (no more pages)
	// Actual Bug: HasMore=true because len(items) == limit
	result, err := store.ListTasks(ctx, core.ListTasksParams{
		ListID: &listUUID,
		Limit:  10,
		Offset: 0,
	})
	require.NoError(t, err)
	assert.Len(t, result.Items, 10, "Should return all 10 items")
	assert.False(t, result.HasMore, "HasMore should be false when we've returned all items (10 total, limit=10)")

	// Verify second page is empty (if client requests it)
	result2, err := store.ListTasks(ctx, core.ListTasksParams{
		ListID: &listUUID,
		Limit:  10,
		Offset: 10,
	})
	require.NoError(t, err)
	assert.Len(t, result2.Items, 0, "Second page should be empty")
	assert.False(t, result2.HasMore, "HasMore should be false on empty page")
}
