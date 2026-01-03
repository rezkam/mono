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

// TestOrderByDirection_HonorsAscDescKeywords verifies that the order_by parameter
// actually honors the direction (asc/desc) keyword, not just the field name.
//
// BUG: The current implementation accepts direction keywords but silently ignores them.
// The SQL has hardcoded directions (due_at ASC, created_at DESC, etc.) regardless
// of what the client requests.
//
// This test ensures:
// - "created_at asc" returns oldest items first
// - "created_at desc" returns newest items first
// - The directions are NOT the same (proving the direction is honored)
func TestOrderByDirection_HonorsAscDescKeywords(t *testing.T) {
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
		ID:        listID,
		Title:     "Order By Direction Test",
		CreatedAt: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create 5 items with distinctly different create times
	// We'll use manual create times to ensure predictable ordering
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	itemIDs := make([]string, 5)

	for i := range 5 {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)
		itemIDs[i] = itemUUID.String()

		// Each item created 1 hour apart
		createTime := baseTime.Add(time.Duration(i) * time.Hour)

		item := &domain.TodoItem{
			ID:        itemIDs[i],
			Title:     "Task " + string(rune('A'+i)), // Task A, B, C, D, E
			Status:    domain.TaskStatusTodo,
			CreatedAt: createTime,
			UpdatedAt: createTime,
		}
		_, err = store.CreateItem(ctx, listID, item)
		require.NoError(t, err)
	}

	t.Run("created_at_asc_returns_oldest_first", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			OrderBy:  ptrString("created_at"),
			OrderDir: ptrString("asc"),
		})
		require.NoError(t, err)

		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
			Limit:  10,
			Offset: 0,
		}, nil)
		require.NoError(t, err)
		require.Len(t, result.Items, 5)

		// With ASC, oldest (Task A) should be first, newest (Task E) should be last
		assert.Equal(t, "Task A", result.Items[0].Title,
			"ASC order should return oldest item (Task A) first")
		assert.Equal(t, "Task E", result.Items[4].Title,
			"ASC order should return newest item (Task E) last")

		// Verify monotonically increasing create times
		for i := 1; i < len(result.Items); i++ {
			assert.True(t, result.Items[i].CreatedAt.After(result.Items[i-1].CreatedAt) ||
				result.Items[i].CreatedAt.Equal(result.Items[i-1].CreatedAt),
				"ASC order: item %d should have created_at >= item %d", i, i-1)
		}
	})

	t.Run("created_at_desc_returns_newest_first", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			OrderBy:  ptrString("created_at"),
			OrderDir: ptrString("desc"),
		})
		require.NoError(t, err)

		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
			Limit:  10,
			Offset: 0,
		}, nil)
		require.NoError(t, err)
		require.Len(t, result.Items, 5)

		// With DESC, newest (Task E) should be first, oldest (Task A) should be last
		assert.Equal(t, "Task E", result.Items[0].Title,
			"DESC order should return newest item (Task E) first")
		assert.Equal(t, "Task A", result.Items[4].Title,
			"DESC order should return oldest item (Task A) last")

		// Verify monotonically decreasing create times
		for i := 1; i < len(result.Items); i++ {
			assert.True(t, result.Items[i].CreatedAt.Before(result.Items[i-1].CreatedAt) ||
				result.Items[i].CreatedAt.Equal(result.Items[i-1].CreatedAt),
				"DESC order: item %d should have created_at <= item %d", i, i-1)
		}
	})

	t.Run("asc_and_desc_produce_different_orders", func(t *testing.T) {
		ascFilter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			OrderBy:  ptrString("created_at"),
			OrderDir: ptrString("asc"),
		})
		require.NoError(t, err)

		ascResult, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID: &listID,
			Filter: ascFilter,
			Limit:  10,
			Offset: 0,
		}, nil)
		require.NoError(t, err)

		descFilter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			OrderBy:  ptrString("created_at"),
			OrderDir: ptrString("desc"),
		})
		require.NoError(t, err)

		descResult, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID: &listID,
			Filter: descFilter,
			Limit:  10,
			Offset: 0,
		}, nil)
		require.NoError(t, err)

		require.Len(t, ascResult.Items, 5)
		require.Len(t, descResult.Items, 5)

		// The first item in ASC should be the last item in DESC
		assert.Equal(t, ascResult.Items[0].ID, descResult.Items[4].ID,
			"First item in ASC should be last item in DESC - proves direction is honored")
		assert.Equal(t, ascResult.Items[4].ID, descResult.Items[0].ID,
			"Last item in ASC should be first item in DESC - proves direction is honored")

		// The orders should be exactly reversed
		for i := range 5 {
			assert.Equal(t, ascResult.Items[i].ID, descResult.Items[4-i].ID,
				"ASC[%d] should equal DESC[%d]", i, 4-i)
		}
	})
}

// TestOrderByDirection_DueAt verifies due_at sorting honors direction.
func TestOrderByDirection_DueAt(t *testing.T) {
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
		ID:        listID,
		Title:     "Due Time Order Test",
		CreatedAt: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create items with different due times
	baseTime := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	now := time.Now().UTC()

	for i := range 3 {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)

		dueTime := baseTime.Add(time.Duration(i) * 24 * time.Hour) // 1 day apart

		item := &domain.TodoItem{
			ID:        itemUUID.String(),
			Title:     "Due Task " + string(rune('A'+i)),
			Status:    domain.TaskStatusTodo,
			CreatedAt: now,
			UpdatedAt: now,
			DueAt:     &dueTime,
		}
		_, err = store.CreateItem(ctx, listID, item)
		require.NoError(t, err)
	}

	t.Run("due_time_asc_returns_earliest_due_first", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			OrderBy:  ptrString("due_at"),
			OrderDir: ptrString("asc"),
		})
		require.NoError(t, err)

		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
			Limit:  10,
			Offset: 0,
		}, nil)
		require.NoError(t, err)
		require.Len(t, result.Items, 3)

		// ASC: earliest due (Task A) first
		assert.Equal(t, "Due Task A", result.Items[0].Title,
			"due_at ASC should return earliest due first")
		assert.Equal(t, "Due Task C", result.Items[2].Title,
			"due_at ASC should return latest due last")
	})

	t.Run("due_time_desc_returns_latest_due_first", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			OrderBy:  ptrString("due_at"),
			OrderDir: ptrString("desc"),
		})
		require.NoError(t, err)

		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
			Limit:  10,
			Offset: 0,
		}, nil)
		require.NoError(t, err)
		require.Len(t, result.Items, 3)

		// DESC: latest due (Task C) first
		assert.Equal(t, "Due Task C", result.Items[0].Title,
			"due_at DESC should return latest due first")
		assert.Equal(t, "Due Task A", result.Items[2].Title,
			"due_at DESC should return earliest due last")
	})
}

// TestOrderByDirection_Priority verifies priority sorting honors direction.
func TestOrderByDirection_Priority(t *testing.T) {
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
		ID:        listID,
		Title:     "Priority Order Test",
		CreatedAt: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create items with different priorities (HIGH, MEDIUM, LOW)
	// Priority string comparison: HIGH < LOW < MEDIUM (alphabetically)
	now := time.Now().UTC()
	priorities := []domain.TaskPriority{domain.TaskPriorityLow, domain.TaskPriorityMedium, domain.TaskPriorityHigh}

	for i, priority := range priorities {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)

		p := priority
		item := &domain.TodoItem{
			ID:        itemUUID.String(),
			Title:     "Priority " + string(priority),
			Status:    domain.TaskStatusTodo,
			CreatedAt: now.Add(time.Duration(i) * time.Second), // Slight offset to avoid ties
			UpdatedAt: now,
			Priority:  &p,
		}
		_, err = store.CreateItem(ctx, listID, item)
		require.NoError(t, err)
	}

	t.Run("priority_asc_and_desc_produce_different_orders", func(t *testing.T) {
		ascFilter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			OrderBy:  ptrString("priority"),
			OrderDir: ptrString("asc"),
		})
		require.NoError(t, err)

		ascResult, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID: &listID,
			Filter: ascFilter,
			Limit:  10,
			Offset: 0,
		}, nil)
		require.NoError(t, err)

		descFilter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			OrderBy:  ptrString("priority"),
			OrderDir: ptrString("desc"),
		})
		require.NoError(t, err)

		descResult, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID: &listID,
			Filter: descFilter,
			Limit:  10,
			Offset: 0,
		}, nil)
		require.NoError(t, err)

		require.Len(t, ascResult.Items, 3)
		require.Len(t, descResult.Items, 3)

		// The orders should be reversed
		assert.Equal(t, ascResult.Items[0].ID, descResult.Items[2].ID,
			"First item in ASC should be last item in DESC")
		assert.Equal(t, ascResult.Items[2].ID, descResult.Items[0].ID,
			"Last item in ASC should be first item in DESC")
	})
}
