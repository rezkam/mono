package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/http/openapi"
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
		reqBody := map[string]any{
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

	t.Run("due_at is mapped", func(t *testing.T) {
		dueTime := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
		reqBody := map[string]any{
			"title":  "Due Time Test",
			"due_at": dueTime.Format(time.RFC3339),
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
		require.NotNil(t, resp.Item.DueAt)
		assert.Equal(t, dueTime.UTC(), resp.Item.DueAt.UTC())
	})

	t.Run("tags is mapped", func(t *testing.T) {
		reqBody := map[string]any{
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
		reqBody := map[string]any{
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
		high := openapi.ItemPriority("high")
		assert.Equal(t, high, *resp.Item.Priority)
	})

	t.Run("estimated_duration is mapped", func(t *testing.T) {
		reqBody := map[string]any{
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
		assert.Equal(t, "PT1H30M", *resp.Item.EstimatedDuration, "estimated_duration should be ISO 8601 format")
	})

	t.Run("timezone is mapped", func(t *testing.T) {
		reqBody := map[string]any{
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
		// Create a recurring template first (required for instance_date validation)
		ctx := context.Background()
		template, err := ts.TodoService.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
			ListID:            listID,
			Title:             "Test Template",
			RecurrencePattern: domain.RecurrenceDaily,
			RecurrenceConfig:  map[string]any{},
		})
		require.NoError(t, err)

		instanceDate := time.Date(2025, 12, 25, 0, 0, 0, 0, time.UTC)
		reqBody := map[string]any{
			"title":                 "Instance Date Test",
			"instance_date":         instanceDate.Format(time.RFC3339),
			"recurring_template_id": template.ID,
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

	t.Run("starts_at is mapped", func(t *testing.T) {
		startsAt := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
		reqBody := map[string]any{
			"title":     "Starts At Test",
			"starts_at": startsAt.Format("2006-01-02"), // Date only format
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
		require.NotNil(t, resp.Item.StartsAt, "starts_at should be in response")
		assert.Equal(t, "2025-06-15", resp.Item.StartsAt.String())
	})

	t.Run("due_offset is mapped", func(t *testing.T) {
		reqBody := map[string]any{
			"title":      "Due Offset Test",
			"due_offset": "PT3H", // ISO 8601 duration: 3 hours
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
		require.NotNil(t, resp.Item.DueOffset, "due_offset should be in response")
		assert.Equal(t, "PT3H", *resp.Item.DueOffset, "due_offset should be ISO 8601 format")
	})

	t.Run("starts_at and due_offset together", func(t *testing.T) {
		startsAt := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
		reqBody := map[string]any{
			"title":      "Starts At + Due Offset Test",
			"starts_at":  startsAt.Format("2006-01-02"),
			"due_offset": "PT2H30M", // 2 hours 30 minutes
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
		require.NotNil(t, resp.Item.StartsAt, "starts_at should be in response")
		require.NotNil(t, resp.Item.DueOffset, "due_offset should be in response")
		assert.Equal(t, "2025-07-01", resp.Item.StartsAt.String())
		assert.Equal(t, "PT2H30M", *resp.Item.DueOffset, "due_offset should be ISO 8601 format")
	})

	t.Run("all fields together", func(t *testing.T) {
		// Create a recurring template first (required for instance_date validation)
		ctx := context.Background()
		template, err := ts.TodoService.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
			ListID:            listID,
			Title:             "Test Template for All Fields",
			RecurrencePattern: domain.RecurrenceDaily,
			RecurrenceConfig:  map[string]any{},
		})
		require.NoError(t, err)

		dueTime := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
		instanceDate := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
		startsAt := time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC)

		reqBody := map[string]any{
			"title":                 "Complete Item Test",
			"due_at":                dueTime.Format(time.RFC3339),
			"tags":                  []string{"work", "important"},
			"priority":              "urgent",
			"estimated_duration":    "PT2H45M", // ISO 8601 format
			"timezone":              "America/New_York",
			"instance_date":         instanceDate.Format(time.RFC3339),
			"recurring_template_id": template.ID,
			"starts_at":             startsAt.Format("2006-01-02"),
			"due_offset":            "PT4H",
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

		require.NotNil(t, resp.Item.DueAt)
		assert.Equal(t, dueTime.UTC(), resp.Item.DueAt.UTC())

		require.NotNil(t, resp.Item.Tags)
		assert.ElementsMatch(t, []string{"work", "important"}, *resp.Item.Tags)

		require.NotNil(t, resp.Item.Priority)
		urgent := openapi.ItemPriority("urgent")
		assert.Equal(t, urgent, *resp.Item.Priority)

		require.NotNil(t, resp.Item.EstimatedDuration)
		assert.Equal(t, "PT2H45M", *resp.Item.EstimatedDuration, "estimated_duration should be ISO 8601 format")

		require.NotNil(t, resp.Item.Timezone)
		assert.Equal(t, "America/New_York", *resp.Item.Timezone)

		require.NotNil(t, resp.Item.InstanceDate)
		assert.Equal(t, instanceDate.UTC(), resp.Item.InstanceDate.UTC())

		require.NotNil(t, resp.Item.StartsAt)
		assert.Equal(t, "2025-08-01", resp.Item.StartsAt.String())

		require.NotNil(t, resp.Item.DueOffset)
		assert.Equal(t, "PT4H", *resp.Item.DueOffset, "due_offset should be ISO 8601 format")

		// Verify auto-generated fields
		require.NotNil(t, resp.Item.Id)
		require.NotNil(t, resp.Item.Status)
		todo := openapi.ItemStatus("todo")
		assert.Equal(t, todo, *resp.Item.Status)
		require.NotNil(t, resp.Item.CreatedAt)
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
		{"ISO minutes only", "PT30M", "PT30M"},
		{"ISO hours only", "PT2H", "PT2H"},
		{"ISO hours and minutes", "PT1H30M", "PT1H30M"},
		{"ISO seconds", "PT90S", "PT1M30S"},
		{"ISO complex", "PT2H30M15S", "PT2H30M15S"},
		{"ISO fractional hours", "PT1.5H", "PT1H30M"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reqBody := map[string]any{
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
		reqBody := map[string]any{
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

	priorities := []openapi.ItemPriority{"low", "medium", "high", "urgent"}

	for _, priority := range priorities {
		t.Run(string(priority), func(t *testing.T) {
			reqBody := map[string]any{
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
		reqBody := map[string]any{
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
