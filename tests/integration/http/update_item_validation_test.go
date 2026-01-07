package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/http/openapi"
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
	invalidStatus := openapi.ItemStatus("INVALID_STATUS")
	reqBody := openapi.UpdateItemRequest{
		Item: openapi.TodoItem{
			Status: &invalidStatus,
		},
		UpdateMask: []openapi.UpdateItemRequestUpdateMask{"status"},
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
	invalidPriority := openapi.ItemPriority("SUPER_URGENT")
	reqBody := openapi.UpdateItemRequest{
		Item: openapi.TodoItem{
			Priority: &invalidPriority,
		},
		UpdateMask: []openapi.UpdateItemRequestUpdateMask{"priority"},
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
	invalidStatus := openapi.ItemStatus("BAD_STATUS")
	reqBody := openapi.UpdateItemRequest{
		Item: openapi.TodoItem{
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

// TestUpdateItem_EmptyTitleRejected verifies that UpdateItem rejects empty titles
// when sent via field mask, ensuring domain validation is enforced.
func TestUpdateItem_EmptyTitleRejected(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list and item with valid title
	list, err := ts.TodoService.CreateList(ctx, "Title Validation List")
	require.NoError(t, err)

	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title: "Original Valid Title",
	})
	require.NoError(t, err)

	// Attempt to update with empty title via field mask
	emptyTitle := ""
	reqBody := openapi.UpdateItemRequest{
		Item: openapi.TodoItem{
			Title: &emptyTitle,
		},
		UpdateMask: []openapi.UpdateItemRequestUpdateMask{"title"},
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
		"expected 400 for empty title, got %d: %s", w.Code, w.Body.String())

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)

	// Verify the original title is unchanged
	updatedItem, err := ts.TodoService.GetItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "Original Valid Title", updatedItem.Title,
		"title should remain unchanged after rejected update")
}

// TestUpdateItem_InvalidTimezone verifies that UpdateItem returns a 400 validation error
// (not 500 internal server error) when an invalid timezone is provided.
// BUG: Currently returns 500 because timezone validation uses fmt.Errorf instead of domain error.
func TestUpdateItem_InvalidTimezone(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list and item
	list, err := ts.TodoService.CreateList(ctx, "Timezone Validation List")
	require.NoError(t, err)

	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title: "Test Item",
	})
	require.NoError(t, err)

	// Try to update with invalid timezone
	invalidTimezone := "Invalid/Timezone"
	reqBody := openapi.UpdateItemRequest{
		Item: openapi.TodoItem{
			Timezone: &invalidTimezone,
		},
		UpdateMask: []openapi.UpdateItemRequestUpdateMask{"timezone"},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/lists/%s/items/%s", list.ID, item.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Should return 400 Bad Request with validation error (NOT 500)
	assert.Equal(t, http.StatusBadRequest, w.Code,
		"expected 400 for invalid timezone, got %d: %s", w.Code, w.Body.String())

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)

	// Verify error mentions timezone field
	require.NotNil(t, resp.Error.Details)
	require.NotEmpty(t, *resp.Error.Details)
	details := *resp.Error.Details
	assert.Equal(t, "timezone", *details[0].Field)
}

// === Phase 2.2: Additional UpdateItem Field Validation Tests ===

// TestUpdateItem_EmptyTimezone_Normalized verifies empty timezone string is normalized to nil
func TestUpdateItem_EmptyTimezone_Normalized(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list and item with a timezone
	list, err := ts.TodoService.CreateList(ctx, "Timezone Update Test")
	require.NoError(t, err)

	stockholm := "Europe/Stockholm"
	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title:    "Test Item",
		Timezone: &stockholm,
	})
	require.NoError(t, err)
	require.NotNil(t, item.Timezone)
	assert.Equal(t, stockholm, *item.Timezone)

	// Update with empty timezone string - should be normalized to nil
	emptyTimezone := ""
	reqBody := openapi.UpdateItemRequest{
		Item: openapi.TodoItem{
			Timezone: &emptyTimezone,
		},
		UpdateMask: []openapi.UpdateItemRequestUpdateMask{"timezone"},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/lists/%s/items/%s", list.ID, item.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"Empty timezone should be accepted and normalized, got: %s", w.Body.String())

	var resp openapi.UpdateItemResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Item)

	// Verify timezone is now nil (floating time)
	assert.Nil(t, resp.Item.Timezone,
		"Empty timezone should be normalized to nil (floating time)")
}

// TestUpdateItem_Tags_EmptyArrayClearsTags verifies that updating with empty array clears tags
func TestUpdateItem_Tags_EmptyArrayClearsTags(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create item with tags
	list, err := ts.TodoService.CreateList(ctx, "Tags Update Test")
	require.NoError(t, err)

	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title: "Test Item",
		Tags:  []string{"work", "urgent", "important"},
	})
	require.NoError(t, err)
	require.NotNil(t, item.Tags)
	assert.Len(t, item.Tags, 3)

	// Update with empty array - should clear tags
	emptyTags := []string{}
	reqBody := openapi.UpdateItemRequest{
		Item: openapi.TodoItem{
			Tags: &emptyTags,
		},
		UpdateMask: []openapi.UpdateItemRequestUpdateMask{"tags"},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/lists/%s/items/%s", list.ID, item.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"Empty tags array should clear tags, got: %s", w.Body.String())

	var resp openapi.UpdateItemResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Item)
	require.NotNil(t, resp.Item.Tags)

	// Verify tags are cleared (empty array)
	assert.Empty(t, *resp.Item.Tags,
		"Tags should be cleared to empty array")
}

// TestUpdateItem_Tags_PreservesWhenNotInMask verifies tags aren't affected when not in update_mask
func TestUpdateItem_Tags_PreservesWhenNotInMask(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create item with tags
	list, err := ts.TodoService.CreateList(ctx, "Tags Preservation Test")
	require.NoError(t, err)

	originalTags := []string{"work", "important"}
	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title: "Test Item",
		Tags:  originalTags,
	})
	require.NoError(t, err)

	// Update title only (no tags in update_mask)
	newTitle := "Updated Title"
	reqBody := openapi.UpdateItemRequest{
		Item: openapi.TodoItem{
			Title: &newTitle,
		},
		UpdateMask: []openapi.UpdateItemRequestUpdateMask{"title"},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/lists/%s/items/%s", list.ID, item.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp openapi.UpdateItemResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Item)

	// Verify tags are preserved
	require.NotNil(t, resp.Item.Tags)
	assert.ElementsMatch(t, originalTags, *resp.Item.Tags,
		"Tags should be preserved when not in update_mask")
}

// TestUpdateItem_Duration_InvalidFormatRejected verifies invalid duration format is rejected
func TestUpdateItem_Duration_InvalidFormatRejected(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create item
	list, err := ts.TodoService.CreateList(ctx, "Duration Update Test")
	require.NoError(t, err)

	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title: "Test Item",
	})
	require.NoError(t, err)

	// Try to update with invalid duration format
	invalidDuration := "not-a-duration"
	reqBody := openapi.UpdateItemRequest{
		Item: openapi.TodoItem{
			EstimatedDuration: &invalidDuration,
		},
		UpdateMask: []openapi.UpdateItemRequestUpdateMask{"estimated_duration"},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/lists/%s/items/%s", list.ID, item.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code,
		"Invalid duration format should be rejected with 400, got: %s", w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errorObj, ok := resp["error"].(map[string]any)
	require.True(t, ok, "Response should have error object")
	assert.Equal(t, "VALIDATION_ERROR", errorObj["code"],
		"Should return VALIDATION_ERROR for invalid duration")
}

// TestUpdateItem_Duration_ClearingWithNull verifies duration can be cleared by setting to null
func TestUpdateItem_Duration_ClearingWithNull(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create item with estimated_duration
	list, err := ts.TodoService.CreateList(ctx, "Duration Clearing Test")
	require.NoError(t, err)

	duration := 2 * time.Hour
	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title:             "Test Item",
		EstimatedDuration: &duration,
	})
	require.NoError(t, err)
	require.NotNil(t, item.EstimatedDuration)

	// Update with null to clear duration
	reqBody := openapi.UpdateItemRequest{
		Item: openapi.TodoItem{
			EstimatedDuration: nil, // Explicitly null
		},
		UpdateMask: []openapi.UpdateItemRequestUpdateMask{"estimated_duration"},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/lists/%s/items/%s", list.ID, item.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"Clearing duration with null should succeed, got: %s", w.Body.String())

	var resp openapi.UpdateItemResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Item)

	// Verify duration is now nil (cleared)
	assert.Nil(t, resp.Item.EstimatedDuration,
		"Duration should be cleared (nil) after update with null")
}
