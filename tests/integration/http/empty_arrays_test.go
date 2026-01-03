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

// TestEmptyArraysNotNull verifies that all array fields in responses
// return empty arrays [] instead of null when there are no items.
// This follows JSON API best practices and the OpenAPI spec which marks
// all response arrays as nullable: false.
func TestEmptyArraysNotNull(t *testing.T) {
	t.Run("empty lists response returns empty array not null", func(t *testing.T) {
		ts := SetupTestServer(t)
		defer ts.Cleanup()

		// Request lists when database is empty
		req := httptest.NewRequest(http.MethodGet, "/api/v1/lists", nil)
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Parse raw JSON to check if lists is null or []
		var rawResp map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &rawResp)
		require.NoError(t, err)

		// Verify lists exists and is an array (not null)
		lists, exists := rawResp["lists"]
		require.True(t, exists, "lists field should exist")
		require.NotNil(t, lists, "lists should not be null")

		// Verify it's an empty array
		listsArray, ok := lists.([]any)
		require.True(t, ok, "lists should be an array")
		assert.Empty(t, listsArray, "lists array should be empty")
	})

	t.Run("empty items response returns empty array not null", func(t *testing.T) {
		ts := SetupTestServer(t)
		defer ts.Cleanup()

		// Create a list with no items
		ctx := context.Background()
		list, err := ts.TodoService.CreateList(ctx, "Empty List")
		require.NoError(t, err)

		// Request items for empty list
		req := httptest.NewRequest(http.MethodGet, "/api/v1/lists/"+list.ID+"/items", nil)
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Parse raw JSON to check if items is null or []
		var rawResp map[string]any
		err = json.Unmarshal(w.Body.Bytes(), &rawResp)
		require.NoError(t, err)

		// Verify items exists and is an array (not null)
		items, exists := rawResp["items"]
		require.True(t, exists, "items field should exist")
		require.NotNil(t, items, "items should not be null")

		// Verify it's an empty array
		itemsArray, ok := items.([]any)
		require.True(t, ok, "items should be an array")
		assert.Empty(t, itemsArray, "items array should be empty")
	})

	t.Run("item with no tags returns empty array not null", func(t *testing.T) {
		ts := SetupTestServer(t)
		defer ts.Cleanup()

		// Create a list and an item with no tags via service (not HTTP)
		ctx := context.Background()
		list, err := ts.TodoService.CreateList(ctx, "Test List")
		require.NoError(t, err)

		// Create item directly via service (bypasses derefStringSlice helper)
		_, err = ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
			Title: "Item without tags",
			// Tags field not set (will be nil)
		})
		require.NoError(t, err)

		// Now fetch items via HTTP to see if tags is null or []
		req := httptest.NewRequest(http.MethodGet, "/api/v1/lists/"+list.ID+"/items", nil)
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Parse raw JSON to check if tags is null or []
		var rawResp map[string]any
		err = json.Unmarshal(w.Body.Bytes(), &rawResp)
		require.NoError(t, err)

		t.Logf("Raw JSON response: %s", w.Body.String())

		items, ok := rawResp["items"].([]any)
		require.True(t, ok, "items should be an array")
		require.Len(t, items, 1, "should have one item")

		item, ok := items[0].(map[string]any)
		require.True(t, ok, "item should be an object")

		// Verify tags exists and is an array (not null)
		tags, exists := item["tags"]
		require.True(t, exists, "tags field should exist")
		require.NotNil(t, tags, "tags should not be null")

		// Verify it's an empty array
		tagsArray, ok := tags.([]any)
		require.True(t, ok, "tags should be an array")
		assert.Empty(t, tagsArray, "tags array should be empty")
	})

	t.Run("empty recurring templates response returns empty array not null", func(t *testing.T) {
		ts := SetupTestServer(t)
		defer ts.Cleanup()

		// Create a list with no recurring templates
		ctx := context.Background()
		list, err := ts.TodoService.CreateList(ctx, "Test List")
		require.NoError(t, err)

		// Request recurring templates for list with none
		req := httptest.NewRequest(http.MethodGet, "/api/v1/lists/"+list.ID+"/recurring-templates", nil)
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Parse raw JSON to check if templates is null or []
		var rawResp map[string]any
		err = json.Unmarshal(w.Body.Bytes(), &rawResp)
		require.NoError(t, err)

		t.Logf("Raw JSON response: %s", w.Body.String())

		// Verify templates exists and is an array (not null)
		templates, exists := rawResp["templates"]
		require.True(t, exists, "templates field should exist")
		require.NotNil(t, templates, "templates should not be null")

		// Verify it's an empty array
		templatesArray, ok := templates.([]any)
		require.True(t, ok, "templates should be an array")
		assert.Empty(t, templatesArray, "templates array should be empty")
	})

	t.Run("recurring template with no tags returns empty array not null", func(t *testing.T) {
		ts := SetupTestServer(t)
		defer ts.Cleanup()

		// Create a list and a recurring template with no tags via service
		ctx := context.Background()
		list, err := ts.TodoService.CreateList(ctx, "Test List")
		require.NoError(t, err)

		// Create template directly via service (bypasses any HTTP helper)
		template := &domain.RecurringTemplate{
			ListID:                list.ID,
			Title:                 "Template without tags",
			RecurrencePattern:     domain.RecurrenceDaily,
			RecurrenceConfig:      map[string]any{}, // Required field
			IsActive:              true,
			SyncHorizonDays:       14,
			GenerationHorizonDays: 30,
			// Tags field not set (will be nil)
		}
		createdTemplate, err := ts.TodoService.CreateRecurringTemplate(ctx, template)
		require.NoError(t, err)

		// Fetch templates via HTTP to see if tags is null or []
		req := httptest.NewRequest(http.MethodGet, "/api/v1/lists/"+list.ID+"/recurring-templates", nil)
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Parse raw JSON to check if tags is null or []
		var rawResp map[string]any
		err = json.Unmarshal(w.Body.Bytes(), &rawResp)
		require.NoError(t, err)

		t.Logf("Raw JSON response: %s", w.Body.String())

		templates, ok := rawResp["templates"].([]any)
		require.True(t, ok, "templates should be an array")
		require.Len(t, templates, 1, "should have one template")

		templateObj, ok := templates[0].(map[string]any)
		require.True(t, ok, "template should be an object")

		// Verify this is our template
		require.Equal(t, createdTemplate.ID, templateObj["id"].(string))

		// Verify tags exists and is an array (not null)
		tags, exists := templateObj["tags"]
		require.True(t, exists, "tags field should exist")
		require.NotNil(t, tags, "tags should not be null")

		// Verify it's an empty array
		tagsArray, ok := tags.([]any)
		require.True(t, ok, "tags should be an array")
		assert.Empty(t, tagsArray, "tags array should be empty")
	})

	t.Run("error details returns empty array not null when no details", func(t *testing.T) {
		ts := SetupTestServer(t)
		defer ts.Cleanup()

		// Make a request that will cause an error without field-specific details
		// (e.g., trying to get a non-existent list)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/lists/00000000-0000-0000-0000-000000000000", nil)
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		// Parse raw JSON to check error structure
		var rawResp map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &rawResp)
		require.NoError(t, err)

		errorObj, ok := rawResp["error"].(map[string]any)
		require.True(t, ok, "error should be an object")

		// If details field exists, it should be an array (not null)
		if details, exists := errorObj["details"]; exists {
			require.NotNil(t, details, "details should not be null if present")
			_, ok := details.([]any)
			require.True(t, ok, "details should be an array if present")
		}
	})
}
