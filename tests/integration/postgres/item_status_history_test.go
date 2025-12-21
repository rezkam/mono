package integration

import (
	"context"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStatusHistoryPreservation verifies that status history is preserved
// when updating items through the service layer.
func TestStatusHistoryPreservation(t *testing.T) {
	db, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create storage and service
	pgURL := getTestDSN(t)
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	todoService := todo.NewService(store, todo.Config{})

	// Create a list
	list, err := todoService.CreateList(ctx, "Test List")
	require.NoError(t, err)
	listID := list.ID

	// Create an item (status: TODO by default)
	item := &domain.TodoItem{
		Title: "Test Task",
	}
	createdItem, err := todoService.CreateItem(ctx, listID, item)
	require.NoError(t, err)
	itemID := createdItem.ID

	// Give database triggers time to execute
	time.Sleep(100 * time.Millisecond)

	// Verify initial status history entry exists
	var initialHistoryCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM task_status_history
		WHERE task_id = $1
	`, itemID).Scan(&initialHistoryCount)
	require.NoError(t, err)
	assert.Equal(t, 1, initialHistoryCount, "Should have 1 initial status history entry")

	// Update the item's status from TODO to IN_PROGRESS
	createdItem.Status = domain.TaskStatusInProgress
	err = todoService.UpdateItem(ctx, listID, createdItem)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Check status history - should have 2 entries (initial + update)
	var finalHistoryCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM task_status_history
		WHERE task_id = $1
	`, itemID).Scan(&finalHistoryCount)
	require.NoError(t, err)

	assert.Equal(t, 2, finalHistoryCount,
		"Status history should be preserved with 2 entries (initial + update)")

	// Verify the status transitions are correct
	type HistoryEntry struct {
		FromStatus *string
		ToStatus   string
	}
	var entries []HistoryEntry
	rows, err := db.QueryContext(ctx, `
		SELECT from_status, to_status
		FROM task_status_history
		WHERE task_id = $1
		ORDER BY changed_at ASC
	`, itemID)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var entry HistoryEntry
		err := rows.Scan(&entry.FromStatus, &entry.ToStatus)
		require.NoError(t, err)
		entries = append(entries, entry)
	}

	require.Len(t, entries, 2, "Should have exactly 2 history entries")

	// First entry should be initial creation (NULL -> todo)
	assert.Nil(t, entries[0].FromStatus, "First entry should have NULL from_status")
	assert.Equal(t, "todo", entries[0].ToStatus)

	// Second entry should be the transition (todo -> in_progress)
	require.NotNil(t, entries[1].FromStatus)
	assert.Equal(t, "todo", *entries[1].FromStatus)
	assert.Equal(t, "in_progress", entries[1].ToStatus)
}

// TestStatusHistoryPreservationMultipleUpdates verifies that status history
// accumulates correctly across multiple status changes.
func TestStatusHistoryPreservationMultipleUpdates(t *testing.T) {
	db, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	pgURL := getTestDSN(t)
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	todoService := todo.NewService(store, todo.Config{})

	// Create list and item
	list, err := todoService.CreateList(ctx, "Test List")
	require.NoError(t, err)

	item := &domain.TodoItem{Title: "Task"}
	createdItem, err := todoService.CreateItem(ctx, list.ID, item)
	require.NoError(t, err)
	itemID := createdItem.ID

	time.Sleep(100 * time.Millisecond)

	// Transition through multiple statuses: todo -> in_progress -> done
	statuses := []domain.TaskStatus{
		domain.TaskStatusInProgress,
		domain.TaskStatusDone,
	}

	for _, status := range statuses {
		existingItem, err := todoService.GetItem(ctx, itemID)
		require.NoError(t, err)
		existingItem.Status = status
		err = todoService.UpdateItem(ctx, list.ID, existingItem)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
	}

	// Should have 3 entries: initial TODO, TODO->IN_PROGRESS, IN_PROGRESS->DONE
	var historyCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM task_status_history WHERE task_id = $1
	`, itemID).Scan(&historyCount)
	require.NoError(t, err)

	assert.Equal(t, 3, historyCount,
		"Should have 3 status history entries for all transitions")
}

// TestCreateItemDoesNotWipeOtherItemsHistory verifies that creating a new item
// doesn't affect the status history of existing items in the same list.
func TestCreateItemDoesNotWipeOtherItemsHistory(t *testing.T) {
	db, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	pgURL := getTestDSN(t)
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	todoService := todo.NewService(store, todo.Config{})

	// Create list and first item
	list, err := todoService.CreateList(ctx, "Test List")
	require.NoError(t, err)

	item1 := &domain.TodoItem{Title: "Task 1"}
	createdItem1, err := todoService.CreateItem(ctx, list.ID, item1)
	require.NoError(t, err)
	item1ID := createdItem1.ID

	time.Sleep(100 * time.Millisecond)

	// Update first item's status
	createdItem1.Status = domain.TaskStatusDone
	err = todoService.UpdateItem(ctx, list.ID, createdItem1)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify item1 has 2 history entries
	var item1HistoryCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM task_status_history WHERE task_id = $1
	`, item1ID).Scan(&item1HistoryCount)
	require.NoError(t, err)
	assert.Equal(t, 2, item1HistoryCount, "Item 1 should have 2 history entries before adding item 2")

	// Create second item in the same list
	item2 := &domain.TodoItem{Title: "Task 2"}
	_, err = todoService.CreateItem(ctx, list.ID, item2)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify item1's history is still intact after creating item2
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM task_status_history WHERE task_id = $1
	`, item1ID).Scan(&item1HistoryCount)
	require.NoError(t, err)

	assert.Equal(t, 2, item1HistoryCount,
		"Item 1's history should be preserved when creating item 2")
}
