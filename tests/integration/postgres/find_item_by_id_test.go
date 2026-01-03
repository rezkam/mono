package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFindItemByID_Success tests O(1) item lookup by ID.
func TestFindItemByID_Success(t *testing.T) {
	_, cleanup := SetupTestDB(t)
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
		Title:     "FindItemByID Test",
		CreatedAt: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create multiple items
	var targetItemID string
	for i := range 10 {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)

		priority := domain.TaskPriorityMedium
		item := &domain.TodoItem{
			ID:        itemUUID.String(),
			Title:     "Task " + string(rune('A'+i)),
			Status:    domain.TaskStatusTodo,
			Priority:  &priority,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		_, err = store.CreateItem(ctx, listID, item)
		require.NoError(t, err)

		// Mark item 5 as the target
		if i == 5 {
			targetItemID = item.ID
		}
	}

	// Test O(1) lookup - should find item directly without loading list
	item, err := store.FindItemByID(ctx, targetItemID)
	require.NoError(t, err)
	require.NotNil(t, item)

	assert.Equal(t, targetItemID, item.ID)
	assert.Equal(t, "Task F", item.Title)
	assert.Equal(t, domain.TaskStatusTodo, item.Status)
	assert.NotNil(t, item.Priority)
	assert.Equal(t, domain.TaskPriorityMedium, *item.Priority)
}

// TestFindItemByID_NotFound tests that non-existent item returns proper error.
func TestFindItemByID_NotFound(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	// Try to find an item that doesn't exist
	nonExistentID := "019b0000-0000-7000-8000-000000000000"
	item, err := store.FindItemByID(ctx, nonExistentID)

	assert.Nil(t, item)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrItemNotFound),
		"expected ErrItemNotFound, got: %v", err)
}

// TestFindItemByID_InvalidID tests that invalid UUID format returns proper error.
func TestFindItemByID_InvalidID(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	// Try to find with invalid UUID
	item, err := store.FindItemByID(ctx, "not-a-valid-uuid")

	assert.Nil(t, item)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidID),
		"expected ErrInvalidID, got: %v", err)
}

// TestFindItemByID_ReturnsAllFields tests that all item fields are returned correctly.
func TestFindItemByID_ReturnsAllFields(t *testing.T) {
	_, cleanup := SetupTestDB(t)
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
		Title:     "Full Fields Test",
		CreatedAt: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create an item with all fields populated
	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)

	priority := domain.TaskPriorityHigh
	dueTime := time.Now().UTC().Add(24 * time.Hour)
	estimatedDuration := 2 * time.Hour
	timezone := "America/New_York"

	item := &domain.TodoItem{
		ID:                itemUUID.String(),
		Title:             "Full Item",
		Status:            domain.TaskStatusInProgress,
		Priority:          &priority,
		DueAt:             &dueTime,
		EstimatedDuration: &estimatedDuration,
		Tags:              []string{"urgent", "work"},
		Timezone:          &timezone,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	_, err = store.CreateItem(ctx, listID, item)
	require.NoError(t, err)

	// Retrieve and verify all fields
	retrieved, err := store.FindItemByID(ctx, item.ID)
	require.NoError(t, err)

	assert.Equal(t, item.ID, retrieved.ID)
	assert.Equal(t, listID, retrieved.ListID)
	assert.Equal(t, "Full Item", retrieved.Title)
	assert.Equal(t, domain.TaskStatusInProgress, retrieved.Status)

	require.NotNil(t, retrieved.Priority)
	assert.Equal(t, domain.TaskPriorityHigh, *retrieved.Priority)

	require.NotNil(t, retrieved.DueAt)
	assert.WithinDuration(t, dueTime, *retrieved.DueAt, time.Second)

	require.NotNil(t, retrieved.EstimatedDuration)
	assert.Equal(t, estimatedDuration, *retrieved.EstimatedDuration)

	assert.ElementsMatch(t, []string{"urgent", "work"}, retrieved.Tags)

	require.NotNil(t, retrieved.Timezone)
	assert.Equal(t, "America/New_York", *retrieved.Timezone)
}
