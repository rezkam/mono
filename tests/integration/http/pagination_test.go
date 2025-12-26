package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/http/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListTasks_DefaultPageSizeFromService verifies that when no page_size is specified
// in the HTTP request, the default comes from the service layer (25), not the HTTP layer.
//
// This test ensures proper separation of concerns: the HTTP layer should NOT have its
// own pagination defaults - all business logic defaults belong in the application layer.
func TestListTasks_DefaultPageSizeFromService(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list
	list, err := ts.TodoService.CreateList(ctx, "Pagination Test List")
	require.NoError(t, err)

	// Create more items than the service default (25) but less than HTTP's current default (50)
	// If HTTP layer is incorrectly applying its own default (50), all 30 items would be returned.
	// If service layer default (25) is correctly used, only 25 items should be returned.
	numItems := 30
	for i := 0; i < numItems; i++ {
		_, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
			Title: fmt.Sprintf("Item %02d", i+1),
		})
		require.NoError(t, err, "failed to create item %d", i+1)
	}

	// Make HTTP request WITHOUT specifying page_size
	// The service layer should apply its default of 25
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/lists/%s/items", list.ID), nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp openapi.ListItemsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Items)

	// CRITICAL ASSERTION: Default page size should be 25 (from todo.DefaultPageSize)
	// NOT 50 (from HTTP layer's getPageSize function)
	assert.Equal(t, todo.DefaultPageSize, len(*resp.Items),
		"expected service layer default page size (%d), but got %d - "+
			"HTTP layer should not have its own pagination defaults",
		todo.DefaultPageSize, len(*resp.Items))

	// Verify there are more items (next page exists)
	assert.NotNil(t, resp.NextPageToken, "expected next page token since we have %d items > page size %d",
		numItems, todo.DefaultPageSize)
}

// TestListTasks_PageSizeExceedingMaxRejected verifies that when a page_size exceeding the
// maximum (100) is requested, OpenAPI validation rejects it with 400 Bad Request.
// This is defense-in-depth: invalid requests are rejected at the API boundary.
func TestListTasks_PageSizeExceedingMaxRejected(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list
	list, err := ts.TodoService.CreateList(ctx, "Max Page Size Test List")
	require.NoError(t, err)

	// Request more than max page size (OpenAPI spec has maximum: 100)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/lists/%s/items?page_size=200", list.ID), nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// OpenAPI validation rejects page_size > 100
	require.Equal(t, http.StatusBadRequest, w.Code,
		"page_size exceeding maximum should be rejected by OpenAPI validation")

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
}

// TestListTasks_ExplicitPageSizeRespected verifies that explicit page_size in request
// is passed through to service layer.
func TestListTasks_ExplicitPageSizeRespected(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list with items
	list, err := ts.TodoService.CreateList(ctx, "Explicit Page Size Test")
	require.NoError(t, err)

	numItems := 20
	for i := 0; i < numItems; i++ {
		_, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
			Title: fmt.Sprintf("Item %02d", i+1),
		})
		require.NoError(t, err)
	}

	// Request specific page size
	requestedSize := 10
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/lists/%s/items?page_size=%d", list.ID, requestedSize), nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp openapi.ListItemsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Items)

	assert.Equal(t, requestedSize, len(*resp.Items),
		"expected requested page size (%d), but got %d",
		requestedSize, len(*resp.Items))
}
