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

// TestPrioritySorting_SemanticOrder verifies that priority sorting uses semantic
// order (LOW < MEDIUM < HIGH < URGENT) not alphabetical order.
//
// The SQL query uses a CASE expression to convert priority strings to
// numeric weights (LOW=1, MEDIUM=2, HIGH=3, URGENT=4) for sorting.
func TestPrioritySorting_SemanticOrder(t *testing.T) {
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
		Title:      "Priority Semantic Order Test",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create items with all four priorities
	// Create them in a non-semantic order to ensure the test is meaningful
	priorities := []domain.TaskPriority{
		domain.TaskPriorityHigh,   // Create HIGH first
		domain.TaskPriorityLow,    // Then LOW
		domain.TaskPriorityUrgent, // Then URGENT
		domain.TaskPriorityMedium, // Then MEDIUM
	}

	now := time.Now().UTC()
	itemIDs := make(map[domain.TaskPriority]string)

	for i, priority := range priorities {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)
		itemID := itemUUID.String()
		itemIDs[priority] = itemID

		p := priority
		item := &domain.TodoItem{
			ID:         itemID,
			Title:      "Priority " + string(priority),
			Status:     domain.TaskStatusTodo,
			CreateTime: now.Add(time.Duration(i) * time.Second), // Slight offset
			UpdatedAt:  now,
			Priority:   &p,
		}
		err = store.CreateItem(ctx, listID, item)
		require.NoError(t, err)
	}

	t.Run("priority_asc_returns_LOW_first", func(t *testing.T) {
		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID:   &listID,
			OrderBy:  "priority",
			OrderDir: "asc",
			Limit:    10,
			Offset:   0,
		})
		require.NoError(t, err)
		require.Len(t, result.Items, 4)

		// Semantic ASC order: LOW, MEDIUM, HIGH, URGENT
		expectedOrder := []domain.TaskPriority{
			domain.TaskPriorityLow,
			domain.TaskPriorityMedium,
			domain.TaskPriorityHigh,
			domain.TaskPriorityUrgent,
		}

		for i, expectedPriority := range expectedOrder {
			require.NotNil(t, result.Items[i].Priority,
				"Item %d should have a priority set", i)
			assert.Equal(t, expectedPriority, *result.Items[i].Priority,
				"Position %d: expected %s but got %s (semantic ASC order)",
				i, expectedPriority, *result.Items[i].Priority)
		}
	})

	t.Run("priority_desc_returns_URGENT_first", func(t *testing.T) {
		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID:   &listID,
			OrderBy:  "priority",
			OrderDir: "desc",
			Limit:    10,
			Offset:   0,
		})
		require.NoError(t, err)
		require.Len(t, result.Items, 4)

		// Semantic DESC order: URGENT, HIGH, MEDIUM, LOW
		expectedOrder := []domain.TaskPriority{
			domain.TaskPriorityUrgent,
			domain.TaskPriorityHigh,
			domain.TaskPriorityMedium,
			domain.TaskPriorityLow,
		}

		for i, expectedPriority := range expectedOrder {
			require.NotNil(t, result.Items[i].Priority,
				"Item %d should have a priority set", i)
			assert.Equal(t, expectedPriority, *result.Items[i].Priority,
				"Position %d: expected %s but got %s (semantic DESC order)",
				i, expectedPriority, *result.Items[i].Priority)
		}
	})

	t.Run("asc_and_desc_are_exact_reverses", func(t *testing.T) {
		ascResult, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID:   &listID,
			OrderBy:  "priority",
			OrderDir: "asc",
			Limit:    10,
			Offset:   0,
		})
		require.NoError(t, err)

		descResult, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID:   &listID,
			OrderBy:  "priority",
			OrderDir: "desc",
			Limit:    10,
			Offset:   0,
		})
		require.NoError(t, err)

		require.Len(t, ascResult.Items, 4)
		require.Len(t, descResult.Items, 4)

		// ASC and DESC should be exact reverses
		for i := 0; i < 4; i++ {
			assert.Equal(t, ascResult.Items[i].ID, descResult.Items[3-i].ID,
				"ASC[%d] should equal DESC[%d]", i, 3-i)
		}
	})
}

// TestPrioritySorting_NotAlphabetical verifies that HIGH does NOT come before LOW
// in ascending order (which would happen with alphabetical sorting).
func TestPrioritySorting_NotAlphabetical(t *testing.T) {
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
		Title:      "Not Alphabetical Test",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create only HIGH and LOW items (alphabetically: HIGH < LOW)
	now := time.Now().UTC()

	highPriority := domain.TaskPriorityHigh
	highItem := &domain.TodoItem{
		ID:         uuid.Must(uuid.NewV7()).String(),
		Title:      "High Priority Task",
		Status:     domain.TaskStatusTodo,
		CreateTime: now,
		UpdatedAt:  now,
		Priority:   &highPriority,
	}
	err = store.CreateItem(ctx, listID, highItem)
	require.NoError(t, err)

	lowPriority := domain.TaskPriorityLow
	lowItem := &domain.TodoItem{
		ID:         uuid.Must(uuid.NewV7()).String(),
		Title:      "Low Priority Task",
		Status:     domain.TaskStatusTodo,
		CreateTime: now.Add(time.Second),
		UpdatedAt:  now,
		Priority:   &lowPriority,
	}
	err = store.CreateItem(ctx, listID, lowItem)
	require.NoError(t, err)

	t.Run("asc_LOW_before_HIGH_not_alphabetical", func(t *testing.T) {
		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID:   &listID,
			OrderBy:  "priority",
			OrderDir: "asc",
			Limit:    10,
			Offset:   0,
		})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)

		// Alphabetically: HIGH (H) < LOW (L), so HIGH would be first
		// Semantically: LOW (1) < HIGH (3), so LOW should be first
		assert.Equal(t, domain.TaskPriorityLow, *result.Items[0].Priority,
			"ASC should return LOW first (semantic), not HIGH (alphabetical)")
		assert.Equal(t, domain.TaskPriorityHigh, *result.Items[1].Priority,
			"ASC should return HIGH second (semantic)")
	})

	t.Run("desc_HIGH_before_LOW_not_alphabetical", func(t *testing.T) {
		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID:   &listID,
			OrderBy:  "priority",
			OrderDir: "desc",
			Limit:    10,
			Offset:   0,
		})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)

		// Alphabetically DESC: LOW (L) > HIGH (H), so LOW would be first
		// Semantically DESC: HIGH (3) > LOW (1), so HIGH should be first
		assert.Equal(t, domain.TaskPriorityHigh, *result.Items[0].Priority,
			"DESC should return HIGH first (semantic), not LOW (alphabetical)")
		assert.Equal(t, domain.TaskPriorityLow, *result.Items[1].Priority,
			"DESC should return LOW second (semantic)")
	})
}

// TestPrioritySorting_WithNulls verifies that NULL priorities are handled correctly
// (sorted to the end with NULLS LAST).
func TestPrioritySorting_WithNulls(t *testing.T) {
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
		Title:      "Nulls Last Test",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	now := time.Now().UTC()

	// Create item with priority
	highPriority := domain.TaskPriorityHigh
	highItem := &domain.TodoItem{
		ID:         uuid.Must(uuid.NewV7()).String(),
		Title:      "High Priority",
		Status:     domain.TaskStatusTodo,
		CreateTime: now,
		UpdatedAt:  now,
		Priority:   &highPriority,
	}
	err = store.CreateItem(ctx, listID, highItem)
	require.NoError(t, err)

	// Create item without priority (NULL)
	nullItem := &domain.TodoItem{
		ID:         uuid.Must(uuid.NewV7()).String(),
		Title:      "No Priority",
		Status:     domain.TaskStatusTodo,
		CreateTime: now.Add(time.Second),
		UpdatedAt:  now,
		Priority:   nil, // NULL priority
	}
	err = store.CreateItem(ctx, listID, nullItem)
	require.NoError(t, err)

	lowPriority := domain.TaskPriorityLow
	lowItem := &domain.TodoItem{
		ID:         uuid.Must(uuid.NewV7()).String(),
		Title:      "Low Priority",
		Status:     domain.TaskStatusTodo,
		CreateTime: now.Add(2 * time.Second),
		UpdatedAt:  now,
		Priority:   &lowPriority,
	}
	err = store.CreateItem(ctx, listID, lowItem)
	require.NoError(t, err)

	t.Run("asc_nulls_last", func(t *testing.T) {
		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID:   &listID,
			OrderBy:  "priority",
			OrderDir: "asc",
			Limit:    10,
			Offset:   0,
		})
		require.NoError(t, err)
		require.Len(t, result.Items, 3)

		// Expected: LOW, HIGH, NULL (nulls last)
		assert.Equal(t, domain.TaskPriorityLow, *result.Items[0].Priority)
		assert.Equal(t, domain.TaskPriorityHigh, *result.Items[1].Priority)
		assert.Nil(t, result.Items[2].Priority, "NULL priority should be last in ASC")
	})

	t.Run("desc_nulls_last", func(t *testing.T) {
		result, err := store.FindItems(ctx, domain.ListTasksParams{
			ListID:   &listID,
			OrderBy:  "priority",
			OrderDir: "desc",
			Limit:    10,
			Offset:   0,
		})
		require.NoError(t, err)
		require.Len(t, result.Items, 3)

		// Expected: HIGH, LOW, NULL (nulls last)
		assert.Equal(t, domain.TaskPriorityHigh, *result.Items[0].Priority)
		assert.Equal(t, domain.TaskPriorityLow, *result.Items[1].Priority)
		assert.Nil(t, result.Items[2].Priority, "NULL priority should be last in DESC")
	})
}
