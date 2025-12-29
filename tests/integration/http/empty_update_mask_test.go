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
	"github.com/rezkam/mono/internal/infrastructure/http/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateItem_EmptyUpdateMask verifies that UpdateItem returns a validation error
// when update_mask is empty.
func TestUpdateItem_EmptyUpdateMask(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list and item
	list, err := ts.TodoService.CreateList(ctx, "Empty Mask Test List")
	require.NoError(t, err)

	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title: "Test Item",
	})
	require.NoError(t, err)

	// Try to update with empty update_mask
	title := "New Title"
	reqBody := openapi.UpdateItemRequest{
		Item: openapi.TodoItem{
			Title: &title,
		},
		UpdateMask: []openapi.UpdateItemRequestUpdateMask{}, // Empty mask
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
		"expected 400 for empty update_mask, got %d: %s", w.Code, w.Body.String())

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
	assert.Contains(t, *resp.Error.Message, "update_mask cannot be empty")
}

// TestUpdateRecurringTemplate_EmptyUpdateMask verifies that UpdateRecurringTemplate
// returns a validation error when update_mask is empty.
func TestUpdateRecurringTemplate_EmptyUpdateMask(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create a list and recurring template
	list, err := ts.TodoService.CreateList(ctx, "Empty Mask Template Test")
	require.NoError(t, err)

	template, err := ts.TodoService.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
		ListID:            list.ID,
		Title:             "Test Template",
		RecurrencePattern: domain.RecurrenceDaily,
	})
	require.NoError(t, err)

	// Try to update with empty update_mask
	title := "New Title"
	reqBody := openapi.UpdateRecurringTemplateRequest{
		Template: openapi.RecurringItemTemplate{
			Title: &title,
		},
		UpdateMask: []openapi.UpdateRecurringTemplateRequestUpdateMask{}, // Empty mask
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", list.ID, template.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Should return 400 Bad Request with validation error
	assert.Equal(t, http.StatusBadRequest, w.Code,
		"expected 400 for empty update_mask, got %d: %s", w.Code, w.Body.String())

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "VALIDATION_ERROR", *resp.Error.Code)
	assert.Contains(t, *resp.Error.Message, "update_mask cannot be empty")
}
