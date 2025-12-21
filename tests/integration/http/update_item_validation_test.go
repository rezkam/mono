package http_test

import (
	"bytes"
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

// TestUpdateItem_InvalidStatus verifies that UpdateItem returns a validation error
// when an invalid status is provided.
func TestUpdateItem_InvalidStatus(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list and item
	list, err := ts.TodoService.CreateList(ctx, "Validation Test List")
	require.NoError(t, err)

	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title: "Test Item",
	})
	require.NoError(t, err)

	// Try to update with invalid status
	invalidStatus := openapi.TaskStatus("INVALID_STATUS")
	updateMask := []string{"status"}
	reqBody := openapi.UpdateItemRequest{
		Item: &openapi.TodoItem{
			Status: &invalidStatus,
		},
		UpdateMask: &updateMask,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/lists/%s/items/%s", list.ID, item.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Should return 400 Bad Request with validation error
	assert.Equal(t, http.StatusBadRequest, w.Code,
		"expected 400 for invalid status, got %d: %s", w.Code, w.Body.String())

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
}

// TestUpdateItem_InvalidPriority verifies that UpdateItem returns a validation error
// when an invalid priority is provided.
func TestUpdateItem_InvalidPriority(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list and item
	list, err := ts.TodoService.CreateList(ctx, "Priority Validation List")
	require.NoError(t, err)

	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title: "Test Item",
	})
	require.NoError(t, err)

	// Try to update with invalid priority
	invalidPriority := openapi.TaskPriority("SUPER_URGENT")
	updateMask := []string{"priority"}
	reqBody := openapi.UpdateItemRequest{
		Item: &openapi.TodoItem{
			Priority: &invalidPriority,
		},
		UpdateMask: &updateMask,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/lists/%s/items/%s", list.ID, item.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Should return 400 Bad Request with validation error
	assert.Equal(t, http.StatusBadRequest, w.Code,
		"expected 400 for invalid priority, got %d: %s", w.Code, w.Body.String())

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
}

// TestUpdateItem_InvalidStatusWithoutMask verifies validation works when no update_mask is provided.
func TestUpdateItem_InvalidStatusWithoutMask(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	list, err := ts.TodoService.CreateList(ctx, "No Mask Validation List")
	require.NoError(t, err)

	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title: "Test Item",
	})
	require.NoError(t, err)

	// Try to update with invalid status (no update_mask = update all provided fields)
	invalidStatus := openapi.TaskStatus("BAD_STATUS")
	reqBody := openapi.UpdateItemRequest{
		Item: &openapi.TodoItem{
			Status: &invalidStatus,
		},
		// No UpdateMask - should still validate
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/lists/%s/items/%s", list.ID, item.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code,
		"expected 400 for invalid status without mask, got %d: %s", w.Code, w.Body.String())
}
