package integration_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/rezkam/mono/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestListTasks_NegativeOffset verifies that a crafted page token encoding a negative
// offset returns InvalidArgument (400) instead of Internal (500).
//
// The decodePageToken function validates that offset >= 0 and returns an error
// for negative values, which the service layer maps to InvalidArgument.
func TestListTasks_NegativeOffset(t *testing.T) {
	pgURL := os.Getenv("TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("TEST_POSTGRES_URL not set, skipping PostgreSQL tests")
	}

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	// Cleanup
	defer func() {
		db, err := sql.Open("pgx", pgURL)
		if err == nil {
			db.Exec("TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
			db.Close()
		}
	}()

	// Create service
	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Negative Offset Test",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a few items so the list isn't empty
	for i := 0; i < 3; i++ {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)
		item := &domain.TodoItem{
			ID:         itemUUID.String(),
			Title:      fmt.Sprintf("Task %d", i),
			Status:     domain.TaskStatusTodo,
			CreateTime: time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}
		err = store.CreateItem(ctx, listID, item)
		require.NoError(t, err)
	}

	testCases := []struct {
		name          string
		pageToken     string
		expectedError codes.Code
		description   string
	}{
		{
			name:          "negative_offset_minus_5",
			pageToken:     base64.StdEncoding.EncodeToString([]byte("-5")),
			expectedError: codes.InvalidArgument,
			description:   "Page token encoding -5 should return InvalidArgument",
		},
		{
			name:          "negative_offset_minus_1000",
			pageToken:     base64.StdEncoding.EncodeToString([]byte("-1000")),
			expectedError: codes.InvalidArgument,
			description:   "Page token encoding -1000 should return InvalidArgument",
		},
		{
			name:          "negative_offset_minus_max_int",
			pageToken:     base64.StdEncoding.EncodeToString([]byte("-2147483648")),
			expectedError: codes.InvalidArgument,
			description:   "Page token encoding min int32 should return InvalidArgument",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &monov1.ListTasksRequest{
				Parent:    listID,
				PageToken: tc.pageToken,
				PageSize:  10,
			}

			_, err := svc.ListTasks(ctx, req)
			require.Error(t, err, tc.description)

			st, ok := status.FromError(err)
			require.True(t, ok, "Error should be a gRPC status error")
			assert.Equal(t, tc.expectedError, st.Code(),
				"Expected %s but got %s: %s", tc.expectedError, st.Code(), tc.description)
		})
	}
}

// TestListTasks_OverflowOffset verifies that a crafted page token encoding an offset
// that exceeds int32 max returns InvalidArgument (400) instead of causing overflow.
//
// The decodePageToken function validates that offset <= int32 max and returns
// an error for values that exceed this limit.
func TestListTasks_OverflowOffset(t *testing.T) {
	pgURL := os.Getenv("TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("TEST_POSTGRES_URL not set, skipping PostgreSQL tests")
	}

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	// Cleanup
	defer func() {
		db, err := sql.Open("pgx", pgURL)
		if err == nil {
			db.Exec("TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
			db.Close()
		}
	}()

	// Create service
	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Overflow Offset Test",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	testCases := []struct {
		name          string
		pageToken     string
		expectedError codes.Code
		description   string
	}{
		{
			name:          "overflow_int32_max_plus_1",
			pageToken:     base64.StdEncoding.EncodeToString([]byte("2147483648")), // int32 max + 1
			expectedError: codes.InvalidArgument,
			description:   "Page token encoding int32_max+1 should return InvalidArgument",
		},
		{
			name:          "overflow_large_number",
			pageToken:     base64.StdEncoding.EncodeToString([]byte("9999999999999")),
			expectedError: codes.InvalidArgument,
			description:   "Page token encoding very large number should return InvalidArgument",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &monov1.ListTasksRequest{
				Parent:    listID,
				PageToken: tc.pageToken,
				PageSize:  10,
			}

			_, err := svc.ListTasks(ctx, req)
			require.Error(t, err, tc.description)

			st, ok := status.FromError(err)
			require.True(t, ok, "Error should be a gRPC status error")
			assert.Equal(t, tc.expectedError, st.Code(),
				"Expected %s but got %s: %s", tc.expectedError, st.Code(), tc.description)
		})
	}
}

// TestListTasks_InvalidPageToken verifies that malformed page tokens return InvalidArgument.
func TestListTasks_InvalidPageToken(t *testing.T) {
	pgURL := os.Getenv("TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("TEST_POSTGRES_URL not set, skipping PostgreSQL tests")
	}

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	// Cleanup
	defer func() {
		db, err := sql.Open("pgx", pgURL)
		if err == nil {
			db.Exec("TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
			db.Close()
		}
	}()

	// Create service
	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Invalid Token Test",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	testCases := []struct {
		name          string
		pageToken     string
		expectedError codes.Code
		description   string
	}{
		{
			name:          "not_base64",
			pageToken:     "!!!not-valid-base64!!!",
			expectedError: codes.InvalidArgument,
			description:   "Invalid base64 should return InvalidArgument",
		},
		{
			name:          "not_a_number",
			pageToken:     base64.StdEncoding.EncodeToString([]byte("not-a-number")),
			expectedError: codes.InvalidArgument,
			description:   "Non-numeric content should return InvalidArgument",
		},
		{
			name:          "whitespace_only",
			pageToken:     base64.StdEncoding.EncodeToString([]byte("   ")),
			expectedError: codes.InvalidArgument,
			description:   "Whitespace-only content should return InvalidArgument",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &monov1.ListTasksRequest{
				Parent:    listID,
				PageToken: tc.pageToken,
				PageSize:  10,
			}

			_, err := svc.ListTasks(ctx, req)
			require.Error(t, err, tc.description)

			st, ok := status.FromError(err)
			require.True(t, ok, "Error should be a gRPC status error")
			assert.Equal(t, tc.expectedError, st.Code(),
				"Expected %s but got %s: %s", tc.expectedError, st.Code(), tc.description)
		})
	}
}

// TestListTasks_ValidOffset verifies that valid offsets work correctly.
func TestListTasks_ValidOffset(t *testing.T) {
	pgURL := os.Getenv("TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("TEST_POSTGRES_URL not set, skipping PostgreSQL tests")
	}

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	// Cleanup
	defer func() {
		db, err := sql.Open("pgx", pgURL)
		if err == nil {
			db.Exec("TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
			db.Close()
		}
	}()

	// Create service
	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Valid Offset Test",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create 10 items
	for i := 0; i < 10; i++ {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)
		item := &domain.TodoItem{
			ID:         itemUUID.String(),
			Title:      fmt.Sprintf("Task %d", i),
			Status:     domain.TaskStatusTodo,
			CreateTime: time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}
		err = store.CreateItem(ctx, listID, item)
		require.NoError(t, err)
	}

	t.Run("offset_zero_is_valid", func(t *testing.T) {
		req := &monov1.ListTasksRequest{
			Parent:    listID,
			PageToken: base64.StdEncoding.EncodeToString([]byte("0")),
			PageSize:  5,
		}

		resp, err := svc.ListTasks(ctx, req)
		require.NoError(t, err)
		assert.Len(t, resp.Items, 5, "Should return 5 items from offset 0")
	})

	t.Run("offset_5_returns_remaining_items", func(t *testing.T) {
		req := &monov1.ListTasksRequest{
			Parent:    listID,
			PageToken: base64.StdEncoding.EncodeToString([]byte("5")),
			PageSize:  10,
		}

		resp, err := svc.ListTasks(ctx, req)
		require.NoError(t, err)
		assert.Len(t, resp.Items, 5, "Should return 5 items from offset 5")
	})

	t.Run("large_valid_offset_returns_empty", func(t *testing.T) {
		// 1000 is a valid int32 offset, just larger than our dataset
		req := &monov1.ListTasksRequest{
			Parent:    listID,
			PageToken: base64.StdEncoding.EncodeToString([]byte("1000")),
			PageSize:  10,
		}

		resp, err := svc.ListTasks(ctx, req)
		require.NoError(t, err)
		assert.Len(t, resp.Items, 0, "Should return 0 items for offset beyond dataset")
	})
}
