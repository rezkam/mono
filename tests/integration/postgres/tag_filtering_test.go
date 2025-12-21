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

// TestTagFiltering verifies that tag filtering works correctly with real database
func TestTagFiltering(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	todoService := todo.NewService(store, todo.Config{})

	// Create a list
	list, err := todoService.CreateList(ctx, "Tag Test List")
	require.NoError(t, err)
	listID := list.ID

	// Create items with different tags
	item1, err := todoService.CreateItem(ctx, listID, &domain.TodoItem{
		Title: "Item 1",
		Tags:  []string{"urgent", "work"},
	})
	require.NoError(t, err)

	item2, err := todoService.CreateItem(ctx, listID, &domain.TodoItem{
		Title: "Item 2",
		Tags:  []string{"personal"},
	})
	require.NoError(t, err)

	item3, err := todoService.CreateItem(ctx, listID, &domain.TodoItem{
		Title: "Item 3",
		Tags:  []string{"urgent", "personal"},
	})
	require.NoError(t, err)

	item4, err := todoService.CreateItem(ctx, listID, &domain.TodoItem{
		Title: "Item 4 (no tags)",
	})
	require.NoError(t, err)
	_ = item4

	// First, verify items were created by getting the list
	fetchedList, err := todoService.GetList(ctx, listID)
	require.NoError(t, err)
	t.Logf("Total items in list: %d", len(fetchedList.Items))
	for _, item := range fetchedList.Items {
		t.Logf("  - %s: tags=%v", item.Title, item.Tags)
	}
	require.Len(t, fetchedList.Items, 4, "should have 4 items in the list")

	// Test 1: Filter by "urgent" tag
	tag := "urgent"
	result, err := todoService.ListTasks(ctx, domain.ListTasksParams{Tag: &tag})
	require.NoError(t, err)
	t.Logf("Items with 'urgent' tag: %d", len(result.Items))
	assert.Len(t, result.Items, 2, "should return 2 items with 'urgent' tag")
	// Verify the correct items are returned
	ids := []string{result.Items[0].ID, result.Items[1].ID}
	assert.Contains(t, ids, item1.ID)
	assert.Contains(t, ids, item3.ID)

	// Test 2: Filter by "personal" tag
	tag = "personal"
	result, err = todoService.ListTasks(ctx, domain.ListTasksParams{Tag: &tag})
	require.NoError(t, err)
	assert.Len(t, result.Items, 2, "should return 2 items with 'personal' tag")
	ids = []string{result.Items[0].ID, result.Items[1].ID}
	assert.Contains(t, ids, item2.ID)
	assert.Contains(t, ids, item3.ID)

	// Test 3: Filter by "work" tag
	tag = "work"
	result, err = todoService.ListTasks(ctx, domain.ListTasksParams{Tag: &tag})
	require.NoError(t, err)
	assert.Len(t, result.Items, 1, "should return 1 item with 'work' tag")
	assert.Equal(t, item1.ID, result.Items[0].ID)

	// Test 4: Filter by non-existent tag
	tag = "nonexistent"
	result, err = todoService.ListTasks(ctx, domain.ListTasksParams{Tag: &tag})
	require.NoError(t, err)
	assert.Len(t, result.Items, 0, "should return 0 items with 'nonexistent' tag")

	// Test 5: No filter (returns all items)
	result, err = todoService.ListTasks(ctx, domain.ListTasksParams{})
	require.NoError(t, err)
	assert.Len(t, result.Items, 4, "should return all 4 items when no filter is applied")
}
