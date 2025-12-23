package http_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/http/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateItem_AllFieldsMapped verifies that all OpenAPI CreateItemRequest fields
// are properly mapped to the domain model and returned in the response.
func TestCreateItem_AllFieldsMapped(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create a list to add items to
	listReq := openapi.CreateListRequest{Title: "Field Mapping Test List"}
	listBody, err := json.Marshal(listReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/lists", bytes.NewReader(listBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)
	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var listResp openapi.CreateListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	listID := listResp.List.Id.String()

	t.Run("title is mapped", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"title": "Test Title Mapping",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", listID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

		var resp openapi.CreateItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Item)
		require.NotNil(t, resp.Item.Title)
		assert.Equal(t, "Test Title Mapping", *resp.Item.Title)
	})

	t.Run("due_time is mapped", func(t *testing.T) {
		dueTime := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
		reqBody := map[string]interface{}{
			"title":    "Due Time Test",
			"due_time": dueTime.Format(time.RFC3339),
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", listID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

		var resp openapi.CreateItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Item)
		require.NotNil(t, resp.Item.DueTime)
		assert.Equal(t, dueTime.UTC(), resp.Item.DueTime.UTC())
	})

	t.Run("tags is mapped", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"title": "Tags Test",
			"tags":  []string{"urgent", "home", "errand"},
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", listID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

		var resp openapi.CreateItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Item)
		require.NotNil(t, resp.Item.Tags)
		assert.ElementsMatch(t, []string{"urgent", "home", "errand"}, *resp.Item.Tags)
	})

	t.Run("priority is mapped", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"title":    "Priority Test",
			"priority": "high",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", listID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

		var resp openapi.CreateItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Item)
		require.NotNil(t, resp.Item.Priority)
		assert.Equal(t, openapi.High, *resp.Item.Priority)
	})

	t.Run("estimated_duration is mapped", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"title":              "Estimated Duration Test",
			"estimated_duration": "PT1H30M", // ISO 8601 format as documented in OpenAPI
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", listID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

		var resp openapi.CreateItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Item)
		require.NotNil(t, resp.Item.EstimatedDuration, "estimated_duration should be in response")
		assert.Equal(t, "1h30m0s", *resp.Item.EstimatedDuration)
	})

	t.Run("timezone is mapped", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"title":    "Timezone Test",
			"timezone": "Europe/Stockholm",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", listID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

		var resp openapi.CreateItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Item)
		require.NotNil(t, resp.Item.Timezone)
		assert.Equal(t, "Europe/Stockholm", *resp.Item.Timezone)
	})

	t.Run("instance_date is mapped", func(t *testing.T) {
		instanceDate := time.Date(2025, 12, 25, 0, 0, 0, 0, time.UTC)
		reqBody := map[string]interface{}{
			"title":         "Instance Date Test",
			"instance_date": instanceDate.Format(time.RFC3339),
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", listID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

		var resp openapi.CreateItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Item)
		require.NotNil(t, resp.Item.InstanceDate)
		assert.Equal(t, instanceDate.UTC(), resp.Item.InstanceDate.UTC())
	})

	t.Run("all fields together", func(t *testing.T) {
		dueTime := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
		instanceDate := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)

		reqBody := map[string]interface{}{
			"title":              "Complete Item Test",
			"due_time":           dueTime.Format(time.RFC3339),
			"tags":               []string{"work", "important"},
			"priority":           "urgent",
			"estimated_duration": "PT2H45M", // ISO 8601 format
			"timezone":           "America/New_York",
			"instance_date":      instanceDate.Format(time.RFC3339),
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", listID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

		var resp openapi.CreateItemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Item)

		// Verify all fields
		require.NotNil(t, resp.Item.Title)
		assert.Equal(t, "Complete Item Test", *resp.Item.Title)

		require.NotNil(t, resp.Item.DueTime)
		assert.Equal(t, dueTime.UTC(), resp.Item.DueTime.UTC())

		require.NotNil(t, resp.Item.Tags)
		assert.ElementsMatch(t, []string{"work", "important"}, *resp.Item.Tags)

		require.NotNil(t, resp.Item.Priority)
		assert.Equal(t, openapi.Urgent, *resp.Item.Priority)

		require.NotNil(t, resp.Item.EstimatedDuration)
		assert.Equal(t, "2h45m0s", *resp.Item.EstimatedDuration)

		require.NotNil(t, resp.Item.Timezone)
		assert.Equal(t, "America/New_York", *resp.Item.Timezone)

		require.NotNil(t, resp.Item.InstanceDate)
		assert.Equal(t, instanceDate.UTC(), resp.Item.InstanceDate.UTC())

		// Verify auto-generated fields
		require.NotNil(t, resp.Item.Id)
		require.NotNil(t, resp.Item.Status)
		assert.Equal(t, openapi.Todo, *resp.Item.Status)
		require.NotNil(t, resp.Item.CreateTime)
		require.NotNil(t, resp.Item.UpdatedAt)
	})
}

// TestCreateItem_EstimatedDurationFormats tests various duration format inputs
func TestCreateItem_EstimatedDurationFormats(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create a list
	listReq := openapi.CreateListRequest{Title: "Duration Format Test List"}
	listBody, _ := json.Marshal(listReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lists", bytes.NewReader(listBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)
	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var listResp openapi.CreateListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	listID := listResp.List.Id.String()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		// ISO 8601 format (documented in OpenAPI spec)
		{"ISO minutes only", "PT30M", "30m0s"},
		{"ISO hours only", "PT2H", "2h0m0s"},
		{"ISO hours and minutes", "PT1H30M", "1h30m0s"},
		{"ISO seconds", "PT90S", "1m30s"},
		{"ISO complex", "PT2H30M15S", "2h30m15s"},
		{"ISO fractional hours", "PT1.5H", "1h30m0s"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reqBody := map[string]interface{}{
				"title":              fmt.Sprintf("Duration %s", tc.name),
				"estimated_duration": tc.input,
			}
			body, _ := json.Marshal(reqBody)

			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", listID), bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+ts.APIKey)

			w := httptest.NewRecorder()
			ts.Router.ServeHTTP(w, req)

			require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

			var resp openapi.CreateItemResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.NotNil(t, resp.Item)
			require.NotNil(t, resp.Item.EstimatedDuration)
			assert.Equal(t, tc.expected, *resp.Item.EstimatedDuration)
		})
	}

	t.Run("invalid duration returns error", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"title":              "Invalid Duration",
			"estimated_duration": "invalid",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", listID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// TestCreateItem_PriorityValues tests all valid priority values
func TestCreateItem_PriorityValues(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create a list
	listReq := openapi.CreateListRequest{Title: "Priority Test List"}
	listBody, _ := json.Marshal(listReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lists", bytes.NewReader(listBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)
	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var listResp openapi.CreateListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	listID := listResp.List.Id.String()

	priorities := []openapi.TaskPriority{openapi.Low, openapi.Medium, openapi.High, openapi.Urgent}

	for _, priority := range priorities {
		t.Run(string(priority), func(t *testing.T) {
			reqBody := map[string]interface{}{
				"title":    fmt.Sprintf("Priority %s", priority),
				"priority": string(priority),
			}
			body, _ := json.Marshal(reqBody)

			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", listID), bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+ts.APIKey)

			w := httptest.NewRecorder()
			ts.Router.ServeHTTP(w, req)

			require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

			var resp openapi.CreateItemResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.NotNil(t, resp.Item)
			require.NotNil(t, resp.Item.Priority)
			assert.Equal(t, priority, *resp.Item.Priority)
		})
	}

	t.Run("invalid priority returns error", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"title":    "Invalid Priority",
			"priority": "super_urgent",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/lists/%s/items", listID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)

		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
