package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPIResponseValidation_ArraysNeverNull verifies that all array fields in HTTP responses
// are valid JSON arrays (never null), matching the OpenAPI spec which marks arrays as nullable: false.
// This tests the actual HTTP response bytes, not just the deserialized objects.
func TestAPIResponseValidation_ArraysNeverNull(t *testing.T) {
	t.Run("GET /lists returns valid arrays field", func(t *testing.T) {
		ts := SetupTestServer(t)
		defer ts.Cleanup()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/lists", nil)
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		// Validate raw JSON structure
		body := w.Body.Bytes()
		t.Logf("Response: %s", string(body))

		assertJSONArrayNotNull(t, body, "lists", "Response should have 'lists' as an array, not null")
	})

	t.Run("GET /lists/{id}/items returns valid arrays field", func(t *testing.T) {
		ts := SetupTestServer(t)
		defer ts.Cleanup()

		ctx := context.Background()
		list, err := ts.TodoService.CreateList(ctx, "Test List")
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/lists/"+list.ID+"/items", nil)
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		body := w.Body.Bytes()
		t.Logf("Response: %s", string(body))

		assertJSONArrayNotNull(t, body, "items", "Response should have 'items' as an array, not null")
	})

	t.Run("GET /lists/{id}/items with items returns valid tags arrays", func(t *testing.T) {
		ts := SetupTestServer(t)
		defer ts.Cleanup()

		ctx := context.Background()
		list, err := ts.TodoService.CreateList(ctx, "Test List")
		require.NoError(t, err)

		// Create item without tags via service (storage layer)
		_, err = ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
			Title: "Item without tags",
			// Tags not set - will be nil in domain
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/lists/"+list.ID+"/items", nil)
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		body := w.Body.Bytes()
		t.Logf("Response: %s", string(body))

		// Verify each item's tags field is an array, not null
		assertNestedJSONArrayNotNull(t, body, "items", "tags", "Each item should have 'tags' as an array, not null")
	})

	t.Run("GET /lists/{id}/recurring-templates returns valid arrays field", func(t *testing.T) {
		ts := SetupTestServer(t)
		defer ts.Cleanup()

		ctx := context.Background()
		list, err := ts.TodoService.CreateList(ctx, "Test List")
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/lists/"+list.ID+"/recurring-templates", nil)
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		body := w.Body.Bytes()
		t.Logf("Response: %s", string(body))

		assertJSONArrayNotNull(t, body, "templates", "Response should have 'templates' as an array, not null")
	})

	t.Run("GET /lists/{id}/recurring-templates with templates returns valid tags arrays", func(t *testing.T) {
		ts := SetupTestServer(t)
		defer ts.Cleanup()

		ctx := context.Background()
		list, err := ts.TodoService.CreateList(ctx, "Test List")
		require.NoError(t, err)

		// Create template without tags via service (storage layer)
		_, err = ts.TodoService.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
			ListID:                list.ID,
			Title:                 "Template without tags",
			RecurrencePattern:     domain.RecurrenceDaily,
			RecurrenceConfig:      map[string]any{},
			IsActive:              true,
			SyncHorizonDays:       14,
			GenerationHorizonDays: 30,
			// Tags not set - will be nil in domain
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/lists/"+list.ID+"/recurring-templates", nil)
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		body := w.Body.Bytes()
		t.Logf("Response: %s", string(body))

		// Verify each template's tags field is an array, not null
		assertNestedJSONArrayNotNull(t, body, "templates", "tags", "Each template should have 'tags' as an array, not null")
	})

	t.Run("error responses with details field never return null", func(t *testing.T) {
		ts := SetupTestServer(t)
		defer ts.Cleanup()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/lists/00000000-0000-0000-0000-000000000000", nil)
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)

		body := w.Body.Bytes()
		t.Logf("Response: %s", string(body))

		// Error responses may omit details entirely, but if present should be an array
		var response map[string]any
		require.NoError(t, json.Unmarshal(body, &response))

		if errorObj, ok := response["error"].(map[string]any); ok {
			if details, exists := errorObj["details"]; exists {
				require.NotNil(t, details, "If 'details' field exists, it should not be null")
				_, isArray := details.([]any)
				require.True(t, isArray, "If 'details' field exists, it should be an array")
			}
		}
	})
}

// assertJSONArrayNotNull verifies that a top-level field in JSON is an array, not null.
func assertJSONArrayNotNull(t *testing.T, jsonBytes []byte, fieldName string, msgAndArgs ...any) {
	t.Helper()

	var data map[string]any
	err := json.Unmarshal(jsonBytes, &data)
	require.NoError(t, err, "Response should be valid JSON")

	field, exists := data[fieldName]
	require.True(t, exists, "Field '%s' should exist in response", fieldName)
	require.NotNil(t, field, msgAndArgs...)

	_, isArray := field.([]any)
	require.True(t, isArray, "Field '%s' should be a JSON array, not null or other type. Got: %T", fieldName, field)
}

// assertNestedJSONArrayNotNull verifies that fields within an array are arrays, not null.
// For example, verifies each item in "items" array has a "tags" field that is an array.
func assertNestedJSONArrayNotNull(t *testing.T, jsonBytes []byte, arrayField string, nestedField string, msgAndArgs ...any) {
	t.Helper()

	var data map[string]any
	err := json.Unmarshal(jsonBytes, &data)
	require.NoError(t, err, "Response should be valid JSON")

	arrayData, exists := data[arrayField]
	require.True(t, exists, "Field '%s' should exist", arrayField)
	require.NotNil(t, arrayData, "Field '%s' should not be null", arrayField)

	array, isArray := arrayData.([]any)
	require.True(t, isArray, "Field '%s' should be an array", arrayField)

	if len(array) == 0 {
		t.Logf("Array '%s' is empty, skipping nested field validation", arrayField)
		return
	}

	// Check each item in the array
	for i, item := range array {
		itemMap, ok := item.(map[string]any)
		require.True(t, ok, "Item %d in '%s' should be an object", i, arrayField)

		nestedValue, exists := itemMap[nestedField]
		require.True(t, exists, "Item %d should have field '%s'", i, nestedField)

		if len(msgAndArgs) > 0 {
			require.NotNil(t, nestedValue, "Item %d: %v", i, msgAndArgs[0])
		} else {
			require.NotNil(t, nestedValue, "Item %d: field '%s' should not be null", i, nestedField)
		}

		_, isNestedArray := nestedValue.([]any)
		require.True(t, isNestedArray, "Item %d: field '%s' should be an array, not null. Got: %T", i, nestedField, nestedValue)
	}
}

// TestHTTPResponseReader verifies we can read and parse HTTP responses.
func TestHTTPResponseReader(t *testing.T) {
	t.Run("httptest.ResponseRecorder provides valid JSON", func(t *testing.T) {
		w := httptest.NewRecorder()
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"items":[],"next_page_token":null}`))
		require.NoError(t, err)

		// Read body multiple ways to verify test infrastructure
		bodyBytes := w.Body.Bytes()
		require.NotEmpty(t, bodyBytes)

		bodyString := w.Body.String()
		require.Contains(t, bodyString, "items")

		// Parse as JSON
		var data map[string]any
		err = json.Unmarshal(bodyBytes, &data)
		require.NoError(t, err)

		items, exists := data["items"]
		require.True(t, exists)
		require.NotNil(t, items)

		itemsArray, ok := items.([]any)
		require.True(t, ok)
		assert.Empty(t, itemsArray)
	})

	t.Run("can detect null vs empty array in JSON", func(t *testing.T) {
		nullJSON := `{"field":null}`
		emptyArrayJSON := `{"field":[]}`

		var nullData map[string]any
		err := json.Unmarshal([]byte(nullJSON), &nullData)
		require.NoError(t, err)
		assert.Nil(t, nullData["field"], "null should parse as nil")

		var arrayData map[string]any
		err = json.Unmarshal([]byte(emptyArrayJSON), &arrayData)
		require.NoError(t, err)
		assert.NotNil(t, arrayData["field"], "[] should not parse as nil")
		arr, ok := arrayData["field"].([]any)
		assert.True(t, ok, "[] should parse as array")
		assert.Empty(t, arr, "[] should be empty array")
	})
}

// BenchmarkJSONArrayValidation benchmarks the JSON validation helper.
func BenchmarkJSONArrayValidation(b *testing.B) {
	jsonBytes := []byte(`{"items":[{"id":"1","tags":[]},{"id":"2","tags":["a","b"]}]}`)

	for b.Loop() {
		var data map[string]any
		_ = json.Unmarshal(jsonBytes, &data)

		items, _ := data["items"].([]any)
		for _, item := range items {
			itemMap, _ := item.(map[string]any)
			_, _ = itemMap["tags"].([]any)
		}
	}
}
