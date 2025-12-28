package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oapi-codegen/runtime/types"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/http/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecurringTemplate_ListOwnership verifies that recurring templates
// can only be accessed via their owning list's route.
//
// Security concern: Without list ownership validation, templates could be
// accessed/modified via any list_id in the path, breaking route semantics
// and potentially leaking information across tenant boundaries.

func TestRecurringTemplate_GetViaWrongList_ReturnsNotFound(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create two lists
	listA := createTestList(t, ts, "List A - Owner")
	listB := createTestList(t, ts, "List B - Wrong List")

	// Create a template in List A
	template := createTestRecurringTemplate(t, ts, listA.Id.String(), "Template in List A")

	// Try to GET the template via List B's route - should fail
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", listB.Id.String(), template.Id.String()), nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Should return 404, not 200
	assert.Equal(t, http.StatusNotFound, w.Code,
		"Accessing template via wrong list should return 404, not allow cross-list access")

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Message)
	assert.Equal(t, "recurring template not found", *resp.Error.Message)
}

func TestRecurringTemplate_UpdateViaWrongList_ReturnsNotFound(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create two lists
	listA := createTestList(t, ts, "List A - Owner")
	listB := createTestList(t, ts, "List B - Wrong List")

	// Create a template in List A
	template := createTestRecurringTemplate(t, ts, listA.Id.String(), "Template in List A")

	// Try to UPDATE the template via List B's route - should fail
	reqBody := openapi.UpdateRecurringTemplateRequest{
		Template: openapi.RecurringItemTemplate{
			Title: ptrString("Malicious Update"),
		},
		UpdateMask: []openapi.UpdateRecurringTemplateRequestUpdateMask{
			openapi.UpdateRecurringTemplateRequestUpdateMaskTitle,
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", listB.Id.String(), template.Id.String()),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Should return 404, not 200
	assert.Equal(t, http.StatusNotFound, w.Code,
		"Updating template via wrong list should return 404, not allow cross-list modification")

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Message)
	assert.Equal(t, "recurring template not found", *resp.Error.Message)

	// Verify the template was NOT modified
	originalTemplate, err := ts.TodoService.GetRecurringTemplate(context.Background(), listA.Id.String(), template.Id.String())
	require.NoError(t, err)
	assert.Equal(t, "Template in List A", originalTemplate.Title,
		"Template should not have been modified via wrong list")
}

func TestRecurringTemplate_DeleteViaWrongList_ReturnsNotFound(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create two lists
	listA := createTestList(t, ts, "List A - Owner")
	listB := createTestList(t, ts, "List B - Wrong List")

	// Create a template in List A
	template := createTestRecurringTemplate(t, ts, listA.Id.String(), "Template in List A")

	// Try to DELETE the template via List B's route - should fail
	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", listB.Id.String(), template.Id.String()), nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Should return 404, not 204
	assert.Equal(t, http.StatusNotFound, w.Code,
		"Deleting template via wrong list should return 404, not allow cross-list deletion")

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Message)
	assert.Equal(t, "recurring template not found", *resp.Error.Message)

	// Verify the template was NOT deleted
	_, err := ts.TodoService.GetRecurringTemplate(context.Background(), listA.Id.String(), template.Id.String())
	require.NoError(t, err, "Template should still exist - it should not have been deleted via wrong list")
}

func TestRecurringTemplate_AccessViaCorrectList_Works(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create a list and template
	list := createTestList(t, ts, "Owner List")
	template := createTestRecurringTemplate(t, ts, list.Id.String(), "My Template")

	// GET via correct list should work
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", list.Id.String(), template.Id.String()), nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Accessing template via correct list should work")

	var resp openapi.GetRecurringTemplateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Template)
	assert.Equal(t, "My Template", *resp.Template.Title)
}

// === Helper Functions ===

func createTestRecurringTemplate(t *testing.T, ts *TestServer, listID, title string) *openapi.RecurringItemTemplate {
	t.Helper()

	// Use service directly for reliable test setup
	template := &domain.RecurringTemplate{
		ListID:            listID,
		Title:             title,
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  make(map[string]any),
	}

	created, err := ts.TodoService.CreateRecurringTemplate(context.Background(), template)
	require.NoError(t, err, "Failed to create test recurring template")

	// Map to openapi type for return
	id := types.UUID{}
	_ = id.UnmarshalText([]byte(created.ID))
	return &openapi.RecurringItemTemplate{
		Id:    &id,
		Title: &created.Title,
	}
}
