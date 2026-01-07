package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rezkam/mono/internal/infrastructure/http/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateRecurringTemplate_Title_Validation verifies title validation for recurring templates
func TestCreateRecurringTemplate_Title_Validation(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create list for template
	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "Template Validation Test")
	require.NoError(t, err)

	t.Run("empty title is rejected", func(t *testing.T) {
		reqBody := `{
			"title": "",
			"recurrence_pattern": "daily",
			"recurrence_config": "{}"
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/recurring-templates", list.ID),
			bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"Empty title should be rejected, got: %s", w.Body.String())

		var errResp openapi.ErrorResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
		require.NotNil(t, errResp.Error)
		require.NotNil(t, errResp.Error.Code)
		assert.Equal(t, "VALIDATION_ERROR", *errResp.Error.Code)
	})

	t.Run("whitespace-only title is rejected", func(t *testing.T) {
		reqBody := `{
			"title": "   ",
			"recurrence_pattern": "daily",
			"recurrence_config": "{}"
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/recurring-templates", list.ID),
			bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"Whitespace-only title should be rejected, got: %s", w.Body.String())

		var errResp openapi.ErrorResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
		require.NotNil(t, errResp.Error)
		require.NotNil(t, errResp.Error.Code)
		assert.Equal(t, "VALIDATION_ERROR", *errResp.Error.Code)
	})

	t.Run("title over 255 characters is rejected by OpenAPI", func(t *testing.T) {
		// OpenAPI schema enforces maxLength: 255
		longTitle := make([]byte, 256)
		for i := range longTitle {
			longTitle[i] = 'a'
		}

		reqBody := fmt.Sprintf(`{
			"title": "%s",
			"recurrence_pattern": "daily",
			"recurrence_config": "{}"
		}`, string(longTitle))

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/recurring-templates", list.ID),
			bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"Title over 255 chars should be rejected by OpenAPI middleware, got: %s", w.Body.String())
	})
}

// TestCreateRecurringTemplate_RecurrencePattern_Validation verifies recurrence pattern validation
func TestCreateRecurringTemplate_RecurrencePattern_Validation(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "Pattern Validation Test")
	require.NoError(t, err)

	t.Run("invalid pattern is rejected", func(t *testing.T) {
		reqBody := `{
			"title": "Test Template",
			"recurrence_pattern": "invalid_pattern",
			"recurrence_config": "{}"
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/recurring-templates", list.ID),
			bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"Invalid recurrence pattern should be rejected by OpenAPI enum, got: %s", w.Body.String())
	})

	t.Run("all valid patterns are accepted", func(t *testing.T) {
		validPatterns := []string{
			"daily", "weekly", "biweekly", "monthly", "yearly", "quarterly", "weekdays",
		}

		for _, pattern := range validPatterns {
			reqBody := fmt.Sprintf(`{
				"title": "Test Template - %s",
				"recurrence_pattern": "%s",
				"recurrence_config": "{}"
			}`, pattern, pattern)

			req := httptest.NewRequest(http.MethodPost,
				fmt.Sprintf("/api/v1/lists/%s/recurring-templates", list.ID),
				bytes.NewReader([]byte(reqBody)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+ts.APIKey)

			w := httptest.NewRecorder()
			ts.Router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusCreated, w.Code,
				"Pattern '%s' should be accepted, got: %s", pattern, w.Body.String())
		}
	})
}

// TestCreateRecurringTemplate_HorizonDays_Validation verifies horizon days validation
func TestCreateRecurringTemplate_HorizonDays_Validation(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "Horizon Validation Test")
	require.NoError(t, err)

	t.Run("negative sync_horizon_days is rejected", func(t *testing.T) {
		reqBody := `{
			"title": "Test Template",
			"recurrence_pattern": "daily",
			"recurrence_config": "{}",
			"sync_horizon_days": -1
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/recurring-templates", list.ID),
			bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"Negative sync_horizon_days should be rejected, got: %s", w.Body.String())

		var errResp openapi.ErrorResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
		require.NotNil(t, errResp.Error)
		require.NotNil(t, errResp.Error.Code)
		assert.Equal(t, "VALIDATION_ERROR", *errResp.Error.Code)
	})

	t.Run("sync_horizon_days over 90 is rejected", func(t *testing.T) {
		reqBody := `{
			"title": "Test Template",
			"recurrence_pattern": "daily",
			"recurrence_config": "{}",
			"sync_horizon_days": 91
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/recurring-templates", list.ID),
			bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"sync_horizon_days > 90 should be rejected, got: %s", w.Body.String())

		var errResp openapi.ErrorResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
		require.NotNil(t, errResp.Error)
		require.NotNil(t, errResp.Error.Code)
		assert.Equal(t, "VALIDATION_ERROR", *errResp.Error.Code)
	})

	t.Run("generation_horizon_days less than 30 is rejected", func(t *testing.T) {
		reqBody := `{
			"title": "Test Template",
			"recurrence_pattern": "daily",
			"recurrence_config": "{}",
			"generation_horizon_days": 29
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/recurring-templates", list.ID),
			bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"generation_horizon_days < 30 should be rejected, got: %s", w.Body.String())

		var errResp openapi.ErrorResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
		require.NotNil(t, errResp.Error)
		require.NotNil(t, errResp.Error.Code)
		assert.Equal(t, "VALIDATION_ERROR", *errResp.Error.Code)
	})

	t.Run("generation_horizon_days over 730 is rejected", func(t *testing.T) {
		reqBody := `{
			"title": "Test Template",
			"recurrence_pattern": "daily",
			"recurrence_config": "{}",
			"generation_horizon_days": 731
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/recurring-templates", list.ID),
			bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"generation_horizon_days > 730 should be rejected, got: %s", w.Body.String())

		var errResp openapi.ErrorResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
		require.NotNil(t, errResp.Error)
		require.NotNil(t, errResp.Error.Code)
		assert.Equal(t, "VALIDATION_ERROR", *errResp.Error.Code)
	})
}

// TestCreateRecurringTemplate_Priority_Validation verifies priority validation
func TestCreateRecurringTemplate_Priority_Validation(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "Priority Validation Test")
	require.NoError(t, err)

	t.Run("invalid priority is rejected", func(t *testing.T) {
		reqBody := `{
			"title": "Test Template",
			"recurrence_pattern": "daily",
			"recurrence_config": "{}",
			"priority": "invalid_priority"
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/recurring-templates", list.ID),
			bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"Invalid priority should be rejected by OpenAPI enum, got: %s", w.Body.String())
	})
}

// TestCreateRecurringTemplate_RecurrenceConfig_Validation verifies recurrence config validation
func TestCreateRecurringTemplate_RecurrenceConfig_Validation(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "Config Validation Test")
	require.NoError(t, err)

	t.Run("invalid JSON in recurrence_config is rejected", func(t *testing.T) {
		reqBody := `{
			"title": "Test Template",
			"recurrence_pattern": "daily",
			"recurrence_config": "not a json object"
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/recurring-templates", list.ID),
			bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"Invalid JSON for recurrence_config should be rejected, got: %s", w.Body.String())
	})

	t.Run("empty object for recurrence_config is accepted", func(t *testing.T) {
		reqBody := `{
			"title": "Test Template",
			"recurrence_pattern": "daily",
			"recurrence_config": "{}"
		}`

		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/lists/%s/recurring-templates", list.ID),
			bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code,
			"Empty recurrence_config object should be accepted, got: %s", w.Body.String())
	})
}
