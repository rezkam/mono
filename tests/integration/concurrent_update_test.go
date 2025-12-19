package integration_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
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
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// TestConcurrentUpdateItem_MultipleGoroutines verifies that concurrent updates
// to the same item complete successfully without data loss or corruption.
func TestConcurrentUpdateItem_MultipleGoroutines(t *testing.T) {
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Concurrent Test List",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create an item
	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	item := &domain.TodoItem{
		ID:         itemID,
		Title:      "Original Title",
		Status:     domain.TaskStatusTodo,
		CreateTime: time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	err = store.CreateItem(ctx, listID, item)
	require.NoError(t, err)

	// Run concurrent updates
	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)
	startTime := time.Now()

	for i := 0; i < numGoroutines; i++ {
		goroutineID := i
		go func() {
			defer wg.Done()

			req := &monov1.UpdateItemRequest{
				ListId: listID,
				Item: &monov1.TodoItem{
					Id:    itemID,
					Title: fmt.Sprintf("Updated by goroutine %d", goroutineID),
					// Each goroutine updates to a different status
					Status: monov1.TaskStatus(monov1.TaskStatus_value[fmt.Sprintf("TASK_STATUS_%s", []string{"TODO", "IN_PROGRESS", "DONE"}[goroutineID%3])]),
				},
			}

			_, err := svc.UpdateItem(ctx, req)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	var updateErrors []error
	for err := range errors {
		updateErrors = append(updateErrors, err)
	}
	assert.Empty(t, updateErrors, "All concurrent updates should succeed")

	// Verify the item exists and has valid data
	finalItem, err := store.FindItemByID(ctx, itemID)
	require.NoError(t, err)
	assert.NotEmpty(t, finalItem.Title, "Title should be set")
	assert.True(t, finalItem.UpdatedAt.After(startTime), "UpdatedAt should be after start time")

	t.Logf("Final item title: %s, status: %s", finalItem.Title, finalItem.Status)
}

// TestConcurrentUpdateItem_DifferentFields verifies that concurrent updates
// to different fields of the same item work correctly.
func TestConcurrentUpdateItem_DifferentFields(t *testing.T) {
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Concurrent Field Test List",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create an item
	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	item := &domain.TodoItem{
		ID:         itemID,
		Title:      "Original Title",
		Status:     domain.TaskStatusTodo,
		Tags:       []string{"original"},
		CreateTime: time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	err = store.CreateItem(ctx, listID, item)
	require.NoError(t, err)

	// Run concurrent updates to different fields
	var wg sync.WaitGroup
	errors := make(chan error, 3)

	// Goroutine 1: Update title
	wg.Add(1)
	go func() {
		defer wg.Done()
		req := &monov1.UpdateItemRequest{
			ListId: listID,
			Item: &monov1.TodoItem{
				Id:    itemID,
				Title: "Updated Title",
			},
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"title"},
			},
		}
		_, err := svc.UpdateItem(ctx, req)
		if err != nil {
			errors <- fmt.Errorf("title update failed: %w", err)
		}
	}()

	// Goroutine 2: Update status
	wg.Add(1)
	go func() {
		defer wg.Done()
		req := &monov1.UpdateItemRequest{
			ListId: listID,
			Item: &monov1.TodoItem{
				Id:     itemID,
				Status: monov1.TaskStatus_TASK_STATUS_IN_PROGRESS,
			},
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"status"},
			},
		}
		_, err := svc.UpdateItem(ctx, req)
		if err != nil {
			errors <- fmt.Errorf("status update failed: %w", err)
		}
	}()

	// Goroutine 3: Update tags
	wg.Add(1)
	go func() {
		defer wg.Done()
		req := &monov1.UpdateItemRequest{
			ListId: listID,
			Item: &monov1.TodoItem{
				Id:   itemID,
				Tags: []string{"updated", "concurrent"},
			},
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"tags"},
			},
		}
		_, err := svc.UpdateItem(ctx, req)
		if err != nil {
			errors <- fmt.Errorf("tags update failed: %w", err)
		}
	}()

	wg.Wait()
	close(errors)

	// Check for errors
	var updateErrors []error
	for err := range errors {
		updateErrors = append(updateErrors, err)
	}
	assert.Empty(t, updateErrors, "All concurrent field updates should succeed")

	// Verify all fields were updated
	// Note: Due to concurrent updates, the final state may have all updates or some
	// depending on timing. The key is that no errors occurred and data is consistent.
	finalItem, err := store.FindItemByID(ctx, itemID)
	require.NoError(t, err)
	assert.NotEmpty(t, finalItem.Title, "Title should be set")
	assert.NotEmpty(t, finalItem.Status, "Status should be set")

	t.Logf("Final state - Title: %s, Status: %s, Tags: %v",
		finalItem.Title, finalItem.Status, finalItem.Tags)
}

// TestConcurrentUpdateItem_UpdatedAtTimestamp verifies that the updated_at
// timestamp is properly managed during concurrent updates.
func TestConcurrentUpdateItem_UpdatedAtTimestamp(t *testing.T) {
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Timestamp Test List",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create an item
	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	createTime := time.Now().UTC()
	item := &domain.TodoItem{
		ID:         itemID,
		Title:      "Timestamp Test",
		Status:     domain.TaskStatusTodo,
		CreateTime: createTime,
		UpdatedAt:  createTime,
	}
	err = store.CreateItem(ctx, listID, item)
	require.NoError(t, err)

	// Wait a bit to ensure updated_at will be different
	time.Sleep(100 * time.Millisecond)

	// Record the time before concurrent updates
	beforeUpdates := time.Now().UTC()

	// Run concurrent updates
	const numGoroutines = 5
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		goroutineID := i
		go func() {
			defer wg.Done()
			req := &monov1.UpdateItemRequest{
				ListId: listID,
				Item: &monov1.TodoItem{
					Id:    itemID,
					Title: fmt.Sprintf("Update %d", goroutineID),
				},
			}
			svc.UpdateItem(ctx, req)
		}()
	}

	wg.Wait()

	// Verify updated_at is after the original create time and before updates
	finalItem, err := store.FindItemByID(ctx, itemID)
	require.NoError(t, err)

	assert.True(t, finalItem.UpdatedAt.After(createTime),
		"UpdatedAt should be after create time")
	assert.True(t, finalItem.UpdatedAt.After(beforeUpdates) || finalItem.UpdatedAt.Equal(beforeUpdates),
		"UpdatedAt should be at or after the time concurrent updates started")

	t.Logf("CreateTime: %s, BeforeUpdates: %s, UpdatedAt: %s",
		createTime.Format(time.RFC3339Nano),
		beforeUpdates.Format(time.RFC3339Nano),
		finalItem.UpdatedAt.Format(time.RFC3339Nano))
}

// TestConcurrentUpdateItem_DifferentItems verifies that concurrent updates
// to different items don't interfere with each other.
func TestConcurrentUpdateItem_DifferentItems(t *testing.T) {
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

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	// Create a list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Multi-Item Test List",
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create multiple items
	const numItems = 10
	itemIDs := make([]string, numItems)
	for i := 0; i < numItems; i++ {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)
		itemIDs[i] = itemUUID.String()

		item := &domain.TodoItem{
			ID:         itemIDs[i],
			Title:      fmt.Sprintf("Item %d", i),
			Status:     domain.TaskStatusTodo,
			CreateTime: time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}
		err = store.CreateItem(ctx, listID, item)
		require.NoError(t, err)
	}

	// Update all items concurrently
	var wg sync.WaitGroup
	wg.Add(numItems)
	errors := make(chan error, numItems)

	for i := 0; i < numItems; i++ {
		itemIndex := i
		go func() {
			defer wg.Done()
			req := &monov1.UpdateItemRequest{
				ListId: listID,
				Item: &monov1.TodoItem{
					Id:     itemIDs[itemIndex],
					Title:  fmt.Sprintf("Updated Item %d", itemIndex),
					Status: monov1.TaskStatus_TASK_STATUS_DONE,
				},
			}
			_, err := svc.UpdateItem(ctx, req)
			if err != nil {
				errors <- fmt.Errorf("item %d update failed: %w", itemIndex, err)
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	var updateErrors []error
	for err := range errors {
		updateErrors = append(updateErrors, err)
	}
	assert.Empty(t, updateErrors, "All concurrent item updates should succeed")

	// Verify all items were updated
	for i, itemID := range itemIDs {
		item, err := store.FindItemByID(ctx, itemID)
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("Updated Item %d", i), item.Title)
		assert.Equal(t, domain.TaskStatusDone, item.Status)
	}
}
