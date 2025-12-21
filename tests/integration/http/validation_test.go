package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/http/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// === Empty/Null Required Fields Tests ===

// TestValidation_EmptyTitle_CreateList verifies that creating a list with empty title fails.
func TestValidation_EmptyTitle_CreateList(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	reqBody := openapi.CreateListRequest{
		Title: "", // Empty title
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/lists", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
}

// TestValidation_EmptyTitle_CreateItem verifies that creating an item with empty title fails.
func TestValidation_EmptyTitle_CreateItem(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list first
	list, err := ts.TodoService.CreateList(ctx, "Test List")
	require.NoError(t, err)

	reqBody := openapi.CreateItemRequest{
		Title: "", // Empty title
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", list.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
}

// TestValidation_InvalidListID_CreateItem verifies that invalid list ID format returns error.
func TestValidation_InvalidListID_CreateItem(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	reqBody := openapi.CreateItemRequest{
		Title: "Valid Title",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	// Use invalid UUID format
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lists/not-a-valid-uuid/items", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Should return 400 for invalid UUID format
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestValidation_NonExistentList_CreateItem verifies that creating item on non-existent list fails.
func TestValidation_NonExistentList_CreateItem(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	reqBody := openapi.CreateItemRequest{
		Title: "Valid Title",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	// Use valid UUID format but non-existent list
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lists/01912345-6789-7abc-def0-123456789abc/items", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "NOT_FOUND", *resp.Error.Code)
}

// === Invalid Status/Priority Tests ===

// TestValidation_InvalidStatus_UpdateItem verifies that invalid status values are rejected.
func TestValidation_InvalidStatus_UpdateItem(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create list and item
	list, err := ts.TodoService.CreateList(ctx, "Status Test List")
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

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
}

// TestValidation_InvalidPriority_UpdateItem verifies that invalid priority values are rejected.
func TestValidation_InvalidPriority_UpdateItem(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create list and item
	list, err := ts.TodoService.CreateList(ctx, "Priority Test List")
	require.NoError(t, err)

	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title: "Test Item",
	})
	require.NoError(t, err)

	// Try to update with invalid priority
	invalidPriority := openapi.TaskPriority("SUPER_HIGH")
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

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
}

// === Valid Large Data Tests ===

// TestValidation_LargeTitle verifies that large (but valid) titles are accepted.
func TestValidation_LargeTitle(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list first
	list, err := ts.TodoService.CreateList(ctx, "Large Title Test List")
	require.NoError(t, err)

	// Create item with 255 character title (max allowed)
	largeTitle := strings.Repeat("A", 255)
	reqBody := openapi.CreateItemRequest{
		Title: largeTitle,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", list.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp openapi.CreateItemResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Item)
	require.NotNil(t, resp.Item.Title)
	assert.Equal(t, 255, len(*resp.Item.Title))
}

// TestValidation_TitleTooLong verifies that titles exceeding 255 characters are rejected.
func TestValidation_TitleTooLong(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list first
	list, err := ts.TodoService.CreateList(ctx, "Title Too Long Test List")
	require.NoError(t, err)

	// Create item with 256 character title (one over max)
	tooLongTitle := strings.Repeat("A", 256)
	reqBody := openapi.CreateItemRequest{
		Title: tooLongTitle,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", list.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
}

// TestValidation_ManyTags verifies that many tags are handled correctly.
func TestValidation_ManyTags(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list first
	list, err := ts.TodoService.CreateList(ctx, "Many Tags Test List")
	require.NoError(t, err)

	// Create 100 tags
	manyTags := make([]string, 100)
	for i := 0; i < 100; i++ {
		manyTags[i] = fmt.Sprintf("tag-%03d", i)
	}

	reqBody := openapi.CreateItemRequest{
		Title: "Item with many tags",
		Tags:  &manyTags,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", list.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp openapi.CreateItemResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Item)
	require.NotNil(t, resp.Item.Tags)
	assert.Len(t, *resp.Item.Tags, 100)
}

// === Invalid JSON Tests ===

// TestValidation_MalformedJSON_CreateList verifies that malformed JSON is rejected.
func TestValidation_MalformedJSON_CreateList(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	testCases := []struct {
		name string
		body string
	}{
		{"trailing_comma", `{"title": "Test",}`},
		{"unclosed_brace", `{"title": "Test"`},
		{"single_quotes", `{'title': 'Test'}`},
		{"not_json", `this is not json at all`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/lists", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+ts.APIKey)

			w := httptest.NewRecorder()
			ts.Router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code, "Malformed JSON should return 400")

			var resp openapi.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.NotNil(t, resp.Error)
			require.NotNil(t, resp.Error.Code)
			assert.Equal(t, "INVALID_REQUEST", *resp.Error.Code)
		})
	}
}

// TestValidation_MalformedJSON_CreateItem verifies that malformed JSON is rejected for items.
func TestValidation_MalformedJSON_CreateItem(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list first
	list, err := ts.TodoService.CreateList(ctx, "JSON Test List")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", list.ID), strings.NewReader(`{"title": "Test",}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "INVALID_REQUEST", *resp.Error.Code)
}

// === Authentication Tests ===

// TestValidation_MissingAuth verifies that missing authentication returns 401.
func TestValidation_MissingAuth(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lists", nil)
	// No Authorization header

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestValidation_InvalidAuth verifies that invalid API key returns 401.
func TestValidation_InvalidAuth(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lists", nil)
	req.Header.Set("Authorization", "Bearer invalid-api-key")

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
