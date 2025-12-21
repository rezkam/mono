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
