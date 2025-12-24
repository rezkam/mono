package http_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/http/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	errCodeNotFound = "NOT_FOUND"

	// Expected error messages for specific resource types
	errMsgListNotFound     = "list not found"
	errMsgItemNotFound     = "item not found"
	errMsgTemplateNotFound = "recurring template not found"
	errMsgResourceNotFound = "resource not found" // Generic fallback (should NOT be used)

	assertMsgList     = "Should return specific 'list not found' not generic 'resource not found'"
	assertMsgItem     = "Should return specific 'item not found' not generic 'resource not found'"
	assertMsgTemplate = "Should return specific 'recurring template not found' not generic 'resource not found'"
)

// TestNotFoundErrors_ConsistentErrorMessages verifies that all not-found errors
// return specific error messages ("list not found", "item not found", "recurring template not found")
// rather than generic "resource not found" messages.
//
// This ensures:
// - Better client error handling (can differentiate between resource types)
// - Consistent API error responses across all endpoints
// - Proper domain error mapping in repository layer

// === List Not Found Tests ===

func TestNotFoundErrors_GetNonexistentList_ReturnsListNotFound(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	nonexistentID := uuid.Must(uuid.NewV7()).String()

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/lists/%s", nonexistentID), nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, errCodeNotFound, *resp.Error.Code)
	require.NotNil(t, resp.Error.Message)
	assert.Equal(t, errMsgListNotFound, *resp.Error.Message, assertMsgList)
}

// === Item Not Found Tests ===

func TestNotFoundErrors_UpdateNonexistentItem_ReturnsItemNotFound(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	list := createTestList(t, ts, "Test List")
	nonexistentItemID := uuid.Must(uuid.NewV7()).String()

	reqBody := openapi.UpdateItemRequest{
		Item: &openapi.TodoItem{
			Title: ptrString("Updated Title"),
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/lists/%s/items/%s", list.Id.String(), nonexistentItemID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Message)
	assert.Equal(t, errMsgItemNotFound, *resp.Error.Message, assertMsgItem)
}

// === Recurring Template Not Found Tests ===

func TestNotFoundErrors_GetNonexistentRecurringTemplate_ReturnsTemplateNotFound(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	nonexistentTemplateID := uuid.Must(uuid.NewV7()).String()

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/recurring-templates/%s", nonexistentTemplateID), nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, errCodeNotFound, *resp.Error.Code)
	require.NotNil(t, resp.Error.Message)
	assert.Equal(t, errMsgTemplateNotFound, *resp.Error.Message, assertMsgTemplate)
}

func TestNotFoundErrors_UpdateNonexistentRecurringTemplate_ReturnsTemplateNotFound(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	nonexistentTemplateID := uuid.Must(uuid.NewV7()).String()

	reqBody := openapi.UpdateRecurringTemplateRequest{
		Template: &openapi.RecurringTaskTemplate{
			Title: ptrString("Updated Title"),
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/recurring-templates/%s", nonexistentTemplateID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Message)
	assert.Equal(t, errMsgTemplateNotFound, *resp.Error.Message, assertMsgTemplate)
}

func TestNotFoundErrors_DeleteNonexistentRecurringTemplate_ReturnsTemplateNotFound(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	nonexistentTemplateID := uuid.Must(uuid.NewV7()).String()

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/recurring-templates/%s", nonexistentTemplateID), nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Message)
	assert.Equal(t, errMsgTemplateNotFound, *resp.Error.Message, assertMsgTemplate)
}

// === Helper Functions ===

func createTestList(t *testing.T, ts *TestServer, title string) *openapi.TodoList {
	t.Helper()

	reqBody := openapi.CreateListRequest{
		Title: title,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/lists", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "Failed to create test list")

	var resp openapi.CreateListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.List, "Response list should not be nil")
	require.NotNil(t, resp.List.Id, "List ID should not be nil")

	return resp.List
}

func ptrString(s string) *string {
	return &s
}
