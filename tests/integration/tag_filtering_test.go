package integration_test

import (
	"context"
	"testing"

	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/application/todo"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/rezkam/mono/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTagFiltering verifies that tag filtering works correctly with real database
func TestTagFiltering(t *testing.T) {
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
	createListResp, err := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Tag Test List"})
	require.NoError(t, err)
	listID := createListResp.List.Id

	// Create items with different tags
	item1, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId: listID,
		Title:  "Item 1",
		Tags:   []string{"urgent", "work"},
	})
	require.NoError(t, err)

	item2, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId: listID,
		Title:  "Item 2",
		Tags:   []string{"personal"},
	})
	require.NoError(t, err)

	item3, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId: listID,
		Title:  "Item 3",
		Tags:   []string{"urgent", "personal"},
	})
	require.NoError(t, err)

	item4, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId: listID,
		Title:  "Item 4 (no tags)",
	})
	require.NoError(t, err)
	_ = item4

	// First, verify items were created by getting the list
	getListResp, err := svc.GetList(ctx, &monov1.GetListRequest{Id: listID})
	require.NoError(t, err)
	t.Logf("Total items in list: %d", len(getListResp.List.Items))
	for _, item := range getListResp.List.Items {
		t.Logf("  - %s: tags=%v", item.Title, item.Tags)
	}
	require.Len(t, getListResp.List.Items, 4, "should have 4 items in the list")

	// Test 1: Filter by "urgent" tag
	resp, err := svc.ListTasks(ctx, &monov1.ListTasksRequest{
		Filter: "tags:urgent",
	})
	require.NoError(t, err)
	t.Logf("Items with 'urgent' tag: %d", len(resp.Items))
	assert.Len(t, resp.Items, 2, "should return 2 items with 'urgent' tag")
	// Verify the correct items are returned
	ids := []string{resp.Items[0].Id, resp.Items[1].Id}
	assert.Contains(t, ids, item1.Item.Id)
	assert.Contains(t, ids, item3.Item.Id)

	// Test 2: Filter by "personal" tag
	resp, err = svc.ListTasks(ctx, &monov1.ListTasksRequest{
		Filter: "tags:personal",
	})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 2, "should return 2 items with 'personal' tag")
	ids = []string{resp.Items[0].Id, resp.Items[1].Id}
	assert.Contains(t, ids, item2.Item.Id)
	assert.Contains(t, ids, item3.Item.Id)

	// Test 3: Filter by "work" tag
	resp, err = svc.ListTasks(ctx, &monov1.ListTasksRequest{
		Filter: "tags:work",
	})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 1, "should return 1 item with 'work' tag")
	assert.Equal(t, item1.Item.Id, resp.Items[0].Id)

	// Test 4: Filter by non-existent tag
	resp, err = svc.ListTasks(ctx, &monov1.ListTasksRequest{
		Filter: "tags:nonexistent",
	})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 0, "should return 0 items with 'nonexistent' tag")

	// Test 5: No filter (returns all items)
	resp, err = svc.ListTasks(ctx, &monov1.ListTasksRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 4, "should return all 4 items when no filter is applied")
}
