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

// TestFindItems_TotalCount_ReturnsActualTotal tests that TotalCount returns the actual
// total number of matching items across all pages.
//
// Uses COUNT(*) OVER() window function to get total count with each row,
// or run a separate count query.
func TestFindItems_TotalCount_ReturnsActualTotal(t *testing.T) {
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
		Title:      "TotalCount Test List",
		CreateTime: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create 50 items
	const totalItems = 50
	for i := 0; i < totalItems; i++ {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)

		item := &domain.TodoItem{
			ID:         itemUUID.String(),
			Title:      "Task " + string(rune('A'+i%26)) + string(rune('0'+i/26)),
			Status:     domain.TaskStatusTodo,
			CreateTime: time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}
		_, err = store.CreateItem(ctx, listID, item)
		require.NoError(t, err)
	}

	// Create default filter (empty)
	filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{})
	require.NoError(t, err)

	t.Run("FirstPage_TotalCountShouldBe50", func(t *testing.T) {
		// Request first page with limit=10
		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
			Limit:  10,
			Offset: 0,
		}, nil)
		require.NoError(t, err)

		assert.Len(t, result.Items, 10, "Should return 10 items on first page")
		assert.True(t, result.HasMore, "HasMore should be true (40 more items)")
		assert.Equal(t, totalItems, result.TotalCount,
			"TotalCount should be 50 (actual total), not 10 (offset+len)")
	})

	t.Run("MiddlePage_TotalCountShouldBe50", func(t *testing.T) {
		// Request third page (offset=20)
		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
			Limit:  10,
			Offset: 20,
		}, nil)
		require.NoError(t, err)

		assert.Len(t, result.Items, 10, "Should return 10 items on middle page")
		assert.True(t, result.HasMore, "HasMore should be true (20 more items)")
		assert.Equal(t, totalItems, result.TotalCount,
			"TotalCount should be 50 (actual total), not 30 (offset+len)")
	})

	t.Run("LastPage_TotalCountShouldBe50", func(t *testing.T) {
		// Request last page (offset=40)
		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
			Limit:  10,
			Offset: 40,
		}, nil)
		require.NoError(t, err)

		assert.Len(t, result.Items, 10, "Should return 10 items on last page")
		assert.False(t, result.HasMore, "HasMore should be false (no more items)")
		assert.Equal(t, totalItems, result.TotalCount,
			"TotalCount should be 50 (actual total), not 50 (offset+len)")
	})

	t.Run("EmptyPage_TotalCountShouldBe50", func(t *testing.T) {
		// Request beyond last page (offset=50)
		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
			Limit:  10,
			Offset: 50,
		}, nil)
		require.NoError(t, err)

		assert.Len(t, result.Items, 0, "Should return 0 items beyond last page")
		assert.False(t, result.HasMore, "HasMore should be false")
		assert.Equal(t, totalItems, result.TotalCount,
			"TotalCount should still be 50 even on empty page")
	})
}

// TestFindItems_TotalCount_WithFilters tests that TotalCount reflects filtered results.
func TestFindItems_TotalCount_WithFilters(t *testing.T) {
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
		Title:      "Filter TotalCount Test",
		CreateTime: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create 30 TODO items and 20 DONE items
	for i := 0; i < 30; i++ {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)

		item := &domain.TodoItem{
			ID:         itemUUID.String(),
			Title:      "Todo Task " + string(rune('A'+i%26)),
			Status:     domain.TaskStatusTodo,
			CreateTime: time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}
		_, err = store.CreateItem(ctx, listID, item)
		require.NoError(t, err)
	}

	for i := 0; i < 20; i++ {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)

		item := &domain.TodoItem{
			ID:         itemUUID.String(),
			Title:      "Done Task " + string(rune('A'+i%26)),
			Status:     domain.TaskStatusDone,
			CreateTime: time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}
		_, err = store.CreateItem(ctx, listID, item)
		require.NoError(t, err)
	}

	t.Run("FilterByStatus_TotalCountReflectsFilter", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Statuses: []string{"todo"},
		})
		require.NoError(t, err)

		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
			Limit:  10,
			Offset: 0,
		}, nil)
		require.NoError(t, err)

		assert.Len(t, result.Items, 10, "Should return 10 TODO items")
		assert.True(t, result.HasMore, "HasMore should be true (20 more TODO items)")
		assert.Equal(t, 30, result.TotalCount,
			"TotalCount should be 30 (TODO items only), not 10")
	})

	t.Run("FilterByStatus_Done_TotalCountReflectsFilter", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Statuses: []string{"done"},
		})
		require.NoError(t, err)

		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
			Limit:  10,
			Offset: 0,
		}, nil)
		require.NoError(t, err)

		assert.Len(t, result.Items, 10, "Should return 10 DONE items")
		assert.True(t, result.HasMore, "HasMore should be true (10 more DONE items)")
		assert.Equal(t, 20, result.TotalCount,
			"TotalCount should be 20 (DONE items only), not 10")
	})
}
