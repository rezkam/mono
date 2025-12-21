package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/require"
)

// TestCreateItem_InvalidListID verifies that CreateItem returns NotFound
// for non-existent list IDs instead of Internal (FK violation).
// This test follows TDD approach and will FAIL until the implementation is fixed.
func TestCreateItem_InvalidListID(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	todoService := todo.NewService(store, todo.Config{})

	// Try to create an item with a non-existent list_id
	item := &domain.TodoItem{
		Title: "Test Item",
	}
	_, err = todoService.CreateItem(ctx, "00000000-0000-0000-0000-000000000000", item)

	require.Error(t, err, "should return error for non-existent list")

	// Should return NotFound for missing list
	require.True(t, errors.Is(err, domain.ErrListNotFound),
		"should return ErrListNotFound for missing list")
}

// TestCreateItem_ValidList verifies normal operation still works
func TestCreateItem_ValidList(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	todoService := todo.NewService(store, todo.Config{})

	// First create a valid list
	list, err := todoService.CreateList(ctx, "Test List")
	require.NoError(t, err)

	// Now create item in that list - should succeed
	item := &domain.TodoItem{
		Title: "Test Item",
	}
	createdItem, err := todoService.CreateItem(ctx, list.ID, item)

	require.NoError(t, err)
	require.NotNil(t, createdItem)
	require.Equal(t, "Test Item", createdItem.Title)
	require.NotEmpty(t, createdItem.ID, "item should have an ID")
}
