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

	// First, verify items were created by getting the list items
	allItemsFilter, err := domain.NewItemsFilter(domain.ItemsFilterInput{})
	require.NoError(t, err)
	allItems, err := todoService.ListItems(ctx, domain.ListTasksParams{ListID: &listID, Filter: allItemsFilter})
	require.NoError(t, err)
	t.Logf("Total items in list: %d", len(allItems.Items))
	for _, item := range allItems.Items {
		t.Logf("  - %s: tags=%v", item.Title, item.Tags)
	}
	require.Len(t, allItems.Items, 4, "should have 4 items in the list")

	// Test 1: Filter by "urgent" tag
	filter1, err := domain.NewItemsFilter(domain.ItemsFilterInput{
		Tags: []string{"urgent"},
	})
	require.NoError(t, err)
	result, err := todoService.ListItems(ctx, domain.ListTasksParams{Filter: filter1})
	require.NoError(t, err)
	t.Logf("Items with 'urgent' tag: %d", len(result.Items))
	assert.Len(t, result.Items, 2, "should return 2 items with 'urgent' tag")
	// Verify the correct items are returned
	ids := []string{result.Items[0].ID, result.Items[1].ID}
	assert.Contains(t, ids, item1.ID)
	assert.Contains(t, ids, item3.ID)

	// Test 2: Filter by "personal" tag
	filter2, err := domain.NewItemsFilter(domain.ItemsFilterInput{
		Tags: []string{"personal"},
	})
	require.NoError(t, err)
	result, err = todoService.ListItems(ctx, domain.ListTasksParams{Filter: filter2})
	require.NoError(t, err)
	assert.Len(t, result.Items, 2, "should return 2 items with 'personal' tag")
	ids = []string{result.Items[0].ID, result.Items[1].ID}
	assert.Contains(t, ids, item2.ID)
	assert.Contains(t, ids, item3.ID)

	// Test 3: Filter by "work" tag
	filter3, err := domain.NewItemsFilter(domain.ItemsFilterInput{
		Tags: []string{"work"},
	})
	require.NoError(t, err)
	result, err = todoService.ListItems(ctx, domain.ListTasksParams{Filter: filter3})
	require.NoError(t, err)
	assert.Len(t, result.Items, 1, "should return 1 item with 'work' tag")
	assert.Equal(t, item1.ID, result.Items[0].ID)

	// Test 4: Filter by non-existent tag
	filter4, err := domain.NewItemsFilter(domain.ItemsFilterInput{
		Tags: []string{"nonexistent"},
	})
	require.NoError(t, err)
	result, err = todoService.ListItems(ctx, domain.ListTasksParams{Filter: filter4})
	require.NoError(t, err)
	assert.Len(t, result.Items, 0, "should return 0 items with 'nonexistent' tag")

	// Test 5: No filter (returns all items excluding archived/cancelled)
	filter5, err := domain.NewItemsFilter(domain.ItemsFilterInput{})
	require.NoError(t, err)
	result, err = todoService.ListItems(ctx, domain.ListTasksParams{Filter: filter5})
	require.NoError(t, err)
	assert.Len(t, result.Items, 4, "should return all 4 items when no filter is applied")

	// Test 6: Multiple tags with AND logic
	filter6, err := domain.NewItemsFilter(domain.ItemsFilterInput{
		Tags: []string{"urgent", "personal"},
	})
	require.NoError(t, err)
	result, err = todoService.ListItems(ctx, domain.ListTasksParams{Filter: filter6})
	require.NoError(t, err)
	assert.Len(t, result.Items, 1, "should return 1 item with BOTH 'urgent' AND 'personal' tags")
	assert.Equal(t, item3.ID, result.Items[0].ID, "should return item3 which has both tags")

	// Test 7: Multiple tags where no item has all
	filter7, err := domain.NewItemsFilter(domain.ItemsFilterInput{
		Tags: []string{"work", "personal"},
	})
	require.NoError(t, err)
	result, err = todoService.ListItems(ctx, domain.ListTasksParams{Filter: filter7})
	require.NoError(t, err)
	assert.Len(t, result.Items, 0, "should return 0 items as no item has BOTH 'work' AND 'personal' tags")
}
