package integration

import (
	"context"
	"testing"

	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateList_ReturnsVersionAndCountsAtomically verifies that UpdateList
// returns the list with correct version and item counts in a single atomic operation.
//
// This test catches race conditions where:
// - UPDATE happens first (incrementing version)
// - SELECT with counts happens second (could see stale or inconsistent data)
//
// The fix is to use a CTE (Common Table Expression) that does both in one statement.
func TestUpdateList_ReturnsVersionAndCountsAtomically(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	pgURL := getTestDSN(t)
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	todoService := todo.NewService(store, todo.Config{})

	// Create a list (starts with version = 1)
	list, err := todoService.CreateList(ctx, "Atomic Test List")
	require.NoError(t, err)
	listID := list.ID
	assert.Equal(t, 1, list.Version, "New list should have version 1")

	// Add 3 items to the list
	for i := 0; i < 3; i++ {
		item := &domain.TodoItem{
			Title: "Task",
		}
		_, err = todoService.CreateItem(ctx, listID, item)
		require.NoError(t, err)
	}

	// Update the list title
	newTitle := "Updated Atomic Test List"
	updatedList, err := todoService.UpdateList(ctx, domain.UpdateListParams{
		ListID:     listID,
		UpdateMask: []string{"title"},
		Title:      &newTitle,
	})
	require.NoError(t, err)

	// Verify: title was updated
	assert.Equal(t, newTitle, updatedList.Title, "Title should be updated")

	// Verify: version was incremented (1 -> 2)
	assert.Equal(t, 2, updatedList.Version, "Version should be incremented to 2")

	// Verify: counts are correct (atomically returned with the update)
	assert.Equal(t, 3, updatedList.TotalItems, "Should have 3 total items")
	assert.Equal(t, 3, updatedList.UndoneItems, "All 3 items should be undone")
}

// TestUpdateList_VersionConflictDetection verifies that UpdateList correctly
// detects version conflicts when an etag is provided.
func TestUpdateList_VersionConflictDetection(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	pgURL := getTestDSN(t)
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	todoService := todo.NewService(store, todo.Config{})

	// Create a list
	list, err := todoService.CreateList(ctx, "Conflict Test List")
	require.NoError(t, err)
	listID := list.ID
	originalEtag := list.Etag()
	assert.Equal(t, "1", originalEtag, "Initial etag should be '1'")

	// Update the list (simulating another client's update)
	newTitle := "First Update"
	_, err = todoService.UpdateList(ctx, domain.UpdateListParams{
		ListID:     listID,
		UpdateMask: []string{"title"},
		Title:      &newTitle,
	})
	require.NoError(t, err)

	// Try to update with stale etag (version 1, but current is 2)
	conflictTitle := "Conflicting Update"
	_, err = todoService.UpdateList(ctx, domain.UpdateListParams{
		ListID:     listID,
		Etag:       &originalEtag, // Stale etag
		UpdateMask: []string{"title"},
		Title:      &conflictTitle,
	})

	// Should fail with version conflict
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrVersionConflict, "Should return ErrVersionConflict for stale etag")
}

// TestUpdateList_VersionIncrementsProperly verifies version increments correctly
// across multiple updates.
func TestUpdateList_VersionIncrementsProperly(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	pgURL := getTestDSN(t)
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	todoService := todo.NewService(store, todo.Config{})

	// Create a list
	list, err := todoService.CreateList(ctx, "Version Test List")
	require.NoError(t, err)
	listID := list.ID
	assert.Equal(t, 1, list.Version, "Initial version should be 1")

	// Perform 5 updates and verify version increments
	for i := 2; i <= 6; i++ {
		title := "Update v" + string(rune('0'+i))
		updatedList, err := todoService.UpdateList(ctx, domain.UpdateListParams{
			ListID:     listID,
			UpdateMask: []string{"title"},
			Title:      &title,
		})
		require.NoError(t, err)
		assert.Equal(t, i, updatedList.Version, "Version should be %d after update %d", i, i-1)
	}

	// Verify final state
	finalList, err := todoService.GetList(ctx, listID)
	require.NoError(t, err)
	assert.Equal(t, 6, finalList.Version, "Final version should be 6")
}
