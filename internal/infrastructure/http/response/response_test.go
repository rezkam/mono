package response_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rezkam/mono/internal/infrastructure/http/response"
)

// unencodableType simulates a type that fails during JSON encoding.
// This represents real scenarios like streaming data, channels, or types
// with custom MarshalJSON that can fail.
type unencodableType struct {
	BadField chan int `json:"bad_field"` // Channels cannot be JSON encoded
}

func (u unencodableType) MarshalJSON() ([]byte, error) {
	// Simulate encoding failure
	_, err := json.Marshal(u.BadField)
	return nil, err
}

// TestOK_EncodingFailure_Returns500WithErrorJSON verifies that if JSON marshaling
// fails, we return HTTP 500 with a proper JSON error response (not 200 OK).
//
// This ensures:
// 1. No success status when encoding fails
// 2. Response body is valid JSON in ErrorResponse format
// 3. Maintains OpenAPI spec compliance (all responses are JSON)
func TestOK_EncodingFailure_Returns500WithErrorJSON(t *testing.T) {
	// Arrange: Create response recorder and data that will fail encoding
	w := httptest.NewRecorder()
	data := unencodableType{}

	// Act: Try to send successful response with unencodable data
	response.OK(w, data)

	// Assert: Should return 500 Internal Server Error
	result := w.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected 500 Internal Server Error when marshaling fails, got %d", result.StatusCode)
	}

	// Verify Content-Type is JSON
	contentType := result.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	// Verify response body is valid ErrorResponse JSON
	var errorResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(result.Body).Decode(&errorResp); err != nil {
		t.Fatalf("Response body is not valid JSON: %v", err)
	}

	// Verify error details
	if errorResp.Error.Code != "INTERNAL_ERROR" {
		t.Errorf("Expected error code INTERNAL_ERROR, got %s", errorResp.Error.Code)
	}
	if errorResp.Error.Message != "failed to encode response" {
		t.Errorf("Expected error message 'failed to encode response', got %s", errorResp.Error.Message)
	}
}

// TestCreated_EncodingFailure_Returns500WithErrorJSON verifies the same behavior
// for 201 Created responses - marshaling failures return 500 with JSON error.
func TestCreated_EncodingFailure_Returns500WithErrorJSON(t *testing.T) {
	// Arrange
	w := httptest.NewRecorder()
	data := unencodableType{}

	// Act
	response.Created(w, data)

	// Assert
	result := w.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected 500 Internal Server Error when marshaling fails, got %d", result.StatusCode)
	}

	// Verify it's valid JSON error response
	var errorResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(result.Body).Decode(&errorResp); err != nil {
		t.Fatalf("Response body is not valid JSON: %v", err)
	}

	if errorResp.Error.Code != "INTERNAL_ERROR" {
		t.Errorf("Expected error code INTERNAL_ERROR, got %s", errorResp.Error.Code)
	}
}

// TestOK_Success_ReturnsValidJSON verifies that successful marshaling returns
// 200 OK with valid JSON response.
func TestOK_Success_ReturnsValidJSON(t *testing.T) {
	// Arrange: Create normal, encodable data
	w := httptest.NewRecorder()
	data := map[string]any{
		"id":      "123",
		"message": "success",
		"items":   []string{"a", "b", "c"},
	}

	// Act
	response.OK(w, data)

	// Assert: Should return 200 OK
	result := w.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", result.StatusCode)
	}

	// Verify Content-Type
	if ct := result.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", ct)
	}

	// Verify response is valid JSON
	var decoded map[string]any
	if err := json.NewDecoder(result.Body).Decode(&decoded); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	// Verify data matches
	if decoded["id"] != "123" {
		t.Errorf("Expected id=123, got %v", decoded["id"])
	}
	if decoded["message"] != "success" {
		t.Errorf("Expected message=success, got %v", decoded["message"])
	}
}

// TestCreated_Success_ReturnsValidJSON verifies that successful marshaling returns
// 201 Created with valid JSON response.
func TestCreated_Success_ReturnsValidJSON(t *testing.T) {
	// Arrange
	w := httptest.NewRecorder()
	data := map[string]string{"id": "new-resource-123"}

	// Act
	response.Created(w, data)

	// Assert: Should return 201 Created
	result := w.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusCreated {
		t.Errorf("Expected 201 Created, got %d", result.StatusCode)
	}

	// Verify Content-Type
	if ct := result.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", ct)
	}

	// Verify response is valid JSON
	var decoded map[string]string
	if err := json.NewDecoder(result.Body).Decode(&decoded); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	if decoded["id"] != "new-resource-123" {
		t.Errorf("Expected id=new-resource-123, got %v", decoded["id"])
	}
}

// TestError_Success_ReturnsValidJSON verifies that error responses return valid JSON.
func TestError_Success_ReturnsValidJSON(t *testing.T) {
	// Arrange
	w := httptest.NewRecorder()

	// Act: Send a 400 Bad Request error
	response.Error(w, "INVALID_INPUT", "missing required field", http.StatusBadRequest)

	// Assert
	result := w.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request, got %d", result.StatusCode)
	}

	// Verify Content-Type
	if ct := result.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", ct)
	}

	// Verify response matches ErrorResponse schema
	var errorResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(result.Body).Decode(&errorResp); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	if errorResp.Error.Code != "INVALID_INPUT" {
		t.Errorf("Expected code=INVALID_INPUT, got %s", errorResp.Error.Code)
	}
	if errorResp.Error.Message != "missing required field" {
		t.Errorf("Expected message='missing required field', got %s", errorResp.Error.Message)
	}
}

// TestValidationError_Success_ReturnsValidJSON verifies that validation errors return valid JSON.
func TestValidationError_Success_ReturnsValidJSON(t *testing.T) {
	// Arrange
	w := httptest.NewRecorder()

	// Act
	response.ValidationError(w, "email", "invalid format")

	// Assert
	result := w.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request, got %d", result.StatusCode)
	}

	// Verify Content-Type
	if ct := result.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", ct)
	}

	// Verify response matches ErrorResponse schema with details
	var errorResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details []struct {
				Field string `json:"field"`
				Issue string `json:"issue"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(result.Body).Decode(&errorResp); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	if errorResp.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("Expected code=VALIDATION_ERROR, got %s", errorResp.Error.Code)
	}
	if errorResp.Error.Message != "validation failed" {
		t.Errorf("Expected message='validation failed', got %s", errorResp.Error.Message)
	}
	if len(errorResp.Error.Details) != 1 {
		t.Fatalf("Expected 1 detail, got %d", len(errorResp.Error.Details))
	}
	if errorResp.Error.Details[0].Field != "email" {
		t.Errorf("Expected field=email, got %s", errorResp.Error.Details[0].Field)
	}
	if errorResp.Error.Details[0].Issue != "invalid format" {
		t.Errorf("Expected issue='invalid format', got %s", errorResp.Error.Details[0].Issue)
	}
}
