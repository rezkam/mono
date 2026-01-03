package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/rezkam/mono/internal/recurring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateList_PreservesCreatedAt is a regression test ensuring that
// UpdateList does NOT corrupt created_at to zero value.
//
// Bug: Prior implementation included created_at in UPDATE SET clause,
// causing it to be overwritten with Go's zero time when not explicitly set.
func TestUpdateList_PreservesCreatedAt(t *testing.T) {
	db, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	pgURL := getTestDSN(t)
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	// Create a list with known created_at
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()
	originalCreatedAt := time.Now().UTC().Truncate(time.Microsecond)

	list := &domain.TodoList{
		ID:        listID,
		Title:     "Original Title",
		CreatedAt: originalCreatedAt,
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Verify created_at was set correctly in database
	var dbCreatedAt time.Time
	err = db.QueryRowContext(ctx, `
		SELECT created_at FROM todo_lists WHERE id = $1
	`, listID).Scan(&dbCreatedAt)
	require.NoError(t, err)
	require.False(t, dbCreatedAt.IsZero(), "created_at should be set on creation")

	// Update the list title (UpdateList only takes Title, not CreatedAt)
	list.Title = "Updated Title"
	_, err = store.UpdateList(ctx, ListToUpdateParams(list))
	require.NoError(t, err)

	// Verify created_at was NOT modified
	var afterUpdateCreatedAt time.Time
	err = db.QueryRowContext(ctx, `
		SELECT created_at FROM todo_lists WHERE id = $1
	`, listID).Scan(&afterUpdateCreatedAt)
	require.NoError(t, err)

	assert.Equal(t, dbCreatedAt.Unix(), afterUpdateCreatedAt.Unix(),
		"created_at should be immutable after creation - UpdateList must not modify it")
	assert.False(t, afterUpdateCreatedAt.IsZero(),
		"created_at must not be corrupted to zero value")
}

// TestUpdateList_PreservesCreatedAt_MultipleUpdates ensures created_at
// remains stable across multiple list updates.
func TestUpdateList_PreservesCreatedAt_MultipleUpdates(t *testing.T) {
	db, cleanup := SetupTestDB(t)
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
		ID:        listID,
		Title:     "Title v1",
		CreatedAt: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Get original created_at
	var originalCreatedAt time.Time
	err = db.QueryRowContext(ctx, `
		SELECT created_at FROM todo_lists WHERE id = $1
	`, listID).Scan(&originalCreatedAt)
	require.NoError(t, err)

	// Perform multiple updates
	titles := []string{"Title v2", "Title v3", "Title v4"}
	for _, title := range titles {
		list.Title = title
		_, err = store.UpdateList(ctx, ListToUpdateParams(list))
		require.NoError(t, err)
	}

	// Verify created_at unchanged after all updates
	var finalCreatedAt time.Time
	err = db.QueryRowContext(ctx, `
		SELECT created_at FROM todo_lists WHERE id = $1
	`, listID).Scan(&finalCreatedAt)
	require.NoError(t, err)

	assert.Equal(t, originalCreatedAt.Unix(), finalCreatedAt.Unix(),
		"created_at should remain unchanged after multiple updates")
}

// TestUpdateItem_PreservesCreatedAt ensures item created_at is immutable.
func TestUpdateItem_PreservesCreatedAt(t *testing.T) {
	db, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	pgURL := getTestDSN(t)
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	generator := recurring.NewDomainGenerator()
	todoService := todo.NewService(store, generator, todo.Config{})

	// Create list and item
	list, err := todoService.CreateList(ctx, "Test List")
	require.NoError(t, err)
	listID := list.ID

	item := &domain.TodoItem{
		Title: "Original Task",
	}
	createdItem, err := todoService.CreateItem(ctx, listID, item)
	require.NoError(t, err)
	itemID := createdItem.ID

	// Get original created_at
	var originalCreatedAt time.Time
	err = db.QueryRowContext(ctx, `
		SELECT created_at FROM todo_items WHERE id = $1
	`, itemID).Scan(&originalCreatedAt)
	require.NoError(t, err)
	require.False(t, originalCreatedAt.IsZero())

	// Update the item
	existingItem, err := todoService.GetItem(ctx, itemID)
	require.NoError(t, err)
	existingItem.Title = "Updated Task"
	existingItem.Status = domain.TaskStatusInProgress
	_, err = todoService.UpdateItem(ctx, ItemToUpdateParams(listID, existingItem))
	require.NoError(t, err)

	// Verify created_at unchanged
	var afterUpdateCreatedAt time.Time
	err = db.QueryRowContext(ctx, `
		SELECT created_at FROM todo_items WHERE id = $1
	`, itemID).Scan(&afterUpdateCreatedAt)
	require.NoError(t, err)

	assert.Equal(t, originalCreatedAt.Unix(), afterUpdateCreatedAt.Unix(),
		"item created_at should be immutable")
}

// TestListLists_ReturnsCorrectItemCounts verifies list metadata (TotalItems, UndoneItems)
// is accurate after various item operations.
func TestListLists_ReturnsCorrectItemCounts(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	pgURL := getTestDSN(t)
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	generator := recurring.NewDomainGenerator()
	todoService := todo.NewService(store, generator, todo.Config{})

	// Create a list
	list, err := todoService.CreateList(ctx, "Count Test")
	require.NoError(t, err)
	listID := list.ID

	// Add 3 items
	for range 3 {
		item := &domain.TodoItem{
			Title: "Task",
		}
		_, err = todoService.CreateItem(ctx, listID, item)
		require.NoError(t, err)
	}

	// Verify counts via ListLists (which includes TotalItems/UndoneItems)
	result, err := todoService.ListLists(ctx, domain.ListListsParams{})
	require.NoError(t, err)

	var targetList *domain.TodoList
	for _, l := range result.Lists {
		if l.ID == listID {
			targetList = l
			break
		}
	}
	require.NotNil(t, targetList)

	assert.Equal(t, 3, targetList.TotalItems, "Should have 3 total items")
	assert.Equal(t, 3, targetList.UndoneItems, "All 3 items should be undone")

	// Mark one item as DONE
	itemsFilter, err := domain.NewItemsFilter(domain.ItemsFilterInput{})
	require.NoError(t, err)
	itemsResult, err := todoService.ListItems(ctx, domain.ListTasksParams{ListID: &listID, Filter: itemsFilter})
	require.NoError(t, err)
	require.Len(t, itemsResult.Items, 3)

	firstItem := &itemsResult.Items[0]
	firstItem.Status = domain.TaskStatusDone
	_, err = todoService.UpdateItem(ctx, ItemToUpdateParams(listID, firstItem))
	require.NoError(t, err)

	// Verify counts updated correctly
	result, err = todoService.ListLists(ctx, domain.ListListsParams{})
	require.NoError(t, err)

	for _, l := range result.Lists {
		if l.ID == listID {
			targetList = l
			break
		}
	}

	assert.Equal(t, 3, targetList.TotalItems, "Should still have 3 total items")
	assert.Equal(t, 2, targetList.UndoneItems, "Should have 2 undone items after marking one DONE")
}
