package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/service"
	sqlstorage "github.com/rezkam/mono/internal/storage/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestDSN(t *testing.T) string {
	pgURL := os.Getenv("TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("TEST_POSTGRES_URL not set")
	}
	return pgURL
}

// TestStatusHistoryPreservation verifies that status history is preserved
// when updating items through the service layer.
func TestStatusHistoryPreservation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create storage and service
	pgURL := getTestDSN(t)
	store, err := sqlstorage.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	svc := service.NewMonoService(store, 50, 100)

	// Create a list
	listResp, err := svc.CreateList(ctx, &monov1.CreateListRequest{
		Title: "Test List",
	})
	require.NoError(t, err)
	listID := listResp.List.Id

	// Create an item (status: TODO by default)
	itemResp, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId: listID,
		Title:  "Test Task",
	})
	require.NoError(t, err)
	itemID := itemResp.Item.Id

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
	_, err = svc.UpdateItem(ctx, &monov1.UpdateItemRequest{
		ListId: listID,
		Item: &monov1.TodoItem{
			Id:     itemID,
			Status: monov1.TaskStatus_TASK_STATUS_IN_PROGRESS,
		},
	})
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

	// First entry should be initial creation (NULL -> TODO)
	assert.Nil(t, entries[0].FromStatus, "First entry should have NULL from_status")
	assert.Equal(t, "TODO", entries[0].ToStatus)

	// Second entry should be the transition (TODO -> IN_PROGRESS)
	require.NotNil(t, entries[1].FromStatus)
	assert.Equal(t, "TODO", *entries[1].FromStatus)
	assert.Equal(t, "IN_PROGRESS", entries[1].ToStatus)
}

// TestStatusHistoryPreservationMultipleUpdates verifies that status history
// accumulates correctly across multiple status changes.
func TestStatusHistoryPreservationMultipleUpdates(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	pgURL := getTestDSN(t)
	store, err := sqlstorage.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	svc := service.NewMonoService(store, 50, 100)

	// Create list and item
	listResp, err := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
	require.NoError(t, err)

	itemResp, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId: listResp.List.Id,
		Title:  "Task",
	})
	require.NoError(t, err)
	itemID := itemResp.Item.Id

	time.Sleep(100 * time.Millisecond)

	// Transition through multiple statuses: TODO -> IN_PROGRESS -> DONE
	statuses := []monov1.TaskStatus{
		monov1.TaskStatus_TASK_STATUS_IN_PROGRESS,
		monov1.TaskStatus_TASK_STATUS_DONE,
	}

	for _, status := range statuses {
		_, err = svc.UpdateItem(ctx, &monov1.UpdateItemRequest{
			ListId: listResp.List.Id,
			Item: &monov1.TodoItem{
				Id:     itemID,
				Status: status,
			},
		})
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
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	pgURL := getTestDSN(t)
	store, err := sqlstorage.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	svc := service.NewMonoService(store, 50, 100)

	// Create list and first item
	listResp, err := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
	require.NoError(t, err)

	item1Resp, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId: listResp.List.Id,
		Title:  "Task 1",
	})
	require.NoError(t, err)
	item1ID := item1Resp.Item.Id

	time.Sleep(100 * time.Millisecond)

	// Update first item's status
	_, err = svc.UpdateItem(ctx, &monov1.UpdateItemRequest{
		ListId: listResp.List.Id,
		Item: &monov1.TodoItem{
			Id:     item1ID,
			Status: monov1.TaskStatus_TASK_STATUS_DONE,
		},
	})
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
	_, err = svc.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId: listResp.List.Id,
		Title:  "Task 2",
	})
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
