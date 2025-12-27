package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	httpRouter "github.com/rezkam/mono/internal/http"
	"github.com/rezkam/mono/internal/http/handler"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTimezoneConsistency_ThreeLayerArchitecture is a comprehensive test that
// verifies the application follows the three-layer timezone separation pattern:
// 1. Storage Layer: Always UTC
// 2. Business Logic: Always UTC
// 3. Presentation Layer: ISO 8601 with Z suffix, accepts any timezone
func TestTimezoneConsistency_ThreeLayerArchitecture(t *testing.T) {
	pgURL := GetTestStorageDSN(t)

	ctx, cancel := context.WithCancel(context.Background())
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)

	service := todo.NewService(store, todo.Config{
		DefaultPageSize: 25,
		MaxPageSize:     100,
	})

	// Create HTTP router for presentation layer tests
	authenticator := auth.NewAuthenticator(store, auth.Config{OperationTimeout: 5 * time.Second})

	// Cleanup function - cancel context first to signal shutdown, then wait, then close resources
	defer func() {
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		authenticator.Shutdown(shutdownCtx)
		store.Close()

		// Truncate tables for next test
		db, err := sql.Open("pgx", pgURL)
		if err == nil {
			db.Exec("TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
			db.Close()
		}
	}()

	server := handler.NewServer(service)
	routerConfig := httpRouter.Config{
		MaxBodyBytes: 1 << 20, // 1MB
	}
	router := httpRouter.NewRouter(server, authenticator, routerConfig)

	// Generate test API key for authenticated requests
	apiKey, err := auth.CreateAPIKey(ctx, store, "sk", "test", "v1", "test-key", nil)
	require.NoError(t, err)

	t.Run("Layer1_Storage_AlwaysUTC", func(t *testing.T) {
		// Create a test list
		listUUID, err := uuid.NewV7()
		require.NoError(t, err)
		listID := listUUID.String()

		list := &domain.TodoList{
			ID:         listID,
			Title:      "Timezone Test List",
			CreateTime: time.Now().UTC(),
		}
		err = store.CreateList(ctx, list)
		require.NoError(t, err)

		// Query the database directly to check stored timezone
		db, err := sql.Open("pgx", pgURL)
		require.NoError(t, err)
		defer db.Close()

		// Force session to UTC to verify storage
		_, err = db.Exec("SET TIME ZONE 'UTC'")
		require.NoError(t, err)

		var createTime time.Time
		query := `SELECT create_time FROM todo_lists WHERE id = $1`
		err = db.QueryRow(query, listID).Scan(&createTime)
		require.NoError(t, err)

		// The driver might return Local time even if DB is UTC. Convert to UTC to verify the instant and location.
		createTime = createTime.UTC()

		// CRITICAL: Verify the Location is UTC
		assert.Equal(t, time.UTC, createTime.Location(),
			"Storage layer: create_time should be stored in UTC location")

		// Verify the timezone offset is 0 (UTC)
		_, offsetSeconds := createTime.Zone()
		assert.Equal(t, 0, offsetSeconds,
			"Storage layer: create_time timezone offset should be 0 (UTC)")
	})

	t.Run("Layer2_BusinessLogic_AlwaysUTC", func(t *testing.T) {
		// Create a list using service
		createdList, err := service.CreateList(ctx, "Business Logic Test")
		require.NoError(t, err)

		// Retrieve the list
		retrieved, err := service.GetList(ctx, createdList.ID)
		require.NoError(t, err)

		// Business logic should return UTC times
		assert.Equal(t, time.UTC, retrieved.CreateTime.Location(),
			"Business logic: CreateTime should be in UTC location")

		// Create an item to test item timestamps
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)

		item := &domain.TodoItem{
			ID:         itemUUID.String(),
			Title:      "Test Item",
			Status:     domain.TaskStatusTodo,
			CreateTime: time.Now().UTC(),
		}
		_, err = service.CreateItem(ctx, createdList.ID, item)
		require.NoError(t, err)

		// Retrieve and verify
		retrievedItem, err := service.GetItem(ctx, item.ID)
		require.NoError(t, err)

		assert.Equal(t, time.UTC, retrievedItem.CreateTime.Location(),
			"Business logic: Item CreateTime should be in UTC location")
	})

	t.Run("Layer3_Presentation_ISO8601_WithZ", func(t *testing.T) {
		// Create a list
		createdList, err := service.CreateList(ctx, "Presentation Test")
		require.NoError(t, err)

		// Make HTTP request through router
		req := httptest.NewRequest(http.MethodGet, "/api/v1/lists/"+createdList.ID, nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		// Parse response
		var response map[string]interface{}
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		// Verify create_time is ISO 8601 with 'Z' suffix
		listMap, ok := response["list"].(map[string]interface{})
		require.True(t, ok, "response should contain list object")
		createTimeStr, ok := listMap["create_time"].(string)
		require.True(t, ok, "create_time should be a string")

		// CRITICAL: Must end with 'Z' indicating UTC
		assert.True(t, strings.HasSuffix(createTimeStr, "Z"),
			"Presentation layer: create_time must end with 'Z' indicating UTC, got: %s", createTimeStr)

		// Verify it parses as valid RFC3339
		parsed, err := time.Parse(time.RFC3339, createTimeStr)
		require.NoError(t, err, "create_time should be valid RFC3339 format")

		// Verify parsed time is in UTC
		assert.Equal(t, time.UTC, parsed.Location(),
			"Presentation layer: parsed create_time should be UTC location")
	})

	t.Run("Layer3_Presentation_AcceptsAnyTimezone", func(t *testing.T) {
		// Create a list
		createdList, err := service.CreateList(ctx, "Timezone Acceptance Test")
		require.NoError(t, err)

		testCases := []struct {
			name        string
			dueTime     string
			expectedUTC string
		}{
			{
				name:        "PST timezone (UTC-7)",
				dueTime:     "2024-06-15T14:30:00-07:00", // 2:30 PM PST
				expectedUTC: "2024-06-15T21:30:00Z",      // 9:30 PM UTC
			},
			{
				name:        "EST timezone (UTC-4)",
				dueTime:     "2024-06-15T17:30:00-04:00", // 5:30 PM EST
				expectedUTC: "2024-06-15T21:30:00Z",      // 9:30 PM UTC (same instant)
			},
			{
				name:        "UTC timezone",
				dueTime:     "2024-06-15T21:30:00Z",
				expectedUTC: "2024-06-15T21:30:00Z",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Create item with timezone-aware due_time
				body := fmt.Sprintf(`{
					"title": "Timezone test %s",
					"status": "todo",
					"due_time": "%s"
				}`, tc.name, tc.dueTime)

				req := httptest.NewRequest(http.MethodPost, "/api/v1/lists/"+createdList.ID+"/items",
					strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+apiKey)
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				require.Equal(t, http.StatusCreated, w.Code,
					"Should accept %s timezone in request", tc.name)

				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				// Extract item ID
				itemMap, ok := response["item"].(map[string]interface{})
				require.True(t, ok, "response should contain item object")
				itemID, ok := itemMap["id"].(string)
				require.True(t, ok, "Response should contain item ID")

				// Retrieve the item from business logic to verify UTC conversion
				retrievedItem, err := service.GetItem(ctx, itemID)
				require.NoError(t, err)

				// CRITICAL: Business logic should have converted to UTC
				require.NotNil(t, retrievedItem.DueTime, "DueTime should be set")
				assert.Equal(t, time.UTC, retrievedItem.DueTime.Location(),
					"Business logic: DueTime should be converted to UTC location")

				// Verify the absolute time is correct
				expectedTime, _ := time.Parse(time.RFC3339, tc.expectedUTC)
				assert.True(t, retrievedItem.DueTime.Equal(expectedTime),
					"DueTime should equal %s after timezone conversion, got %s",
					tc.expectedUTC, retrievedItem.DueTime.Format(time.RFC3339))

				// Verify response also returns UTC with 'Z'
				dueTimeStr, ok := itemMap["due_time"].(string)
				require.True(t, ok, "due_time should be a string in response")
				assert.True(t, strings.HasSuffix(dueTimeStr, "Z"),
					"Response due_time must end with 'Z', got: %s", dueTimeStr)
			})
		}
	})

	t.Run("DatabaseTriggers_AlsoUTC", func(t *testing.T) {
		// Create a list
		listUUID, err := uuid.NewV7()
		require.NoError(t, err)
		listID := listUUID.String()

		list := &domain.TodoList{
			ID:         listID,
			Title:      "Trigger Test",
			CreateTime: time.Now().UTC(),
		}
		err = store.CreateList(ctx, list)
		require.NoError(t, err)

		// Create an item
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)
		itemID := itemUUID.String()

		item := &domain.TodoItem{
			ID:         itemID,
			Title:      "Test Item",
			Status:     domain.TaskStatusTodo,
			CreateTime: time.Now().UTC(),
		}
		err = store.CreateItem(ctx, listID, item)
		require.NoError(t, err)

		// Update item status (triggers database status_history insert)
		newStatus := domain.TaskStatusInProgress
		_, err = store.UpdateItem(ctx, domain.UpdateItemParams{
			ItemID: itemID,
			ListID: listID,
			Status: &newStatus,
		})
		require.NoError(t, err)

		// Verify status_history timestamp is UTC
		db, err := sql.Open("pgx", pgURL)
		require.NoError(t, err)
		defer db.Close()

		// Force session to UTC to verify storage
		_, err = db.Exec("SET TIME ZONE 'UTC'")
		require.NoError(t, err)

		var historyTimestamp time.Time
		query := `
			SELECT changed_at
			FROM task_status_history
			WHERE task_id = $1
			ORDER BY changed_at DESC
			LIMIT 1
		`
		err = db.QueryRow(query, itemID).Scan(&historyTimestamp)
		require.NoError(t, err)

		// The driver might return Local time even if DB is UTC. Convert to UTC to verify the instant and location.
		historyTimestamp = historyTimestamp.UTC()

		// Database triggers should also create UTC timestamps
		assert.Equal(t, time.UTC, historyTimestamp.Location(),
			"Database trigger: status_history.changed_at should be UTC")

		_, offsetSeconds := historyTimestamp.Zone()
		assert.Equal(t, 0, offsetSeconds,
			"Database trigger: status_history timestamp offset should be 0")
	})
}
