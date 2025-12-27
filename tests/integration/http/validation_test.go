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
	"github.com/rezkam/mono/internal/infrastructure/http/openapi"
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
	invalidPriority := openapi.ItemPriority("SUPER_HIGH")
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

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
}

// === Update Mask With Missing Value Tests ===
// These tests verify that when update_mask includes a required field but no value
// is provided, the API returns 400 Bad Request instead of bubbling up as 500.

// TestValidation_UpdateMaskStatusNoValue verifies that including "status" in
// update_mask without providing a status value returns 400, not 500.
// Bug: Previously, this caused a NOT NULL constraint violation at the DB layer,
// returning 500 Internal Server Error. The fix validates at the service layer.
func TestValidation_UpdateMaskStatusNoValue(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create list and item
	list, err := ts.TodoService.CreateList(ctx, "Update Mask Test List")
	require.NoError(t, err)

	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title: "Test Item",
	})
	require.NoError(t, err)

	// Send update with "status" in mask but no status value
	reqBody := openapi.UpdateItemRequest{
		Item:       openapi.TodoItem{}, // No status value!
		UpdateMask: []openapi.UpdateItemRequestUpdateMask{"status"},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/lists/%s/items/%s", list.ID, item.ID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Should return 400 Bad Request, NOT 500 Internal Server Error
	require.Equal(t, http.StatusBadRequest, w.Code,
		"update_mask includes 'status' but no value provided - should return 400, not 500.\n"+
			"Response body: %s", w.Body.String())

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Details, "error should have details")
	require.Len(t, *resp.Error.Details, 1, "should have one validation error")
	detail := (*resp.Error.Details)[0]
	require.NotNil(t, detail.Field)
	assert.Equal(t, "status", *detail.Field, "error should mention the status field")
}

// TestValidation_UpdateMaskPriorityNoValue verifies that including "priority" in
// update_mask without providing a priority value returns 400, not 500.
func TestValidation_UpdateMaskPriorityNoValue(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create list and item with a priority
	list, err := ts.TodoService.CreateList(ctx, "Update Mask Priority Test")
	require.NoError(t, err)

	highPriority := domain.TaskPriorityHigh
	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title:    "Test Item",
		Priority: &highPriority,
	})
	require.NoError(t, err)

	// Send update with "priority" in mask but no priority value
	reqBody := openapi.UpdateItemRequest{
		Item:       openapi.TodoItem{}, // No priority value!
		UpdateMask: []openapi.UpdateItemRequestUpdateMask{"priority"},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/lists/%s/items/%s", list.ID, item.ID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Priority is nullable, so setting it to NULL is valid
	// This should succeed (unlike status which is NOT NULL)
	assert.Equal(t, http.StatusOK, w.Code,
		"priority is nullable - setting to NULL via update_mask should succeed.\n"+
			"Response body: %s", w.Body.String())
}

// TestValidation_UpdateRecurringTemplate_TitleInMaskNoValue verifies that including
// "title" in update_mask without providing a title value returns 400, not a silent no-op.
// Bug: Previously, this would succeed but not update the title - hiding client bugs.
func TestValidation_UpdateRecurringTemplate_TitleInMaskNoValue(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create list and template
	list, err := ts.TodoService.CreateList(ctx, "Template Update Test List")
	require.NoError(t, err)

	template, err := ts.TodoService.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
		ListID:            list.ID,
		Title:             "Original Title",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  make(map[string]interface{}),
	})
	require.NoError(t, err)

	// Send update with "title" in mask but no title value
	reqBody := openapi.UpdateRecurringTemplateRequest{
		Template:   openapi.RecurringItemTemplate{}, // No title value!
		UpdateMask: []openapi.UpdateRecurringTemplateRequestUpdateMask{"title"},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", list.ID, template.ID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Should return 400 Bad Request, NOT 200 with silent no-op
	require.Equal(t, http.StatusBadRequest, w.Code,
		"update_mask includes 'title' but no value provided - should return 400, not silent no-op.\n"+
			"Response body: %s", w.Body.String())

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Details, "error should have details")
	require.Len(t, *resp.Error.Details, 1, "should have one validation error")
	detail := (*resp.Error.Details)[0]
	require.NotNil(t, detail.Field)
	assert.Equal(t, "title", *detail.Field, "error should mention the title field")
}

// TestValidation_UpdateRecurringTemplate_RecurrencePatternInMaskNoValue verifies that
// including "recurrence_pattern" in update_mask without a value returns 400.
func TestValidation_UpdateRecurringTemplate_RecurrencePatternInMaskNoValue(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create list and template
	list, err := ts.TodoService.CreateList(ctx, "Pattern Update Test List")
	require.NoError(t, err)

	template, err := ts.TodoService.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
		ListID:            list.ID,
		Title:             "Test Template",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  make(map[string]interface{}),
	})
	require.NoError(t, err)

	// Send update with "recurrence_pattern" in mask but no pattern value
	reqBody := openapi.UpdateRecurringTemplateRequest{
		Template:   openapi.RecurringItemTemplate{}, // No recurrence_pattern value!
		UpdateMask: []openapi.UpdateRecurringTemplateRequestUpdateMask{"recurrence_pattern"},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", list.ID, template.ID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Should return 400 Bad Request
	require.Equal(t, http.StatusBadRequest, w.Code,
		"update_mask includes 'recurrence_pattern' but no value provided - should return 400.\n"+
			"Response body: %s", w.Body.String())

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Details, "error should have details")
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
			// OpenAPI middleware catches malformed JSON before handler
			assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
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
	// OpenAPI middleware catches malformed JSON before handler
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
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

// === Required Fields Tests ===

// TestValidation_MissingUpdateMask_UpdateRecurringTemplate verifies that
// updating a recurring template without update_mask returns 400.
func TestValidation_MissingUpdateMask_UpdateRecurringTemplate(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list first
	list, err := ts.TodoService.CreateList(ctx, "Template Validation Test List")
	require.NoError(t, err)

	// Create a template
	template, err := ts.TodoService.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
		ListID:            list.ID,
		Title:             "Test Template",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  make(map[string]interface{}),
	})
	require.NoError(t, err)

	// Send empty request body - missing required update_mask and template
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", list.ID, template.ID), bytes.NewReader([]byte(`{}`)))
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

// TestValidation_MissingUpdateMask_UpdateItem verifies that
// updating an item without update_mask returns 400.
func TestValidation_MissingUpdateMask_UpdateItem(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list and item first
	list, err := ts.TodoService.CreateList(ctx, "Item Validation Test List")
	require.NoError(t, err)

	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title: "Test Item",
	})
	require.NoError(t, err)

	// Send empty request body - missing required update_mask and item
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/lists/%s/items/%s", list.ID, item.ID), bytes.NewReader([]byte(`{}`)))
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

// === Unknown Update Mask Field Tests ===
// These tests verify that OpenAPI validation rejects unknown/invalid field names
// in update_mask via the enum constraint. This catches typos and invalid field names
// at the API boundary, preventing silent no-ops where clients think a field was
// updated but nothing happened.

// TestValidation_UnknownUpdateMaskField_UpdateItem verifies that an unknown field
// in update_mask (e.g., "tittle" typo) returns 400 Bad Request.
func TestValidation_UnknownUpdateMaskField_UpdateItem(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create list and item
	list, err := ts.TodoService.CreateList(ctx, "Unknown Mask Field Test")
	require.NoError(t, err)

	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title: "Test Item",
	})
	require.NoError(t, err)

	// Send update with typo in update_mask ("tittle" instead of "title")
	// We use raw JSON because the generated types only allow valid enum values
	reqBody := `{
		"item": {"title": "Updated Title"},
		"update_mask": ["tittle"]
	}`

	req := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/lists/%s/items/%s", list.ID, item.ID),
		strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// OpenAPI enum validation should reject unknown field name
	require.Equal(t, http.StatusBadRequest, w.Code,
		"unknown field 'tittle' in update_mask should return 400, not succeed silently.\n"+
			"Response body: %s", w.Body.String())

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
}

// TestValidation_UnknownUpdateMaskField_UpdateRecurringTemplate verifies that
// an unknown field in update_mask returns 400 Bad Request for recurring templates.
func TestValidation_UnknownUpdateMaskField_UpdateRecurringTemplate(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create list and template
	list, err := ts.TodoService.CreateList(ctx, "Unknown Mask Template Test")
	require.NoError(t, err)

	template, err := ts.TodoService.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
		ListID:            list.ID,
		Title:             "Test Template",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  make(map[string]interface{}),
	})
	require.NoError(t, err)

	// Send update with unknown field in update_mask
	reqBody := `{
		"template": {"title": "Updated Title"},
		"update_mask": ["unknown_field"]
	}`

	req := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", list.ID, template.ID),
		strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// OpenAPI enum validation should reject unknown field name
	require.Equal(t, http.StatusBadRequest, w.Code,
		"unknown field 'unknown_field' in update_mask should return 400.\n"+
			"Response body: %s", w.Body.String())

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
}
