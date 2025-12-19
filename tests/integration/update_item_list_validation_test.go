package integration_test

import (
	"context"
	"database/sql"
	"os"
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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestUpdateItem_ValidatesListOwnership verifies that UpdateItem prevents
// cross-list updates by validating the item belongs to the specified list.
//
// SECURITY ISSUE:
// Previously, UpdateItem only checked item.id and ignored list_id.
// A malicious user could update any item if they knew/guessed its ID,
// even if it belonged to a different list.
//
// FIX:
// The SQL UPDATE now includes `WHERE id = $1 AND list_id = $2`, ensuring
// that the update only succeeds if the item belongs to the specified list.
func TestUpdateItem_ValidatesListOwnership(t *testing.T) {
	pgURL := os.Getenv("TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("TEST_POSTGRES_URL not set, skipping PostgreSQL tests")
	}

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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create two separate lists
	list1UUID, err := uuid.NewV7()
	require.NoError(t, err)
	list1ID := list1UUID.String()

	list2UUID, err := uuid.NewV7()
	require.NoError(t, err)
	list2ID := list2UUID.String()

	list1 := &domain.TodoList{
		ID:         list1ID,
		Title:      "List 1",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list1)
	require.NoError(t, err)

	list2 := &domain.TodoList{
		ID:         list2ID,
		Title:      "List 2",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list2)
	require.NoError(t, err)

	// Create an item in list1
	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	item := &domain.TodoItem{
		ID:         itemID,
		Title:      "Original Title",
		Status:     domain.TaskStatusTodo,
		CreateTime: time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	err = store.CreateItem(ctx, list1ID, item)
	require.NoError(t, err)

	t.Run("update_with_correct_list_id_succeeds", func(t *testing.T) {
		// Update the item with the correct list_id (list1)
		req := &monov1.UpdateItemRequest{
			ListId: list1ID,
			Item: &monov1.TodoItem{
				Id:     itemID,
				Title:  "Updated Title",
				Status: monov1.TaskStatus_TASK_STATUS_IN_PROGRESS,
			},
		}

		resp, err := svc.UpdateItem(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, "Updated Title", resp.Item.Title)
		assert.Equal(t, monov1.TaskStatus_TASK_STATUS_IN_PROGRESS, resp.Item.Status)
	})

	t.Run("update_with_wrong_list_id_fails", func(t *testing.T) {
		// Attempt to update the item with the wrong list_id (list2)
		// This should fail even though the item exists - it's in list1, not list2
		req := &monov1.UpdateItemRequest{
			ListId: list2ID, // WRONG: item belongs to list1
			Item: &monov1.TodoItem{
				Id:     itemID,
				Title:  "Malicious Update",
				Status: monov1.TaskStatus_TASK_STATUS_DONE,
			},
		}

		_, err := svc.UpdateItem(ctx, req)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok, "Error should be a gRPC status error")
		assert.Equal(t, codes.NotFound, st.Code(),
			"Cross-list update should return NotFound")
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

		req := &monov1.UpdateItemRequest{
			ListId: fakeListID, // Non-existent list
			Item: &monov1.TodoItem{
				Id:     itemID,
				Title:  "Another Malicious Update",
				Status: monov1.TaskStatus_TASK_STATUS_DONE,
			},
		}

		_, err = svc.UpdateItem(ctx, req)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
	})
}

// TestUpdateItem_RepositoryLayer_ValidatesListOwnership tests the
// repository layer directly to ensure the SQL validation works correctly.
func TestUpdateItem_RepositoryLayer_ValidatesListOwnership(t *testing.T) {
	pgURL := os.Getenv("TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("TEST_POSTGRES_URL not set, skipping PostgreSQL tests")
	}

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
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list1)
	require.NoError(t, err)

	list2 := &domain.TodoList{
		ID:         list2ID,
		Title:      "Repository Test List 2",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list2)
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
	err = store.CreateItem(ctx, list1ID, item)
	require.NoError(t, err)

	t.Run("repository_update_with_correct_list_succeeds", func(t *testing.T) {
		item.Title = "Updated via Repository"
		item.UpdatedAt = time.Now().UTC()

		err := store.UpdateItem(ctx, list1ID, item)
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
		err := store.UpdateItem(ctx, list2ID, item)
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
	pgURL := os.Getenv("TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("TEST_POSTGRES_URL not set, skipping PostgreSQL tests")
	}

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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	req := &monov1.UpdateItemRequest{
		ListId: "", // Empty list_id
		Item: &monov1.TodoItem{
			Id:     "some-item-id",
			Title:  "Test",
			Status: monov1.TaskStatus_TASK_STATUS_TODO,
		},
	}

	_, err = svc.UpdateItem(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code(),
		"Empty list_id should return InvalidArgument")
}
