package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentUpdateItem_MultipleGoroutines verifies that concurrent updates
// to the same item complete successfully without data loss or corruption.
func TestConcurrentUpdateItem_MultipleGoroutines(t *testing.T) {
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
		Title:      "Concurrent Test List",
		CreateTime: time.Now().UTC().UTC(),
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
		CreateTime: time.Now().UTC().UTC(),
		UpdatedAt:  time.Now().UTC().UTC(),
	}
	err = store.CreateItem(ctx, listID, item)
	require.NoError(t, err)

	// Run concurrent updates
	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)
	startTime := time.Now().UTC()

	for i := 0; i < numGoroutines; i++ {
		goroutineID := i
		go func() {
			defer wg.Done()

			item := &domain.TodoItem{
				ID:    itemID,
				Title: fmt.Sprintf("Updated by goroutine %d", goroutineID),
				// Each goroutine updates to a different status
				Status: domain.TaskStatus([]string{"todo", "in_progress", "done"}[goroutineID%3]),
			}

			_, err := todoService.UpdateItem(ctx, ItemToUpdateParams(listID, item))
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

// TestConcurrentUpdateItem_LostUpdatePrevention verifies that concurrent updates
// to different fields of the same item are prevented via optimistic locking.
//
// Scenario:
//   - Request A updates status (todo → done)
//   - Request B updates title ("Task 1" → "Updated Task")
//   - Both start with version=1
//
// Expected behavior:
//   - One request succeeds (version increments to 2)
//   - Other request fails with ErrVersionConflict
//   - Successful update is preserved, failed request must refetch and retry
func TestConcurrentUpdateItem_LostUpdatePrevention(t *testing.T) {
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

	// Create list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()

	list := &domain.TodoList{
		ID:         listID,
		Title:      "Test List",
		CreateTime: time.Now().UTC().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create item: title="Task 1", status="todo"
	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	item := &domain.TodoItem{
		ID:         itemID,
		Title:      "Task 1",
		Status:     domain.TaskStatusTodo,
		CreateTime: time.Now().UTC().UTC(),
		UpdatedAt:  time.Now().UTC().UTC(),
	}
	err = store.CreateItem(ctx, listID, item)
	require.NoError(t, err)

	// Both requests read the same initial state (version=1)
	requestA_item, err := todoService.GetItem(ctx, itemID)
	require.NoError(t, err)
	require.Equal(t, "Task 1", requestA_item.Title)
	require.Equal(t, domain.TaskStatusTodo, requestA_item.Status)
	require.Equal(t, 1, requestA_item.Version)

	requestB_item, err := todoService.GetItem(ctx, itemID)
	require.NoError(t, err)
	require.Equal(t, 1, requestB_item.Version, "Both requests have same version")

	// Request A: Update status to "done"
	requestA_item.Status = domain.TaskStatusDone
	requestA_item.UpdatedAt = time.Now().UTC().UTC()

	// Request B: Update title to "Updated"
	requestB_item.Title = "Updated Task"
	requestB_item.UpdatedAt = time.Now().UTC().UTC()

	// Execute concurrently
	var wg sync.WaitGroup
	var errA, errB error

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errA = todoService.UpdateItem(ctx, ItemToUpdateParamsWithEtag(listID, requestA_item))
	}()

	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond) // B slightly delayed
		_, errB = todoService.UpdateItem(ctx, ItemToUpdateParamsWithEtag(listID, requestB_item))
	}()

	wg.Wait()

	// VERIFY: One succeeds, one gets version conflict
	t.Logf("Request A (status update) error: %v", errA)
	t.Logf("Request B (title update) error: %v", errB)

	// Exactly one should succeed
	if errA == nil && errB == nil {
		t.Fatal("Both updates succeeded - optimistic locking NOT working!")
	}

	if errA != nil && errB != nil {
		t.Fatal("Both updates failed - unexpected behavior")
	}

	// The failed one should be ErrVersionConflict
	if errA != nil {
		require.ErrorIs(t, errA, domain.ErrVersionConflict, "Failed update must be version conflict")
	}
	if errB != nil {
		require.ErrorIs(t, errB, domain.ErrVersionConflict, "Failed update must be version conflict")
	}

	// VERIFY: Final state preserves the successful update
	finalItem, err := store.FindItemByID(ctx, itemID)
	require.NoError(t, err)
	assert.Equal(t, 2, finalItem.Version, "Version incremented once")

	if errA == nil {
		// Request A succeeded - status should be updated
		assert.Equal(t, domain.TaskStatusDone, finalItem.Status, "Status update preserved")
		assert.Equal(t, "Task 1", finalItem.Title, "Title unchanged (B was rejected)")
		t.Log("✅ Request A (status) succeeded, Request B (title) rejected - NO LOST UPDATE")
	} else {
		// Request B succeeded - title should be updated
		assert.Equal(t, "Updated Task", finalItem.Title, "Title update preserved")
		assert.Equal(t, domain.TaskStatusTodo, finalItem.Status, "Status unchanged (A was rejected)")
		t.Log("✅ Request B (title) succeeded, Request A (status) rejected - NO LOST UPDATE")
	}
}

// TestConcurrentUpdateItem_DifferentFields verifies that when three concurrent
// requests update different fields of the same item, exactly one succeeds.
//
// Scenario:
//   - Request A updates title
//   - Request B updates status
//   - Request C updates tags
//   - All start with version=1
//
// Expected behavior:
//   - Exactly one request succeeds (version → 2)
//   - Two requests fail with ErrVersionConflict
//   - Failed requests can refetch (version=2) and retry
func TestConcurrentUpdateItem_DifferentFields(t *testing.T) {
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
		Title:      "Concurrent Field Test List",
		CreateTime: time.Now().UTC().UTC(),
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
		CreateTime: time.Now().UTC().UTC(),
		UpdatedAt:  time.Now().UTC().UTC(),
	}
	err = store.CreateItem(ctx, listID, item)
	require.NoError(t, err)

	// Fetch initial item (all goroutines will use same version=1)
	originalItem, err := todoService.GetItem(ctx, itemID)
	require.NoError(t, err)
	require.Equal(t, 1, originalItem.Version, "Initial version should be 1")

	// Run concurrent updates to different fields
	var wg sync.WaitGroup
	type updateResult struct {
		name string
		err  error
	}
	results := make(chan updateResult, 3)

	// Goroutine 1: Update title
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Use the same version (simulating concurrent read-modify-write)
		itemCopy := *originalItem
		itemCopy.Title = "Updated Title"
		_, err := todoService.UpdateItem(ctx, ItemToUpdateParamsWithEtag(listID, &itemCopy))
		results <- updateResult{"title", err}
	}()

	// Goroutine 2: Update status
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond) // Slight delay to ensure title update goes first
		itemCopy := *originalItem
		itemCopy.Status = domain.TaskStatusInProgress
		_, err := todoService.UpdateItem(ctx, ItemToUpdateParamsWithEtag(listID, &itemCopy))
		results <- updateResult{"status", err}
	}()

	// Goroutine 3: Update tags
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond) // More delay
		itemCopy := *originalItem
		itemCopy.Tags = []string{"updated", "concurrent"}
		_, err := todoService.UpdateItem(ctx, ItemToUpdateParamsWithEtag(listID, &itemCopy))
		results <- updateResult{"tags", err}
	}()

	wg.Wait()
	close(results)

	// Collect results
	var successCount int
	var versionConflicts []string
	for result := range results {
		if result.err == nil {
			successCount++
			t.Logf("%s update: SUCCESS", result.name)
		} else if errors.Is(result.err, domain.ErrVersionConflict) {
			versionConflicts = append(versionConflicts, result.name)
			t.Logf("%s update: VERSION CONFLICT (expected)", result.name)
		} else {
			t.Errorf("%s update: UNEXPECTED ERROR: %v", result.name, result.err)
		}
	}

	// Verify exactly one succeeded
	assert.Equal(t, 1, successCount, "Exactly one update should succeed with optimistic locking")
	assert.Equal(t, 2, len(versionConflicts), "Two updates should get version conflicts")

	// Verify final state
	finalItem, err := store.FindItemByID(ctx, itemID)
	require.NoError(t, err)
	assert.Equal(t, 2, finalItem.Version, "Version should be 2 after one successful update")
	assert.NotEmpty(t, finalItem.Title, "Title should be set")
	assert.NotEmpty(t, finalItem.Status, "Status should be set")

	t.Logf("Final state - Title: %s, Status: %s, Tags: %v, Version: %d",
		finalItem.Title, finalItem.Status, finalItem.Tags, finalItem.Version)
}

// TestConcurrentUpdateItem_UpdatedAtTimestamp verifies that the updated_at
// timestamp is properly managed during concurrent updates.
func TestConcurrentUpdateItem_UpdatedAtTimestamp(t *testing.T) {
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
		Title:      "Timestamp Test List",
		CreateTime: time.Now().UTC().UTC(),
		Items:      []domain.TodoItem{},
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create an item
	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	createTime := time.Now().UTC().UTC()
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
	beforeUpdates := time.Now().UTC().UTC()

	// Run concurrent updates
	const numGoroutines = 5
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		goroutineID := i
		go func() {
			defer wg.Done()
			item := &domain.TodoItem{
				ID:     itemID,
				Title:  fmt.Sprintf("Update %d", goroutineID),
				Status: domain.TaskStatusTodo,
			}
			todoService.UpdateItem(ctx, ItemToUpdateParams(listID, item))
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
		Title:      "Multi-Item Test List",
		CreateTime: time.Now().UTC().UTC(),
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
			CreateTime: time.Now().UTC().UTC(),
			UpdatedAt:  time.Now().UTC().UTC(),
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
			item := &domain.TodoItem{
				ID:     itemIDs[itemIndex],
				Title:  fmt.Sprintf("Updated Item %d", itemIndex),
				Status: domain.TaskStatusDone,
			}
			_, err := todoService.UpdateItem(ctx, ItemToUpdateParams(listID, item))
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
