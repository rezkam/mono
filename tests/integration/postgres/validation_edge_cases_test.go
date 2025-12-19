package integration

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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

// === Empty/Null Required Fields Tests ===

// TestValidation_EmptyTitle_CreateList verifies that creating a list with empty title fails.
func TestValidation_EmptyTitle_CreateList(t *testing.T) {
	pgURL := GetTestStorageDSN(t)
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Attempt to create list with empty title
	req := &monov1.CreateListRequest{
		Title: "",
	}

	_, err = svc.CreateList(ctx, req)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code(), "Empty title should return InvalidArgument")
}

// TestValidation_EmptyTitle_CreateItem verifies that creating an item with empty title fails.
func TestValidation_EmptyTitle_CreateItem(t *testing.T) {
	pgURL := GetTestStorageDSN(t)
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list first
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Test List",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Attempt to create item with empty title
	req := &monov1.CreateItemRequest{
		ListId: listID,
		Title:  "", // Empty title
	}

	_, err = svc.CreateItem(ctx, req)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code(), "Empty title should return InvalidArgument")
}

// TestValidation_EmptyItemId_UpdateItem verifies that updating with empty item ID fails.
func TestValidation_EmptyItemId_UpdateItem(t *testing.T) {
	pgURL := GetTestStorageDSN(t)
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Attempt to update with empty item ID
	req := &monov1.UpdateItemRequest{
		ListId: "some-list-id",
		Item: &monov1.TodoItem{
			Id:    "", // Empty ID
			Title: "Updated Title",
		},
	}

	_, err = svc.UpdateItem(ctx, req)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code(), "Empty item ID should return InvalidArgument")
}

// === Invalid Timezone Tests ===

// TestValidation_InvalidTimezone_CreateItem verifies that invalid timezone format is rejected.
func TestValidation_InvalidTimezone_CreateItem(t *testing.T) {
	pgURL := GetTestStorageDSN(t)
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list first
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Timezone Test List",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	testCases := []struct {
		name     string
		timezone string
	}{
		{"invalid_format", "Invalid/Timezone"},
		{"not_iana", "Not_A_Real_Zone"},
		{"utc_offset_invalid", "UTC+99"},
		{"random_string", "this-is-not-a-timezone"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &monov1.CreateItemRequest{
				ListId:   listID,
				Title:    "Item with Invalid Timezone",
				Timezone: tc.timezone,
			}

			_, err = svc.CreateItem(ctx, req)
			require.Error(t, err, "Invalid timezone %s should be rejected", tc.timezone)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.InvalidArgument, st.Code(),
				"Invalid timezone should return InvalidArgument")
			assert.Contains(t, st.Message(), "timezone",
				"Error message should mention timezone")
		})
	}
}

// TestValidation_InvalidTimezone_UpdateItem verifies that invalid timezone is rejected during update.
func TestValidation_InvalidTimezone_UpdateItem(t *testing.T) {
	pgURL := GetTestStorageDSN(t)
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list and item first
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Timezone Update Test List",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	item := &domain.TodoItem{
		ID:         itemID,
		Title:      "Test Item",
		Status:     domain.TaskStatusTodo,
		CreateTime: time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	err = store.CreateItem(ctx, listID, item)
	require.NoError(t, err)

	// Attempt to update with invalid timezone
	req := &monov1.UpdateItemRequest{
		ListId: listID,
		Item: &monov1.TodoItem{
			Id:       itemID,
			Timezone: "Not_A_Valid_Timezone",
		},
	}

	_, err = svc.UpdateItem(ctx, req)
	require.Error(t, err, "Invalid timezone should be rejected")
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

// === Very Large Data Tests ===

// TestValidation_VeryLargeTitle verifies that 10K character titles are handled correctly.
func TestValidation_VeryLargeTitle(t *testing.T) {
	pgURL := GetTestStorageDSN(t)
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Large Data Test List",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create item with 10K character title
	largeTitle := strings.Repeat("A", 10000)
	req := &monov1.CreateItemRequest{
		ListId: listID,
		Title:  largeTitle,
	}

	resp, err := svc.CreateItem(ctx, req)
	require.NoError(t, err, "10K character title should be accepted")
	assert.Equal(t, largeTitle, resp.Item.Title)

	// Verify it's stored correctly
	fetchedItem, err := store.FindItemByID(ctx, resp.Item.Id)
	require.NoError(t, err)
	assert.Equal(t, 10000, len(fetchedItem.Title))
	assert.Equal(t, largeTitle, fetchedItem.Title)
}

// TestValidation_ManyTags verifies that 1000 tags can be stored and retrieved.
func TestValidation_ManyTags(t *testing.T) {
	pgURL := GetTestStorageDSN(t)
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Many Tags Test List",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create 1000 tags
	manyTags := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		manyTags[i] = fmt.Sprintf("tag-%04d", i)
	}

	req := &monov1.CreateItemRequest{
		ListId: listID,
		Title:  "Item with 1000 tags",
		Tags:   manyTags,
	}

	resp, err := svc.CreateItem(ctx, req)
	require.NoError(t, err, "1000 tags should be accepted")
	assert.Len(t, resp.Item.Tags, 1000)

	// Verify all tags are stored correctly
	fetchedItem, err := store.FindItemByID(ctx, resp.Item.Id)
	require.NoError(t, err)
	assert.Len(t, fetchedItem.Tags, 1000)

	// Verify tags are in order
	for i := 0; i < 1000; i++ {
		assert.Equal(t, fmt.Sprintf("tag-%04d", i), fetchedItem.Tags[i])
	}
}

// TestValidation_VeryLongTagNames verifies handling of extremely long individual tag names.
func TestValidation_VeryLongTagNames(t *testing.T) {
	pgURL := GetTestStorageDSN(t)
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Long Tag Names Test List",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create tags with very long names (1K characters each)
	longTags := []string{
		strings.Repeat("tag1-", 200), // 1000 chars
		strings.Repeat("tag2-", 200), // 1000 chars
		strings.Repeat("tag3-", 200), // 1000 chars
	}

	req := &monov1.CreateItemRequest{
		ListId: listID,
		Title:  "Item with very long tag names",
		Tags:   longTags,
	}

	resp, err := svc.CreateItem(ctx, req)
	require.NoError(t, err, "Long tag names should be accepted")
	assert.Len(t, resp.Item.Tags, 3)

	// Verify long tags are stored correctly
	fetchedItem, err := store.FindItemByID(ctx, resp.Item.Id)
	require.NoError(t, err)
	assert.Len(t, fetchedItem.Tags, 3)
	for i, tag := range fetchedItem.Tags {
		assert.Equal(t, 1000, len(tag), "Tag %d should have 1000 characters", i)
		assert.Equal(t, longTags[i], tag)
	}
}

// TestValidation_CombinedLargeData verifies handling of multiple large data fields together.
func TestValidation_CombinedLargeData(t *testing.T) {
	pgURL := GetTestStorageDSN(t)
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Combined Large Data Test List",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Combine: 10K title + 500 tags + long tag names
	largeTitle := strings.Repeat("Title ", 2000) // ~12K chars
	manyTags := make([]string, 500)
	for i := 0; i < 500; i++ {
		manyTags[i] = strings.Repeat(fmt.Sprintf("t%d-", i), 20) // ~100 chars each
	}

	req := &monov1.CreateItemRequest{
		ListId: listID,
		Title:  largeTitle,
		Tags:   manyTags,
	}

	resp, err := svc.CreateItem(ctx, req)
	require.NoError(t, err, "Combined large data should be accepted")
	assert.Greater(t, len(resp.Item.Title), 10000)
	assert.Len(t, resp.Item.Tags, 500)

	// Verify storage
	fetchedItem, err := store.FindItemByID(ctx, resp.Item.Id)
	require.NoError(t, err)
	assert.Equal(t, largeTitle, fetchedItem.Title)
	assert.Len(t, fetchedItem.Tags, 500)
}

// === Malformed JSON in recurrence_config Tests ===

// TestValidation_MalformedJSON_RecurrenceConfig verifies that malformed JSON in recurrence_config is rejected.
func TestValidation_MalformedJSON_RecurrenceConfig(t *testing.T) {
	pgURL := GetTestStorageDSN(t)
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list first
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Recurring Template Test List",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	testCases := []struct {
		name   string
		config string
	}{
		{"invalid_json_syntax", `{"key": "value",}`}, // Trailing comma
		{"unclosed_brace", `{"key": "value"`},
		{"invalid_quotes", `{'key': 'value'}`}, // Single quotes
		{"invalid_escape", `{"key": "\x"}`},
		{"not_json", `this is not json at all`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &monov1.CreateRecurringTemplateRequest{
				ListId:            listID,
				Title:             "Weekly Task",
				RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKLY,
				RecurrenceConfig:  tc.config,
			}

			_, err := svc.CreateRecurringTemplate(ctx, req)
			require.Error(t, err, "Malformed JSON %s should be rejected", tc.config)
			st, ok := status.FromError(err)
			require.True(t, ok)
			// The error could be InvalidArgument or Internal depending on when validation occurs
			assert.Contains(t, []codes.Code{codes.InvalidArgument, codes.Internal}, st.Code(),
				"Malformed JSON should return InvalidArgument or Internal error")
		})
	}
}

// TestValidation_ValidJSON_RecurrenceConfig verifies that valid JSON in recurrence_config is accepted.
func TestValidation_ValidJSON_RecurrenceConfig(t *testing.T) {
	pgURL := GetTestStorageDSN(t)
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list first
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Valid JSON Test List",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	testCases := []struct {
		name   string
		config string
	}{
		{"simple_object", `{"day": "monday"}`},
		{"nested_object", `{"schedule": {"day": "monday", "time": "09:00"}}`},
		{"array", `{"days": ["monday", "wednesday", "friday"]}`},
		{"empty_object", `{}`},
		{"complex", `{"days": [1,2,3], "time": "09:00", "enabled": true, "count": 42}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &monov1.CreateRecurringTemplateRequest{
				ListId:            listID,
				Title:             fmt.Sprintf("Task with %s config", tc.name),
				RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKLY,
				RecurrenceConfig:  tc.config,
			}

			resp, err := svc.CreateRecurringTemplate(ctx, req)
			require.NoError(t, err, "Valid JSON %s should be accepted", tc.config)
			assert.NotEmpty(t, resp.Template.Id)

			// Verify the template was stored with the config
			template, err := store.FindRecurringTemplate(ctx, resp.Template.Id)
			require.NoError(t, err)
			assert.NotNil(t, template.RecurrenceConfig)
		})
	}
}

// TestValidation_EmptyJSON_RecurrenceConfig verifies that empty/null JSON is handled correctly.
func TestValidation_EmptyJSON_RecurrenceConfig(t *testing.T) {
	pgURL := GetTestStorageDSN(t)
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list first
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Empty JSON Test List",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Test with empty string (should be accepted - means no config)
	req := &monov1.CreateRecurringTemplateRequest{
		ListId:            listID,
		Title:             "Task with empty config",
		RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
		RecurrenceConfig:  "",
	}

	resp, err := svc.CreateRecurringTemplate(ctx, req)
	require.NoError(t, err, "Empty recurrence_config should be accepted")
	assert.NotEmpty(t, resp.Template.Id)
}
