package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/rezkam/mono/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateList_PreservesCreateTime is a regression test ensuring that
// UpdateList does NOT corrupt create_time to zero value.
//
// Bug: Prior implementation included create_time in UPDATE SET clause,
// causing it to be overwritten with Go's zero time when not explicitly set.
func TestUpdateList_PreservesCreateTime(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	pgURL := getTestDSN(t)
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	// Create a list with known create_time
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()
	originalCreateTime := time.Now().UTC().Truncate(time.Microsecond)

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Original Title",
		CreateTime: originalCreateTime,
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Verify create_time was set correctly in database
	var dbCreateTime time.Time
	err = db.QueryRowContext(ctx, `
		SELECT create_time FROM todo_lists WHERE id = $1
	`, listID).Scan(&dbCreateTime)
	require.NoError(t, err)
	require.False(t, dbCreateTime.IsZero(), "create_time should be set on creation")

	// Update the list title (UpdateList only takes Title, not CreateTime)
	list.Title = "Updated Title"
	err = store.UpdateList(ctx, list)
	require.NoError(t, err)

	// Verify create_time was NOT modified
	var afterUpdateCreateTime time.Time
	err = db.QueryRowContext(ctx, `
		SELECT create_time FROM todo_lists WHERE id = $1
	`, listID).Scan(&afterUpdateCreateTime)
	require.NoError(t, err)

	assert.Equal(t, dbCreateTime.Unix(), afterUpdateCreateTime.Unix(),
		"create_time should be immutable after creation - UpdateList must not modify it")
	assert.False(t, afterUpdateCreateTime.IsZero(),
		"create_time must not be corrupted to zero value")
}

// TestUpdateList_PreservesCreateTime_MultipleUpdates ensures create_time
// remains stable across multiple list updates.
func TestUpdateList_PreservesCreateTime_MultipleUpdates(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	pgURL := getTestDSN(t)
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Title v1",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Get original create_time
	var originalCreateTime time.Time
	err = db.QueryRowContext(ctx, `
		SELECT create_time FROM todo_lists WHERE id = $1
	`, listID).Scan(&originalCreateTime)
	require.NoError(t, err)

	// Perform multiple updates
	titles := []string{"Title v2", "Title v3", "Title v4"}
	for _, title := range titles {
		list.Title = title
		err = store.UpdateList(ctx, list)
		require.NoError(t, err)
	}

	// Verify create_time unchanged after all updates
	var finalCreateTime time.Time
	err = db.QueryRowContext(ctx, `
		SELECT create_time FROM todo_lists WHERE id = $1
	`, listID).Scan(&finalCreateTime)
	require.NoError(t, err)

	assert.Equal(t, originalCreateTime.Unix(), finalCreateTime.Unix(),
		"create_time should remain unchanged after multiple updates")
}

// TestUpdateItem_PreservesCreateTime ensures item create_time is immutable.
func TestUpdateItem_PreservesCreateTime(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	pgURL := getTestDSN(t)
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create list and item
	listResp, err := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
	require.NoError(t, err)

	itemResp, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId: listResp.List.Id,
		Title:  "Original Task",
	})
	require.NoError(t, err)
	itemID := itemResp.Item.Id

	// Get original create_time
	var originalCreateTime time.Time
	err = db.QueryRowContext(ctx, `
		SELECT create_time FROM todo_items WHERE id = $1
	`, itemID).Scan(&originalCreateTime)
	require.NoError(t, err)
	require.False(t, originalCreateTime.IsZero())

	// Update the item
	_, err = svc.UpdateItem(ctx, &monov1.UpdateItemRequest{
		ListId: listResp.List.Id,
		Item: &monov1.TodoItem{
			Id:     itemID,
			Title:  "Updated Task",
			Status: monov1.TaskStatus_TASK_STATUS_IN_PROGRESS,
		},
	})
	require.NoError(t, err)

	// Verify create_time unchanged
	var afterUpdateCreateTime time.Time
	err = db.QueryRowContext(ctx, `
		SELECT create_time FROM todo_items WHERE id = $1
	`, itemID).Scan(&afterUpdateCreateTime)
	require.NoError(t, err)

	assert.Equal(t, originalCreateTime.Unix(), afterUpdateCreateTime.Unix(),
		"item create_time should be immutable")
}

// TestListLists_ReturnsCorrectItemCounts verifies list metadata (TotalItems, UndoneItems)
// is accurate after various item operations.
func TestListLists_ReturnsCorrectItemCounts(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	pgURL := getTestDSN(t)
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list
	listResp, err := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Count Test"})
	require.NoError(t, err)
	listID := listResp.List.Id

	// Add 3 items
	for i := 0; i < 3; i++ {
		_, err = svc.CreateItem(ctx, &monov1.CreateItemRequest{
			ListId: listID,
			Title:  "Task",
		})
		require.NoError(t, err)
	}

	// Verify counts via ListLists (which includes TotalItems/UndoneItems)
	listsResp, err := svc.ListLists(ctx, &monov1.ListListsRequest{})
	require.NoError(t, err)

	var targetList *monov1.TodoList
	for _, l := range listsResp.Lists {
		if l.Id == listID {
			targetList = l
			break
		}
	}
	require.NotNil(t, targetList)

	assert.Equal(t, int32(3), targetList.TotalItems, "Should have 3 total items")
	assert.Equal(t, int32(3), targetList.UndoneItems, "All 3 items should be undone")

	// Mark one item as DONE
	getResp, err := svc.GetList(ctx, &monov1.GetListRequest{Id: listID})
	require.NoError(t, err)
	require.Len(t, getResp.List.Items, 3)

	_, err = svc.UpdateItem(ctx, &monov1.UpdateItemRequest{
		ListId: listID,
		Item: &monov1.TodoItem{
			Id:     getResp.List.Items[0].Id,
			Status: monov1.TaskStatus_TASK_STATUS_DONE,
		},
	})
	require.NoError(t, err)

	// Verify counts updated correctly
	listsResp, err = svc.ListLists(ctx, &monov1.ListListsRequest{})
	require.NoError(t, err)

	for _, l := range listsResp.Lists {
		if l.Id == listID {
			targetList = l
			break
		}
	}

	assert.Equal(t, int32(3), targetList.TotalItems, "Should still have 3 total items")
	assert.Equal(t, int32(2), targetList.UndoneItems, "Should have 2 undone items after marking one DONE")
}
