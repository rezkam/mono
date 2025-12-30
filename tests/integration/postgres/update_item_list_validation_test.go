package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateItem_ValidatesListOwnership verifies that UpdateItem prevents
// cross-list updates by validating the item belongs to the specified list.
// The SQL UPDATE includes `WHERE id = $1 AND list_id = $2` to ensure
// updates only succeed if the item belongs to the specified list.
func TestUpdateItem_ValidatesListOwnership(t *testing.T) {
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

	todoService := todo.NewService(store, todo.Config{})

	// Create two separate lists
	list1, err := todoService.CreateList(ctx, "List 1")
	require.NoError(t, err)
	list1ID := list1.ID

	list2, err := todoService.CreateList(ctx, "List 2")
	require.NoError(t, err)
	list2ID := list2.ID

	// Create an item in list1
	item, err := todoService.CreateItem(ctx, list1ID, &domain.TodoItem{
		Title: "Original Title",
	})
	require.NoError(t, err)
	itemID := item.ID

	t.Run("update_with_correct_list_id_succeeds", func(t *testing.T) {
		// Update the item with the correct list_id (list1)
		updateItem := &domain.TodoItem{
			ID:     itemID,
			Title:  "Updated Title",
			Status: domain.TaskStatusInProgress,
		}

		_, err := todoService.UpdateItem(ctx, ItemToUpdateParams(list1ID, updateItem))
		require.NoError(t, err)

		// Fetch and verify
		fetchedItem, err := todoService.GetItem(ctx, itemID)
		require.NoError(t, err)
		assert.Equal(t, "Updated Title", fetchedItem.Title)
		assert.Equal(t, domain.TaskStatusInProgress, fetchedItem.Status)
	})

	t.Run("update_with_wrong_list_id_fails", func(t *testing.T) {
		// Attempt to update the item with the wrong list_id (list2)
		// This should fail even though the item exists - it's in list1, not list2
		maliciousUpdate := &domain.TodoItem{
			ID:     itemID,
			Title:  "Malicious Update",
			Status: domain.TaskStatusDone,
		}

		_, err := todoService.UpdateItem(ctx, ItemToUpdateParams(list2ID, maliciousUpdate)) // WRONG: item belongs to list1
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found", "Cross-list update should return NotFound error")
	})

	t.Run("verify_item_was_not_updated_by_wrong_list_id", func(t *testing.T) {
		// Verify that the malicious update didn't succeed
		fetchedItem, err := store.FindItemByID(ctx, itemID)
		require.NoError(t, err)

		// Title should still be "Updated Title" from the successful update,
		// not "Malicious Update" from the failed cross-list attempt
		assert.Equal(t, "Updated Title", fetchedItem.Title)
		assert.Equal(t, domain.TaskStatusInProgress, fetchedItem.Status)
	})

	t.Run("update_with_nonexistent_list_id_fails", func(t *testing.T) {
		// Attempt to update with a non-existent list_id
		fakeListUUID, err := uuid.NewV7()
		require.NoError(t, err)
		fakeListID := fakeListUUID.String()

		maliciousUpdate := &domain.TodoItem{
			ID:     itemID,
			Title:  "Another Malicious Update",
			Status: domain.TaskStatusDone,
		}

		_, err = todoService.UpdateItem(ctx, ItemToUpdateParams(fakeListID, maliciousUpdate)) // Non-existent list
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestUpdateItem_RepositoryLayer_ValidatesListOwnership tests the
// repository layer directly to ensure the SQL validation works correctly.
func TestUpdateItem_RepositoryLayer_ValidatesListOwnership(t *testing.T) {
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

	// Create two lists
	list1UUID, err := uuid.NewV7()
	require.NoError(t, err)
	list1ID := list1UUID.String()

	list2UUID, err := uuid.NewV7()
	require.NoError(t, err)
	list2ID := list2UUID.String()

	list1 := &domain.TodoList{
		ID:         list1ID,
		Title:      "Repository Test List 1",
		CreateTime: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list1)
	require.NoError(t, err)

	list2 := &domain.TodoList{
		ID:         list2ID,
		Title:      "Repository Test List 2",
		CreateTime: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list2)
	require.NoError(t, err)

	// Create an item in list1
	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	item := &domain.TodoItem{
		ID:         itemID,
		Title:      "Repository Test Item",
		Status:     domain.TaskStatusTodo,
		CreateTime: time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	_, err = store.CreateItem(ctx, list1ID, item)
	require.NoError(t, err)

	t.Run("repository_update_with_correct_list_succeeds", func(t *testing.T) {
		item.Title = "Updated via Repository"
		item.UpdatedAt = time.Now().UTC()

		_, err := store.UpdateItem(ctx, ItemToUpdateParams(list1ID, item))
		require.NoError(t, err)

		// Verify update succeeded
		fetched, err := store.FindItemByID(ctx, itemID)
		require.NoError(t, err)
		assert.Equal(t, "Updated via Repository", fetched.Title)
	})

	t.Run("repository_update_with_wrong_list_fails", func(t *testing.T) {
		item.Title = "Malicious Repository Update"
		item.UpdatedAt = time.Now().UTC()

		// Try to update with wrong list_id
		_, err := store.UpdateItem(ctx, ItemToUpdateParams(list2ID, item))
		require.Error(t, err)

		// Should be a NotFound error (wrapped by domain error)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("repository_verify_malicious_update_did_not_succeed", func(t *testing.T) {
		fetched, err := store.FindItemByID(ctx, itemID)
		require.NoError(t, err)

		// Title should still be from the successful update
		assert.Equal(t, "Updated via Repository", fetched.Title)
		assert.NotEqual(t, "Malicious Repository Update", fetched.Title)
	})
}

// TestUpdateItem_EmptyListId_ReturnsInvalidArgument verifies that
// UpdateItem rejects requests with empty list_id.
func TestUpdateItem_EmptyListId_ReturnsInvalidArgument(t *testing.T) {
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

	todoService := todo.NewService(store, todo.Config{})

	updateItem := &domain.TodoItem{
		ID:     "some-item-id",
		Title:  "Test",
		Status: domain.TaskStatusTodo,
	}

	_, err = todoService.UpdateItem(ctx, ItemToUpdateParams("", updateItem)) // Empty list_id
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list not found", "Empty list_id should return list not found error")
}
