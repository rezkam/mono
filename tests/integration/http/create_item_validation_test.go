package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/infrastructure/http/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// === Phase 1.1: CreateItem Status/Priority Validation Tests ===

// TestCreateItem_InvalidStatus_Rejected verifies that CreateItem rejects invalid status values.
// This is a critical security test - invalid enum values must be rejected to prevent data corruption.
//
// Expected (RED phase): This test should FAIL initially because CreateItem currently accepts
// any status value and only sets default if empty (service.go:210-212).
func TestCreateItem_InvalidStatus_Rejected(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "Status Test List")
	require.NoError(t, err)

	// Create request with invalid status via HTTP
	reqBody := `{
		"title": "Test Item",
		"status": "INVALID_STATUS"
	}`

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
		strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// EXPECT: 400 Bad Request with VALIDATION_ERROR
	assert.Equal(t, http.StatusBadRequest, w.Code,
		"Invalid status should be rejected, got: %s", w.Body.String())

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
}

// TestCreateItem_InvalidPriority_Rejected verifies that CreateItem rejects invalid priority values.
//
// Expected (RED phase): This test should FAIL initially because priority validation happens at
// handler level but we need to ensure enum validation works correctly.
func TestCreateItem_InvalidPriority_Rejected(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "Priority Test List")
	require.NoError(t, err)

	// Create request with invalid priority
	reqBody := `{
		"title": "Test Item",
		"priority": "SUPER_DUPER_URGENT"
	}`

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
		strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code,
		"Invalid priority should be rejected, got: %s", w.Body.String())

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
}

// TestCreateItem_EmptyPriority_Rejected verifies that empty string priority is rejected.
// Priority is optional (can be nil), but if provided as string it must be valid.
//
// Expected (RED phase): This test should FAIL if empty strings bypass validation.
func TestCreateItem_EmptyPriority_Rejected(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "Empty Priority Test")
	require.NoError(t, err)

	// Create request with empty priority string
	reqBody := `{
		"title": "Test Item",
		"priority": ""
	}`

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
		strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code,
		"Empty priority should be rejected")

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
}

// === Phase 1.2: Empty Timezone Normalization Tests ===

// TestCreateItem_EmptyTimezone_Normalized verifies that empty timezone strings are normalized to nil.
// Empty timezone should mean "not set" (floating time), not "empty string" stored in database.
//
// Expected (RED phase): This test should FAIL if empty string is stored instead of nil.
func TestCreateItem_EmptyTimezone_Normalized(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "Timezone Test")
	require.NoError(t, err)

	reqBody := `{
		"title": "Test Item",
		"timezone": ""
	}`

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
		strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Empty timezone should be accepted and normalized to nil (not rejected)
	assert.Equal(t, http.StatusCreated, w.Code,
		"Empty timezone should be accepted and normalized, got: %s", w.Body.String())

	var resp openapi.CreateItemResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Item)

	// Verify timezone was normalized to nil (not stored as empty string)
	assert.Nil(t, resp.Item.Timezone,
		"Empty timezone should be normalized to nil, got: %v", resp.Item.Timezone)
}

// TestCreateItem_InvalidTimezone_Rejected verifies that invalid IANA timezone names are rejected.
//
// Expected (RED phase): This test should PASS if validation already works, FAIL if invalid timezones accepted.
func TestCreateItem_InvalidTimezone_Rejected(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "Invalid TZ Test")
	require.NoError(t, err)

	reqBody := `{
		"title": "Test Item",
		"timezone": "NotAValidTimezone/Zone"
	}`

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
		strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code,
		"Invalid timezone should be rejected, got: %s", w.Body.String())

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
}

// === Phase 1.3: Nil Pointer Safety Tests ===

// TestCreateItem_NilItem_FailsGracefully verifies that the service layer handles nil item gracefully.
// This is defensive programming - the handler should never pass nil, but the service should not panic.
//
// Expected (RED phase): This test should PANIC if no nil check exists, or PASS if check exists.
func TestCreateItem_NilItem_FailsGracefully(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "Nil Safety Test")
	require.NoError(t, err)

	// Call service directly with nil item (bypassing handler)
	// This should return error, not panic
	_, err = ts.TodoService.CreateItem(ctx, list.ID, nil)

	// Should return error, not panic
	assert.Error(t, err, "Nil item should return error, not panic")
	// The error should be a domain error, not a panic recovery
	assert.NotContains(t, err.Error(), "panic", "Should not panic on nil item")
}

// TestCreateItem_Tags_Validation verifies tag handling in CreateItem
func TestCreateItem_Tags_Validation(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "Tags Test")
	require.NoError(t, err)

	t.Run("empty tag in array is accepted", func(t *testing.T) {
		reqBody := `{
			"title": "Test Item",
			"tags": ["", "valid-tag"]
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
			strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code,
			"Empty tag in array should be accepted, got: %s", w.Body.String())

		var resp openapi.CreateItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Item)
		require.NotNil(t, resp.Item.Tags)
		assert.Contains(t, *resp.Item.Tags, "", "Empty tag should be stored")
		assert.Contains(t, *resp.Item.Tags, "valid-tag")
	})

	t.Run("whitespace-only tag is accepted", func(t *testing.T) {
		reqBody := `{
			"title": "Test Item",
			"tags": ["   ", "valid-tag"]
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
			strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code,
			"Whitespace-only tag should be accepted, got: %s", w.Body.String())
	})

	t.Run("very long tag (1000 chars) is accepted", func(t *testing.T) {
		longTag := strings.Repeat("a", 1000)
		reqBody := fmt.Sprintf(`{
			"title": "Test Item",
			"tags": ["%s"]
		}`, longTag)

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
			strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code,
			"Very long tag should be accepted, got: %s", w.Body.String())

		var resp openapi.CreateItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Item)
		require.NotNil(t, resp.Item.Tags)
		assert.Contains(t, *resp.Item.Tags, longTag, "Long tag should be stored")
	})

	t.Run("special characters in tags are accepted", func(t *testing.T) {
		reqBody := `{
			"title": "Test Item",
			"tags": ["tag-with-dash", "tag_with_underscore", "tag.with.dot", "tag@special", "tag#hash"]
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
			strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code,
			"Special characters in tags should be accepted, got: %s", w.Body.String())

		var resp openapi.CreateItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Item)
		require.NotNil(t, resp.Item.Tags)
		assert.Len(t, *resp.Item.Tags, 5, "All special character tags should be stored")
	})

	t.Run("duplicate tags are accepted", func(t *testing.T) {
		reqBody := `{
			"title": "Test Item",
			"tags": ["work", "important", "work", "important"]
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
			strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code,
			"Duplicate tags should be accepted, got: %s", w.Body.String())

		var resp openapi.CreateItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Item)
		require.NotNil(t, resp.Item.Tags)
		// System may deduplicate or preserve duplicates - just verify accepted
		assert.True(t, len(*resp.Item.Tags) >= 2, "Tags should be stored (may be deduplicated)")
	})
}

// TestCreateItem_EstimatedDuration_Validation verifies duration parsing and validation
func TestCreateItem_EstimatedDuration_Validation(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "Duration Test")
	require.NoError(t, err)

	t.Run("invalid ISO8601 duration is rejected", func(t *testing.T) {
		reqBody := `{
			"title": "Test Item",
			"estimated_duration": "not-a-duration"
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
			strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"Invalid ISO8601 duration should be rejected with 400, got: %s", w.Body.String())

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		errorObj, ok := resp["error"].(map[string]any)
		require.True(t, ok, "Response should have error object")
		assert.Equal(t, "VALIDATION_ERROR", errorObj["code"],
			"Should return VALIDATION_ERROR for invalid duration")
	})

	t.Run("negative duration is rejected", func(t *testing.T) {
		reqBody := `{
			"title": "Test Item",
			"estimated_duration": "-PT1H"
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
			strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"Negative duration should be rejected with 400, got: %s", w.Body.String())

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		errorObj, ok := resp["error"].(map[string]any)
		require.True(t, ok, "Response should have error object")
		assert.Equal(t, "VALIDATION_ERROR", errorObj["code"],
			"Should return VALIDATION_ERROR for negative duration")
	})

	t.Run("zero duration is accepted", func(t *testing.T) {
		reqBody := `{
			"title": "Test Item",
			"estimated_duration": "PT0S"
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
			strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code,
			"Zero duration should be accepted, got: %s", w.Body.String())

		var resp openapi.CreateItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Item)
		require.NotNil(t, resp.Item.EstimatedDuration)
		assert.Equal(t, "PT0S", *resp.Item.EstimatedDuration)
	})

	t.Run("empty string duration is rejected", func(t *testing.T) {
		reqBody := `{
			"title": "Test Item",
			"estimated_duration": ""
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
			strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"Empty string duration should be rejected with 400, got: %s", w.Body.String())

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		errorObj, ok := resp["error"].(map[string]any)
		require.True(t, ok, "Response should have error object")
		assert.Equal(t, "VALIDATION_ERROR", errorObj["code"],
			"Should return VALIDATION_ERROR for empty duration string")
	})
}

// TestCreateItem_DueOffset_Validation verifies due_offset handling with/without starts_at
func TestCreateItem_DueOffset_Validation(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "DueOffset Test")
	require.NoError(t, err)

	t.Run("due_offset without starts_at is accepted but unused", func(t *testing.T) {
		reqBody := `{
			"title": "Test Item",
			"due_offset": "PT2H"
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
			strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code,
			"due_offset without starts_at should be accepted (stored but unused), got: %s", w.Body.String())

		var resp openapi.CreateItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Item)
		require.NotNil(t, resp.Item.DueOffset)
		assert.Equal(t, "PT2H", *resp.Item.DueOffset, "DueOffset should be stored")
		assert.Nil(t, resp.Item.DueAt, "DueAt should be nil when starts_at not provided")
	})

	t.Run("due_offset with starts_at does not auto-calculate due_at", func(t *testing.T) {
		startsAt := time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC)
		reqBody := fmt.Sprintf(`{
			"title": "Test Item",
			"starts_at": "%s",
			"due_offset": "PT2H30M"
		}`, startsAt.Format("2006-01-02"))

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
			strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code,
			"due_offset with starts_at should be accepted, got: %s", w.Body.String())

		var resp openapi.CreateItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Item)
		require.NotNil(t, resp.Item.StartsAt)
		require.NotNil(t, resp.Item.DueOffset)

		// Current behavior: due_at is NOT automatically calculated
		// Both fields are stored independently
		assert.Nil(t, resp.Item.DueAt,
			"DueAt should be nil - system does not auto-calculate from starts_at + due_offset")
	})
}

// TestCreateItem_RecurringTemplateID_Validation verifies recurring_template_id validation
func TestCreateItem_RecurringTemplateID_Validation(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "RecurringTemplate Test")
	require.NoError(t, err)

	t.Run("non-existent template ID is rejected", func(t *testing.T) {
		// Use a valid UUIDv7 that doesn't exist in the database
		nonExistentID := "019b9999-9999-7999-9999-999999999999"
		instanceDate := time.Date(2025, 12, 25, 0, 0, 0, 0, time.UTC)

		reqBody := fmt.Sprintf(`{
			"title": "Test Item",
			"recurring_template_id": "%s",
			"instance_date": "%s"
		}`, nonExistentID, instanceDate.Format(time.RFC3339))

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
			strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		// Expect 400 (validation error) or 404 (not found)
		assert.True(t, w.Code == http.StatusBadRequest || w.Code == http.StatusNotFound,
			"Non-existent template ID should be rejected with 400 or 404, got: %d - %s",
			w.Code, w.Body.String())

		if w.Code == http.StatusBadRequest {
			var resp map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			errorObj, ok := resp["error"].(map[string]any)
			require.True(t, ok, "Response should have error object")
			// Should be either VALIDATION_ERROR or NOT_FOUND
			assert.Contains(t, []string{"VALIDATION_ERROR", "NOT_FOUND"}, errorObj["code"],
				"Should return appropriate error code for non-existent template")
		}
	})

	t.Run("invalid UUID format for template ID is rejected", func(t *testing.T) {
		instanceDate := time.Date(2025, 12, 25, 0, 0, 0, 0, time.UTC)

		reqBody := fmt.Sprintf(`{
			"title": "Test Item",
			"recurring_template_id": "not-a-uuid",
			"instance_date": "%s"
		}`, instanceDate.Format(time.RFC3339))

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/items", list.ID),
			strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"Invalid UUID format should be rejected with 400, got: %s", w.Body.String())

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		errorObj, ok := resp["error"].(map[string]any)
		require.True(t, ok, "Response should have error object")
		// OpenAPI validates UUID format before reaching handler
		assert.Contains(t, []string{"VALIDATION_ERROR", "BAD_REQUEST", "INVALID_REQUEST"}, errorObj["code"],
			"Should return validation error for invalid UUID format")
	})
}
