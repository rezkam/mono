package http_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMaxBodyBytes_RejectsOversizedRequest verifies that requests exceeding
// the configured body size limit are rejected with 413 Request Entity Too Large.
//
// This test sends a request body that exceeds the configured MONO_HTTP_MAX_BODY_BYTES
// limit (default 1MB) to verify the middleware enforces the limit.
func TestMaxBodyBytes_RejectsOversizedRequest(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create a request body larger than the default 1MB limit
	// Use 1.5MB of JSON data to exceed the limit
	largeTitle := strings.Repeat("x", 1.5*1024*1024) // 1.5MB
	reqBody := `{"title":"` + largeTitle + `"}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/lists", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Should return 413 Request Entity Too Large
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code,
		"Expected 413 for request body exceeding 1MB limit")
}

// TestMaxBodyBytes_AcceptsRequestWithinLimit verifies that requests within
// the size limit are processed normally.
func TestMaxBodyBytes_AcceptsRequestWithinLimit(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create a small request body well within the 1MB limit
	reqBody := `{"title":"Small title"}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/lists", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Should return 201 Created (not 413)
	assert.Equal(t, http.StatusCreated, w.Code,
		"Expected 201 for request within 1MB limit")
}

// TestMaxBodyBytes_RejectsExactlyOverLimit verifies that a request exactly
// 1 byte over the limit is rejected.
func TestMaxBodyBytes_RejectsExactlyOverLimit(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create a request body exactly 1 byte over 1MB
	// 1MB = 1048576 bytes
	// Need to account for JSON structure: {"title":"..."} = 12 extra bytes
	// So title should be 1048576 - 12 + 1 = 1048565 bytes
	largeTitle := strings.Repeat("x", 1048565)
	reqBody := `{"title":"` + largeTitle + `"}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/lists", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Should return 413 Request Entity Too Large
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code,
		"Expected 413 for request exactly 1 byte over limit")
}
