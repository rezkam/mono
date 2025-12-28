package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListTasksPaginationExactMultiple tests pagination when total items is exact multiple of page size.
//
// Implementation uses limit+1 pattern: fetch Limit+1 rows, check if we got more than Limit, trim to Limit.
func TestListTasksPaginationExactMultiple(t *testing.T) {
	pgURL := GetTestStorageDSN(t)

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
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
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Pagination Test List",
		CreateTime: time.Now().UTC(),
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create exactly 10 items (same as our test page size)
	for i := 0; i < 10; i++ {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)

		item := &domain.TodoItem{
			ID:         itemUUID.String(),
			Title:      "Task " + string(rune('A'+i)),
			Status:     domain.TaskStatusTodo,
			CreateTime: time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}
		err = store.CreateItem(ctx, listID, item)
		require.NoError(t, err)
	}

	// Create default filter (empty)
	filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{})
	require.NoError(t, err)

	// Test: Request first page with limit=10
	// Expected: Should get 10 items and HasMore=false (no more pages)
	// CURRENT BUG: HasMore=true because len(items) == limit
	result, err := store.FindItems(ctx, domain.ListTasksParams{
		ListID: &listID,
		Filter: filter,
		Limit:  10,
		Offset: 0,
	}, nil)
	require.NoError(t, err)
	assert.Len(t, result.Items, 10, "Should return all 10 items")
	assert.False(t, result.HasMore, "HasMore should be false when we've returned all items (10 total, limit=10)")

	// Verify second page is empty (if buggy HasMore=true causes client to request it)
	result2, err := store.FindItems(ctx, domain.ListTasksParams{
		ListID: &listID,
		Filter: filter,
		Limit:  10,
		Offset: 10,
	}, nil)
	require.NoError(t, err)
	assert.Len(t, result2.Items, 0, "Second page should be empty")
	assert.False(t, result2.HasMore, "HasMore should be false on empty page")
}
