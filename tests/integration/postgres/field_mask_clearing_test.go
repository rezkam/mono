package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFieldMask_ClearPriority verifies that field mask can clear optional priority field.
func TestFieldMask_ClearPriority(t *testing.T) {
	pgURL := GetTestStorageDSN(t)

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

	todoService := todo.NewService(store, todo.Config{})

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Field Mask Test List",
		CreateTime: time.Now().UTC().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create an item with priority
	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	priority := domain.TaskPriorityHigh
	item := &domain.TodoItem{
		ID:         itemID,
		Title:      "Item with Priority",
		Status:     domain.TaskStatusTodo,
		Priority:   &priority,
		CreateTime: time.Now().UTC().UTC(),
		UpdatedAt:  time.Now().UTC().UTC(),
	}
	_, err = store.CreateItem(ctx, listID, item)
	require.NoError(t, err)

	// Verify priority is set
	fetchedItem, err := store.FindItemByID(ctx, itemID)
	require.NoError(t, err)
	require.NotNil(t, fetchedItem.Priority, "Priority should be set initially")
	assert.Equal(t, domain.TaskPriorityHigh, *fetchedItem.Priority)

	// Clear priority by setting it to nil
	fetchedItem.Priority = nil
	_, err = todoService.UpdateItem(ctx, ItemToUpdateParams(listID, fetchedItem))
	require.NoError(t, err)

	// Verify priority is cleared
	fetchedItem, err = store.FindItemByID(ctx, itemID)
	require.NoError(t, err)
	assert.Nil(t, fetchedItem.Priority, "Priority should be cleared after field mask update")
}

// TestFieldMask_ClearDueTime verifies that field mask can clear optional due_time field.
func TestFieldMask_ClearDueTime(t *testing.T) {
	pgURL := GetTestStorageDSN(t)

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

	todoService := todo.NewService(store, todo.Config{})

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "DueTime Test List",
		CreateTime: time.Now().UTC().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create an item with due_time
	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	dueTime := time.Now().UTC().Add(24 * time.Hour).UTC()
	item := &domain.TodoItem{
		ID:         itemID,
		Title:      "Item with DueTime",
		Status:     domain.TaskStatusTodo,
		DueTime:    &dueTime,
		CreateTime: time.Now().UTC().UTC(),
		UpdatedAt:  time.Now().UTC().UTC(),
	}
	_, err = store.CreateItem(ctx, listID, item)
	require.NoError(t, err)

	// Verify due_time is set
	fetchedItem, err := store.FindItemByID(ctx, itemID)
	require.NoError(t, err)
	require.NotNil(t, fetchedItem.DueTime, "DueTime should be set initially")

	// Clear due_time by setting it to nil
	fetchedItem.DueTime = nil
	_, err = todoService.UpdateItem(ctx, ItemToUpdateParams(listID, fetchedItem))
	require.NoError(t, err)

	// Verify due_time is cleared
	fetchedItem, err = store.FindItemByID(ctx, itemID)
	require.NoError(t, err)
	assert.Nil(t, fetchedItem.DueTime, "DueTime should be cleared after field mask update")
}

// TestFieldMask_ClearEstimatedDuration verifies that field mask can clear optional estimated_duration field.
func TestFieldMask_ClearEstimatedDuration(t *testing.T) {
	pgURL := GetTestStorageDSN(t)

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

	todoService := todo.NewService(store, todo.Config{})

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Duration Test List",
		CreateTime: time.Now().UTC().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create an item with estimated_duration
	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	duration := 2 * time.Hour
	item := &domain.TodoItem{
		ID:                itemID,
		Title:             "Item with Duration",
		Status:            domain.TaskStatusTodo,
		EstimatedDuration: &duration,
		CreateTime:        time.Now().UTC().UTC(),
		UpdatedAt:         time.Now().UTC().UTC(),
	}
	_, err = store.CreateItem(ctx, listID, item)
	require.NoError(t, err)

	// Verify estimated_duration is set
	fetchedItem, err := store.FindItemByID(ctx, itemID)
	require.NoError(t, err)
	require.NotNil(t, fetchedItem.EstimatedDuration, "EstimatedDuration should be set initially")
	assert.Equal(t, 2*time.Hour, *fetchedItem.EstimatedDuration)

	// Clear estimated_duration by setting it to nil
	fetchedItem.EstimatedDuration = nil
	_, err = todoService.UpdateItem(ctx, ItemToUpdateParams(listID, fetchedItem))
	require.NoError(t, err)

	// Verify estimated_duration is cleared
	fetchedItem, err = store.FindItemByID(ctx, itemID)
	require.NoError(t, err)
	assert.Nil(t, fetchedItem.EstimatedDuration, "EstimatedDuration should be cleared after field mask update")
}

// TestFieldMask_ClearTimezone verifies that field mask can clear optional timezone field.
func TestFieldMask_ClearTimezone(t *testing.T) {
	pgURL := GetTestStorageDSN(t)

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

	todoService := todo.NewService(store, todo.Config{})

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Timezone Test List",
		CreateTime: time.Now().UTC().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create an item with timezone
	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	timezone := "America/New_York"
	item := &domain.TodoItem{
		ID:         itemID,
		Title:      "Item with Timezone",
		Status:     domain.TaskStatusTodo,
		Timezone:   &timezone,
		CreateTime: time.Now().UTC().UTC(),
		UpdatedAt:  time.Now().UTC().UTC(),
	}
	_, err = store.CreateItem(ctx, listID, item)
	require.NoError(t, err)

	// Verify timezone is set
	fetchedItem, err := store.FindItemByID(ctx, itemID)
	require.NoError(t, err)
	require.NotNil(t, fetchedItem.Timezone, "Timezone should be set initially")
	assert.Equal(t, "America/New_York", *fetchedItem.Timezone)

	// Clear timezone by setting it to nil
	fetchedItem.Timezone = nil
	_, err = todoService.UpdateItem(ctx, ItemToUpdateParams(listID, fetchedItem))
	require.NoError(t, err)

	// Verify timezone is cleared
	fetchedItem, err = store.FindItemByID(ctx, itemID)
	require.NoError(t, err)
	assert.Nil(t, fetchedItem.Timezone, "Timezone should be cleared")
}

// TestFieldMask_ClearTags verifies that field mask can clear tags array.
func TestFieldMask_ClearTags(t *testing.T) {
	pgURL := GetTestStorageDSN(t)

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

	todoService := todo.NewService(store, todo.Config{})

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Tags Test List",
		CreateTime: time.Now().UTC().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create an item with tags
	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	item := &domain.TodoItem{
		ID:         itemID,
		Title:      "Item with Tags",
		Status:     domain.TaskStatusTodo,
		Tags:       []string{"work", "urgent", "important"},
		CreateTime: time.Now().UTC().UTC(),
		UpdatedAt:  time.Now().UTC().UTC(),
	}
	_, err = store.CreateItem(ctx, listID, item)
	require.NoError(t, err)

	// Verify tags are set
	fetchedItem, err := store.FindItemByID(ctx, itemID)
	require.NoError(t, err)
	assert.Len(t, fetchedItem.Tags, 3, "Tags should be set initially")

	// Clear tags by setting to empty array
	fetchedItem.Tags = []string{}
	_, err = todoService.UpdateItem(ctx, ItemToUpdateParams(listID, fetchedItem))
	require.NoError(t, err)

	// Verify tags are cleared
	fetchedItem, err = store.FindItemByID(ctx, itemID)
	require.NoError(t, err)
	assert.Empty(t, fetchedItem.Tags, "Tags should be empty after field mask update")
}

// TestFieldMask_PartialUpdate_DoesNotClearOtherFields verifies that updating
// one field with field mask doesn't clear other optional fields.
func TestFieldMask_PartialUpdate_DoesNotClearOtherFields(t *testing.T) {
	pgURL := GetTestStorageDSN(t)

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

	todoService := todo.NewService(store, todo.Config{})

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Partial Update Test List",
		CreateTime: time.Now().UTC().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create an item with multiple optional fields set
	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	priority := domain.TaskPriorityHigh
	dueTime := time.Now().UTC().Add(48 * time.Hour).UTC()
	duration := 3 * time.Hour
	timezone := "Europe/Stockholm"

	item := &domain.TodoItem{
		ID:                itemID,
		Title:             "Fully Populated Item",
		Status:            domain.TaskStatusTodo,
		Priority:          &priority,
		DueTime:           &dueTime,
		EstimatedDuration: &duration,
		Timezone:          &timezone,
		Tags:              []string{"test", "important"},
		CreateTime:        time.Now().UTC().UTC(),
		UpdatedAt:         time.Now().UTC().UTC(),
	}
	_, err = store.CreateItem(ctx, listID, item)
	require.NoError(t, err)

	// Update only the title by fetching, modifying title, and updating
	existingItem, err := todoService.GetItem(ctx, itemID)
	require.NoError(t, err)
	existingItem.Title = "Updated Title Only"
	_, err = todoService.UpdateItem(ctx, ItemToUpdateParams(listID, existingItem))
	require.NoError(t, err)

	// Verify only title changed, all other fields preserved
	fetchedItem, err := store.FindItemByID(ctx, itemID)
	require.NoError(t, err)

	assert.Equal(t, "Updated Title Only", fetchedItem.Title, "Title should be updated")
	assert.NotNil(t, fetchedItem.Priority, "Priority should be preserved")
	assert.Equal(t, domain.TaskPriorityHigh, *fetchedItem.Priority, "Priority value should be unchanged")
	assert.NotNil(t, fetchedItem.DueTime, "DueTime should be preserved")
	assert.NotNil(t, fetchedItem.EstimatedDuration, "EstimatedDuration should be preserved")
	assert.Equal(t, 3*time.Hour, *fetchedItem.EstimatedDuration, "Duration value should be unchanged")
	assert.NotNil(t, fetchedItem.Timezone, "Timezone should be preserved")
	assert.Equal(t, "Europe/Stockholm", *fetchedItem.Timezone, "Timezone value should be unchanged")
	assert.Len(t, fetchedItem.Tags, 2, "Tags should be preserved")
}
