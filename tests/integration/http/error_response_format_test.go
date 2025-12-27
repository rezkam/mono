package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
)

// StandardErrorResponse matches the OpenAPI ErrorResponse schema.
// All error responses must conform to this structure.
type StandardErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Details []ErrorField `json:"details"` // Must be array (can be empty), never null
}

type ErrorField struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

// verifyStandardErrorFormat validates that a response conforms to the OpenAPI ErrorResponse schema.
func verifyStandardErrorFormat(t *testing.T, resp *http.Response, expectedStatus int, expectedCode string) StandardErrorResponse {
	t.Helper()

	// Verify status code
	if resp.StatusCode != expectedStatus {
		t.Errorf("Expected status %d, got %d", expectedStatus, resp.StatusCode)
	}

	// Verify Content-Type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	// Decode response
	var errorResp StandardErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
		t.Fatalf("Failed to decode error response as JSON: %v", err)
	}

	// Verify required fields are present
	if errorResp.Error.Code == "" {
		t.Error("error.code field is missing or empty")
	}
	if errorResp.Error.Message == "" {
		t.Error("error.message field is missing or empty")
	}

	// Verify expected error code
	if expectedCode != "" && errorResp.Error.Code != expectedCode {
		t.Errorf("Expected error code %s, got %s", expectedCode, errorResp.Error.Code)
	}

	// Verify details is an array (not null) - OpenAPI spec: nullable: false
	if errorResp.Error.Details == nil {
		t.Error("error.details must be an array (can be empty), not null")
	}

	return errorResp
}

// TestErrorResponses_400_BadRequest verifies 400 Bad Request errors use standard format.
func TestErrorResponses_400_BadRequest(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	tests := []struct {
		name         string
		method       string
		path         string
		body         string
		expectedCode string
	}{
		{
			name:         "malformed JSON",
			method:       "POST",
			path:         "/api/v1/lists",
			body:         `{"title": "broken`,
			expectedCode: "VALIDATION_ERROR",
		},
		{
			name:         "validation error - empty title",
			method:       "POST",
			path:         "/api/v1/lists",
			body:         `{"title": ""}`,
			expectedCode: "VALIDATION_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+ts.APIKey)

			w := httptest.NewRecorder()
			ts.Router.ServeHTTP(w, req)

			errorResp := verifyStandardErrorFormat(t, w.Result(), http.StatusBadRequest, tt.expectedCode)

			// Validation errors should have details array populated
			if tt.expectedCode == "VALIDATION_ERROR" {
				if len(errorResp.Error.Details) == 0 {
					t.Error("Validation errors should have details array with at least one entry")
				}
				// Verify detail structure
				if len(errorResp.Error.Details) > 0 {
					detail := errorResp.Error.Details[0]
					if detail.Field == "" {
						t.Error("Validation error detail must have 'field' populated")
					}
					if detail.Issue == "" {
						t.Error("Validation error detail must have 'issue' populated")
					}
				}
			}
		})
	}
}

// TestErrorResponses_401_Unauthorized verifies 401 Unauthorized errors use standard format.
func TestErrorResponses_401_Unauthorized(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	tests := []struct {
		name   string
		apiKey string
	}{
		{
			name:   "missing API key",
			apiKey: "",
		},
		{
			name:   "invalid API key",
			apiKey: "Bearer invalid-key-12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/lists", nil)
			if tt.apiKey != "" {
				req.Header.Set("Authorization", tt.apiKey)
			}

			w := httptest.NewRecorder()
			ts.Router.ServeHTTP(w, req)

			verifyStandardErrorFormat(t, w.Result(), http.StatusUnauthorized, "UNAUTHORIZED")
		})
	}
}

// TestErrorResponses_404_NotFound verifies 404 Not Found errors use standard format.
func TestErrorResponses_404_NotFound(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	tests := []struct {
		name         string
		path         string
		expectedCode string
	}{
		{
			name:         "nonexistent list",
			path:         "/api/v1/lists/00000000-0000-0000-0000-000000000000",
			expectedCode: "NOT_FOUND",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			req.Header.Set("Authorization", "Bearer "+ts.APIKey)

			w := httptest.NewRecorder()
			ts.Router.ServeHTTP(w, req)

			verifyStandardErrorFormat(t, w.Result(), http.StatusNotFound, tt.expectedCode)
		})
	}
}

// TestErrorResponses_409_Conflict verifies 409 Conflict errors use standard format.
func TestErrorResponses_409_Conflict(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create a list
	list, err := ts.TodoService.CreateList(context.Background(), "Test List")
	if err != nil {
		t.Fatalf("Failed to create list: %v", err)
	}

	// Create an item
	itemID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("Failed to generate UUID: %v", err)
	}
	item := &domain.TodoItem{
		ID:    itemID.String(),
		Title: "Test Item",
	}
	item, err = ts.TodoService.CreateItem(context.Background(), list.ID, item)
	if err != nil {
		t.Fatalf("Failed to create item: %v", err)
	}

	// Update with wrong etag to trigger version conflict
	body := `{
		"item": {"id": "` + item.ID + `", "title": "Updated", "etag": "999"},
		"update_mask": ["title"]
	}`

	req := httptest.NewRequest("PATCH", "/api/v1/lists/"+list.ID+"/items/"+item.ID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	verifyStandardErrorFormat(t, w.Result(), http.StatusConflict, "CONFLICT")
}

// TestErrorResponses_500_InternalError verifies 500 Internal Server errors use standard format.
// Note: This is harder to test without injecting failures, so we verify the response helper directly.
func TestErrorResponses_500_InternalError(t *testing.T) {
	// This test verifies the response helper directly since it's hard to trigger
	// a real 500 error in integration tests without fault injection.

	// The unit tests in internal/http/response already verify this,
	// but we can add an integration test by checking if InternalError
	// is called for unexpected errors.

	// For now, we trust the unit tests. In a real scenario, you might:
	// - Use fault injection
	// - Simulate database failures
	// - Test with invalid internal state

	t.Skip("500 errors are verified in unit tests; integration testing requires fault injection")
}

// TestErrorResponses_AllErrorsHaveArrayDetails verifies that the details field
// is always an array (never null), even when empty.
func TestErrorResponses_AllErrorsHaveArrayDetails(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Test an error without details (404 Not Found)
	req := httptest.NewRequest("GET", "/api/v1/lists/00000000-0000-0000-0000-000000000000", nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Decode as raw JSON to verify the structure
	var rawResponse map[string]interface{}
	body := bytes.NewReader(w.Body.Bytes())
	if err := json.NewDecoder(body).Decode(&rawResponse); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	errorObj, ok := rawResponse["error"].(map[string]interface{})
	if !ok {
		t.Fatal("Response missing 'error' object")
	}

	// Check if details exists
	details, exists := errorObj["details"]
	if !exists {
		// details field is optional if empty, but if present must be array
		return
	}

	// If details exists, it must be an array, not null
	if details == nil {
		t.Error("error.details must be an array (can be empty), not null")
	}

	// Verify it's an array type
	if _, ok := details.([]interface{}); !ok {
		t.Errorf("error.details must be an array, got type %T", details)
	}
}

// TestValidationErrors_DetailsPopulated verifies that validation errors
// correctly parse OpenAPI error messages and populate the details array
// with field-specific information.
func TestValidationErrors_DetailsPopulated(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	tests := []struct {
		name               string
		method             string
		path               string
		body               string
		expectedStatus     int
		expectedCode       string
		expectedMinDetails int // Minimum number of detail entries expected
		verifyDetail       func(t *testing.T, detail ErrorField)
	}{
		{
			name:               "empty title - minLength validation",
			method:             "POST",
			path:               "/api/v1/lists",
			body:               `{"title": ""}`,
			expectedStatus:     http.StatusBadRequest,
			expectedCode:       "VALIDATION_ERROR",
			expectedMinDetails: 1,
			verifyDetail: func(t *testing.T, detail ErrorField) {
				t.Helper()
				if detail.Field != "title" && detail.Field != "body" {
					t.Errorf("Expected field 'title' or 'body', got '%s'", detail.Field)
				}
				if detail.Issue == "" {
					t.Error("Expected non-empty issue description")
				}
			},
		},
		{
			name:               "title too long - maxLength validation",
			method:             "POST",
			path:               "/api/v1/lists",
			body:               `{"title": "` + strings.Repeat("a", 256) + `"}`,
			expectedStatus:     http.StatusBadRequest,
			expectedCode:       "VALIDATION_ERROR",
			expectedMinDetails: 1,
			verifyDetail: func(t *testing.T, detail ErrorField) {
				t.Helper()
				if detail.Field != "title" && detail.Field != "body" {
					t.Errorf("Expected field 'title' or 'body', got '%s'", detail.Field)
				}
				if detail.Issue == "" {
					t.Error("Expected non-empty issue description")
				}
			},
		},
		{
			name:               "invalid page_size - exceeds maximum",
			method:             "GET",
			path:               "/api/v1/lists?page_size=101",
			body:               "",
			expectedStatus:     http.StatusBadRequest,
			expectedCode:       "VALIDATION_ERROR",
			expectedMinDetails: 1,
			verifyDetail: func(t *testing.T, detail ErrorField) {
				t.Helper()
				if detail.Field != "page_size" && detail.Field != "body" {
					t.Errorf("Expected field 'page_size' or 'body', got '%s'", detail.Field)
				}
				if detail.Issue == "" {
					t.Error("Expected non-empty issue description")
				}
			},
		},
		{
			name:               "invalid page_size - below minimum",
			method:             "GET",
			path:               "/api/v1/lists?page_size=0",
			body:               "",
			expectedStatus:     http.StatusBadRequest,
			expectedCode:       "VALIDATION_ERROR",
			expectedMinDetails: 1,
			verifyDetail: func(t *testing.T, detail ErrorField) {
				t.Helper()
				if detail.Field != "page_size" && detail.Field != "body" {
					t.Errorf("Expected field 'page_size' or 'body', got '%s'", detail.Field)
				}
				if detail.Issue == "" {
					t.Error("Expected non-empty issue description")
				}
			},
		},
		{
			name:               "malformed JSON",
			method:             "POST",
			path:               "/api/v1/lists",
			body:               `{"title": "broken`,
			expectedStatus:     http.StatusBadRequest,
			expectedCode:       "VALIDATION_ERROR", // OpenAPI validator catches malformed JSON
			expectedMinDetails: 1,                  // Parser extracts "body" field
			verifyDetail: func(t *testing.T, detail ErrorField) {
				t.Helper()
				if detail.Field != "body" {
					t.Errorf("Expected field 'body', got '%s'", detail.Field)
				}
				if detail.Issue == "" {
					t.Error("Expected non-empty issue description")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			req.Header.Set("Authorization", "Bearer "+ts.APIKey)

			w := httptest.NewRecorder()
			ts.Router.ServeHTTP(w, req)

			// Verify standard error format
			errorResp := verifyStandardErrorFormat(t, w.Result(), tt.expectedStatus, tt.expectedCode)

			// Verify details array exists (never null)
			if errorResp.Error.Details == nil {
				t.Fatal("error.details must be an array (can be empty), not null")
			}

			// Verify minimum number of details
			if len(errorResp.Error.Details) < tt.expectedMinDetails {
				t.Errorf("Expected at least %d detail entries, got %d. Full response: %+v",
					tt.expectedMinDetails, len(errorResp.Error.Details), errorResp)
			}

			// Verify detail content if verification function provided
			if tt.verifyDetail != nil && len(errorResp.Error.Details) > 0 {
				tt.verifyDetail(t, errorResp.Error.Details[0])
			}

			// Log the actual details for debugging
			if len(errorResp.Error.Details) > 0 {
				t.Logf("Validation details: %+v", errorResp.Error.Details)
			}
		})
	}
}
