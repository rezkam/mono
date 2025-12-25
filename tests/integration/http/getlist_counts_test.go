package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/http/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetList_ReturnsTotalAndUndoneItems verifies that GetList returns
// correct total_items and undone_items counts.
func TestGetList_ReturnsTotalAndUndoneItems(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list
	list, err := ts.TodoService.CreateList(ctx, "Count Test List")
	require.NoError(t, err)

	// Add 3 items (all will be "todo" status initially)
	for i := 0; i < 3; i++ {
		_, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
			Title: fmt.Sprintf("Task %d", i+1),
		})
		require.NoError(t, err)
	}

	// Get the list via HTTP
	req := httptest.NewRequest(http.MethodGet, "/api/v1/lists/"+list.ID, nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Verify status
	require.Equal(t, http.StatusOK, w.Code)

	// Parse response
	var resp openapi.GetListResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err, "Failed to parse response: %s", w.Body.String())

	// Verify counts
	require.NotNil(t, resp.List)
	require.NotNil(t, resp.List.TotalItems, "TotalItems should not be nil")
	require.NotNil(t, resp.List.UndoneItems, "UndoneItems should not be nil")

	assert.Equal(t, 3, *resp.List.TotalItems, "Should have 3 total items")
	assert.Equal(t, 3, *resp.List.UndoneItems, "All 3 items should be undone")

	t.Logf("Raw JSON response: %s", w.Body.String())
}

// TestListItems_ExcludesArchivedAndCancelledByDefault verifies that ListItems
// excludes archived and cancelled items by default.
func TestListItems_ExcludesArchivedAndCancelledByDefault(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list
	list, err := ts.TodoService.CreateList(ctx, "Status Filter Test List")
	require.NoError(t, err)

	// Add items with all active statuses (should be included)
	todoItem, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title:  "Todo Task",
		Status: "todo",
	})
	require.NoError(t, err)

	inProgressItem, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title:  "In Progress Task",
		Status: "in_progress",
	})
	require.NoError(t, err)

	blockedItem, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title:  "Blocked Task",
		Status: "blocked",
	})
	require.NoError(t, err)

	doneItem, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title:  "Done Task",
		Status: "done",
	})
	require.NoError(t, err)

	// Add archived item (should be excluded)
	archivedItem, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title:  "Archived Task",
		Status: "archived",
	})
	require.NoError(t, err)

	// Add cancelled item (should be excluded)
	cancelledItem, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title:  "Cancelled Task",
		Status: "cancelled",
	})
	require.NoError(t, err)

	// Get items via HTTP using the new endpoint (without any query parameters)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/lists/"+list.ID+"/items", nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Verify status
	require.Equal(t, http.StatusOK, w.Code)

	// Parse response
	var resp openapi.ListItemsResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err, "Failed to parse response: %s", w.Body.String())

	// Verify response structure
	require.NotNil(t, resp.Items)

	// Verify that only active items are returned (4 items: todo, in_progress, blocked, done)
	assert.Len(t, *resp.Items, 4, "Should return only 4 active items (todo, in_progress, blocked, done)")

	// Verify that the returned items are the correct ones
	returnedIDs := make(map[string]bool)
	for _, item := range *resp.Items {
		require.NotNil(t, item.Id)
		returnedIDs[item.Id.String()] = true
		// Verify none of the returned items are archived or cancelled
		require.NotNil(t, item.Status)
		assert.NotEqual(t, "archived", string(*item.Status), "Should not return archived items")
		assert.NotEqual(t, "cancelled", string(*item.Status), "Should not return cancelled items")
	}

	// Verify all active items are present
	assert.True(t, returnedIDs[todoItem.ID], "Todo item should be in response")
	assert.True(t, returnedIDs[inProgressItem.ID], "In progress item should be in response")
	assert.True(t, returnedIDs[blockedItem.ID], "Blocked item should be in response")
	assert.True(t, returnedIDs[doneItem.ID], "Done item should be in response")

	// Verify archived and cancelled items are NOT present
	assert.False(t, returnedIDs[archivedItem.ID], "Archived item should NOT be in response")
	assert.False(t, returnedIDs[cancelledItem.ID], "Cancelled item should NOT be in response")

	t.Logf("Raw JSON response: %s", w.Body.String())
}
