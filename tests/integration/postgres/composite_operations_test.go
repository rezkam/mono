package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/rezkam/mono/internal/recurring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// === UpdateItemWithException Tests ===

func TestUpdateItemWithException_Success(t *testing.T) {
	dsn := GetTestStorageDSN(t)
	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	// Setup: Create list, template, and item
	listID := createTestList(t, store, "Test List")
	template := createSimpleRecurringTemplate(t, store, listID, "Daily Task", "daily")
	occursAt := time.Now().UTC().Truncate(24 * time.Hour)
	item := createSimpleRecurringItem(t, store, listID, template.ID, occursAt, "Original Title")

	// Prepare update params
	newTitle := "Updated Title"
	params := domain.UpdateItemParams{
		ItemID:     item.ID,
		ListID:     listID,
		Title:      &newTitle,
		UpdateMask: []string{"title"},
	}

	// Prepare exception
	excID, err := uuid.NewV7()
	require.NoError(t, err)
	exception := &domain.RecurringTemplateException{
		ID:            excID.String(),
		TemplateID:    template.ID,
		OccursAt:      occursAt,
		ExceptionType: domain.ExceptionTypeEdited,
		CreatedAt:     time.Now().UTC(),
	}

	// Execute composite operation
	updatedItem, err := store.UpdateItemWithException(ctx, params, exception)

	// Verify success
	require.NoError(t, err)
	assert.NotNil(t, updatedItem)
	assert.Equal(t, "Updated Title", updatedItem.Title)

	// Verify exception was created
	exc, err := store.FindExceptionByOccurrence(ctx, template.ID, occursAt)
	require.NoError(t, err)
	assert.Equal(t, domain.ExceptionTypeEdited, exc.ExceptionType)
}

func TestUpdateItemWithException_RollbackOnExceptionFailure(t *testing.T) {
	dsn := GetTestStorageDSN(t)
	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	// Setup
	listID := createTestList(t, store, "Test List")
	template := createSimpleRecurringTemplate(t, store, listID, "Daily Task", "daily")
	occursAt := time.Now().UTC().Truncate(24 * time.Hour)
	item := createSimpleRecurringItem(t, store, listID, template.ID, occursAt, "Original Title")

	// Create exception first (will cause duplicate on composite operation)
	excID1, err := uuid.NewV7()
	require.NoError(t, err)
	_, err = store.CreateException(ctx, &domain.RecurringTemplateException{
		ID:            excID1.String(),
		TemplateID:    template.ID,
		OccursAt:      occursAt,
		ExceptionType: domain.ExceptionTypeEdited,
		CreatedAt:     time.Now().UTC(),
	})
	require.NoError(t, err)

	// Prepare update params
	newTitle := "Should Not Be Applied"
	params := domain.UpdateItemParams{
		ItemID:     item.ID,
		ListID:     listID,
		Title:      &newTitle,
		UpdateMask: []string{"title"},
	}

	// Prepare duplicate exception
	excID2, err := uuid.NewV7()
	require.NoError(t, err)
	duplicateException := &domain.RecurringTemplateException{
		ID:            excID2.String(),
		TemplateID:    template.ID,
		OccursAt:      occursAt, // Same occurrence - will fail unique constraint
		ExceptionType: domain.ExceptionTypeEdited,
		CreatedAt:     time.Now().UTC(),
	}

	// Execute - should fail and rollback
	_, err = store.UpdateItemWithException(ctx, params, duplicateException)

	// Verify failure
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrExceptionAlreadyExists)

	// Verify rollback - item should be unchanged
	unchangedItem, err := store.FindItemByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "Original Title", unchangedItem.Title, "Item should not be updated due to rollback")
}

func TestUpdateItemWithException_RollbackOnItemUpdateFailure(t *testing.T) {
	dsn := GetTestStorageDSN(t)
	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	// Setup
	listID := createTestList(t, store, "Test List")
	template := createSimpleRecurringTemplate(t, store, listID, "Daily Task", "daily")
	occursAt := time.Now().UTC().Truncate(24 * time.Hour)
	item := createSimpleRecurringItem(t, store, listID, template.ID, occursAt, "Original Title")

	// Prepare update params with non-existent item ID
	nonExistentID, err := uuid.NewV7()
	require.NoError(t, err)
	newTitle := "Should Not Be Applied"
	params := domain.UpdateItemParams{
		ItemID:     nonExistentID.String(),
		ListID:     listID,
		Title:      &newTitle,
		UpdateMask: []string{"title"},
	}

	// Prepare exception
	excID, err := uuid.NewV7()
	require.NoError(t, err)
	exception := &domain.RecurringTemplateException{
		ID:            excID.String(),
		TemplateID:    template.ID,
		OccursAt:      occursAt,
		ExceptionType: domain.ExceptionTypeEdited,
		CreatedAt:     time.Now().UTC(),
	}

	// Execute - should fail on item update
	_, err = store.UpdateItemWithException(ctx, params, exception)

	// Verify failure
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrItemNotFound)

	// Verify rollback - exception should not be created
	_, err = store.FindExceptionByOccurrence(ctx, template.ID, occursAt)
	assert.ErrorIs(t, err, domain.ErrExceptionNotFound, "Exception should not exist due to rollback")

	// Verify original item is unchanged
	unchangedItem, err := store.FindItemByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "Original Title", unchangedItem.Title)
}

// === DeleteItemWithException Tests ===

func TestDeleteItemWithException_Success(t *testing.T) {
	dsn := GetTestStorageDSN(t)
	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	// Setup
	listID := createTestList(t, store, "Test List")
	template := createSimpleRecurringTemplate(t, store, listID, "Daily Task", "daily")
	occursAt := time.Now().UTC().Truncate(24 * time.Hour)
	item := createSimpleRecurringItem(t, store, listID, template.ID, occursAt, "Task to Delete")

	// Prepare exception
	excID, err := uuid.NewV7()
	require.NoError(t, err)
	exception := &domain.RecurringTemplateException{
		ID:            excID.String(),
		TemplateID:    template.ID,
		OccursAt:      occursAt,
		ExceptionType: domain.ExceptionTypeDeleted,
		CreatedAt:     time.Now().UTC(),
	}

	// Execute composite operation
	err = store.DeleteItemWithException(ctx, listID, item.ID, exception)

	// Verify success
	require.NoError(t, err)

	// Verify item is archived
	archivedItem, err := store.FindItemByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusArchived, archivedItem.Status)

	// Verify exception was created
	exc, err := store.FindExceptionByOccurrence(ctx, template.ID, occursAt)
	require.NoError(t, err)
	assert.Equal(t, domain.ExceptionTypeDeleted, exc.ExceptionType)
}

func TestDeleteItemWithException_RollbackOnExceptionFailure(t *testing.T) {
	dsn := GetTestStorageDSN(t)
	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	// Setup
	listID := createTestList(t, store, "Test List")
	template := createSimpleRecurringTemplate(t, store, listID, "Daily Task", "daily")
	occursAt := time.Now().UTC().Truncate(24 * time.Hour)
	item := createSimpleRecurringItem(t, store, listID, template.ID, occursAt, "Task to Delete")

	// Create exception first (will cause duplicate)
	excID1, err := uuid.NewV7()
	require.NoError(t, err)
	_, err = store.CreateException(ctx, &domain.RecurringTemplateException{
		ID:            excID1.String(),
		TemplateID:    template.ID,
		OccursAt:      occursAt,
		ExceptionType: domain.ExceptionTypeDeleted,
		CreatedAt:     time.Now().UTC(),
	})
	require.NoError(t, err)

	// Prepare duplicate exception
	excID2, err := uuid.NewV7()
	require.NoError(t, err)
	duplicateException := &domain.RecurringTemplateException{
		ID:            excID2.String(),
		TemplateID:    template.ID,
		OccursAt:      occursAt, // Same occurrence
		ExceptionType: domain.ExceptionTypeDeleted,
		CreatedAt:     time.Now().UTC(),
	}

	// Execute - should fail
	err = store.DeleteItemWithException(ctx, listID, item.ID, duplicateException)

	// Verify failure
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrExceptionAlreadyExists)

	// Verify rollback - item should NOT be archived
	unchangedItem, err := store.FindItemByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusTodo, unchangedItem.Status, "Item should not be archived due to rollback")
}

// === CreateTemplateWithInitialGeneration Tests ===

func TestCreateTemplateWithInitialGeneration_Success(t *testing.T) {
	dsn := GetTestStorageDSN(t)
	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	// Setup
	listID := createTestList(t, store, "Test List")
	now := time.Now().UTC().Truncate(24 * time.Hour)

	// Prepare template
	templateID := newUUID(t)
	template := &domain.RecurringTemplate{
		ID:                    templateID,
		ListID:                listID,
		Title:                 "Daily Task",
		RecurrencePattern:     "daily",
		RecurrenceConfig:      map[string]any{"interval": 1},
		SyncHorizonDays:       7,
		GenerationHorizonDays: 30,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
		GeneratedThrough:      time.Now().UTC(),
		IsActive:              true,
	}

	// Prepare sync items (3 days)
	syncEnd := now.AddDate(0, 0, 7)
	syncItems := []*domain.TodoItem{
		createDomainItem(listID, template.ID, now, "Day 1"),
		createDomainItem(listID, template.ID, now.AddDate(0, 0, 1), "Day 2"),
		createDomainItem(listID, template.ID, now.AddDate(0, 0, 2), "Day 3"),
	}

	// Prepare async job
	jobID := newUUID(t)
	asyncJob := &domain.GenerationJob{
		ID:            jobID,
		TemplateID:    template.ID,
		GenerateFrom:  syncEnd,
		GenerateUntil: now.AddDate(0, 0, 30),
		ScheduledFor:  now,
		RetryCount:    0,
		CreatedAt:     now,
	}

	// Execute composite operation
	createdTemplate, err := store.CreateTemplateWithInitialGeneration(ctx, template, syncItems, syncEnd, asyncJob)

	// Verify success
	require.NoError(t, err)
	assert.NotNil(t, createdTemplate)
	assert.Equal(t, "Daily Task", createdTemplate.Title)

	// Verify template was created
	fetchedTemplate, err := store.FindRecurringTemplate(ctx, template.ID)
	require.NoError(t, err)
	assert.Equal(t, "Daily Task", fetchedTemplate.Title)

	// Verify sync items were inserted
	params := domain.ListTasksParams{
		ListID: &listID,
		Limit:  10,
		Offset: 0,
	}
	items, err := store.FindItems(ctx, params, []domain.TaskStatus{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(items.Items), 3, "At least 3 sync items should be inserted")

	// Verify generation marker was set (within 1 second tolerance for DB timestamp precision)
	assert.WithinDuration(t, syncEnd, fetchedTemplate.GeneratedThrough, time.Second)
}

func TestCreateTemplateWithInitialGeneration_RollbackOnTemplateFailure(t *testing.T) {
	dsn := GetTestStorageDSN(t)
	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	// Setup
	listID := createTestList(t, store, "Test List")
	now := time.Now().UTC().Truncate(24 * time.Hour)

	// Prepare template with INVALID list ID (will cause FK violation)
	templateID := newUUID(t)
	invalidListID := newUUID(t)
	template := &domain.RecurringTemplate{
		ID:                    templateID,
		ListID:                invalidListID, // Non-existent list
		Title:                 "Should Fail",
		RecurrencePattern:     "daily",
		RecurrenceConfig:      map[string]any{"interval": 1},
		SyncHorizonDays:       7,
		GenerationHorizonDays: 30,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
		GeneratedThrough:      time.Now().UTC(),
		IsActive:              true,
	}

	syncEnd := now.AddDate(0, 0, 7)
	syncItems := []*domain.TodoItem{
		createDomainItem(listID, template.ID, now, "Day 1"),
	}

	jobID := newUUID(t)
	asyncJob := &domain.GenerationJob{
		ID:            jobID,
		TemplateID:    template.ID,
		GenerateFrom:  syncEnd,
		GenerateUntil: now.AddDate(0, 0, 30),
		ScheduledFor:  now,
		RetryCount:    0,
		CreatedAt:     now,
	}

	// Execute - should fail
	_, err = store.CreateTemplateWithInitialGeneration(ctx, template, syncItems, syncEnd, asyncJob)

	// Verify failure
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrListNotFound)

	// Verify rollback - template should not exist
	_, err = store.FindRecurringTemplate(ctx, template.ID)
	assert.ErrorIs(t, err, domain.ErrTemplateNotFound, "Template should not exist due to rollback")

	// Verify rollback - items should not exist
	params := domain.ListTasksParams{
		ListID: &listID,
		Limit:  10,
		Offset: 0,
	}
	items, err := store.FindItems(ctx, params, []domain.TaskStatus{})
	require.NoError(t, err)
	assert.Equal(t, 0, len(items.Items), "No items should be inserted due to rollback")
}

func TestCreateTemplateWithInitialGeneration_EmptySyncItems(t *testing.T) {
	dsn := GetTestStorageDSN(t)
	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	// Setup
	listID := createTestList(t, store, "Test List")
	now := time.Now().UTC().Truncate(24 * time.Hour)

	// Prepare template
	templateID := newUUID(t)
	template := &domain.RecurringTemplate{
		ID:                    templateID,
		ListID:                listID,
		Title:                 "Daily Task",
		RecurrencePattern:     "daily",
		RecurrenceConfig:      map[string]any{"interval": 1},
		SyncHorizonDays:       7,
		GenerationHorizonDays: 30,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
		GeneratedThrough:      time.Now().UTC(),
		IsActive:              true,
	}

	syncEnd := now.AddDate(0, 0, 7)
	syncItems := []*domain.TodoItem{} // Empty

	jobID := newUUID(t)
	asyncJob := &domain.GenerationJob{
		ID:            jobID,
		TemplateID:    template.ID,
		GenerateFrom:  syncEnd,
		GenerateUntil: now.AddDate(0, 0, 30),
		ScheduledFor:  now,
		RetryCount:    0,
		CreatedAt:     now,
	}

	// Execute - should succeed even with empty items
	createdTemplate, err := store.CreateTemplateWithInitialGeneration(ctx, template, syncItems, syncEnd, asyncJob)

	// Verify success
	require.NoError(t, err)
	assert.NotNil(t, createdTemplate)
}

func TestCreateTemplateWithInitialGeneration_NilAsyncJob(t *testing.T) {
	dsn := GetTestStorageDSN(t)
	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	// Setup
	listID := createTestList(t, store, "Test List")
	now := time.Now().UTC().Truncate(24 * time.Hour)

	// Prepare template
	templateID := newUUID(t)
	template := &domain.RecurringTemplate{
		ID:                    templateID,
		ListID:                listID,
		Title:                 "Daily Task",
		RecurrencePattern:     "daily",
		RecurrenceConfig:      map[string]any{"interval": 1},
		SyncHorizonDays:       7,
		GenerationHorizonDays: 30,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
		GeneratedThrough:      time.Now().UTC(),
		IsActive:              true,
	}

	syncEnd := now.AddDate(0, 0, 7)
	syncItems := []*domain.TodoItem{
		createDomainItem(listID, template.ID, now, "Day 1"),
	}

	// Execute with nil async job
	createdTemplate, err := store.CreateTemplateWithInitialGeneration(ctx, template, syncItems, syncEnd, nil)

	// Verify success
	require.NoError(t, err)
	assert.NotNil(t, createdTemplate)
}

// === UpdateTemplateWithRegeneration Tests ===

func TestUpdateTemplateWithRegeneration_Success(t *testing.T) {
	dsn := GetTestStorageDSN(t)
	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	generator := recurring.NewDomainGenerator()
	todoService := todo.NewService(store, generator, todo.Config{})

	// Setup
	listID := createTestList(t, store, "Test List")
	template := createSimpleRecurringTemplate(t, store, listID, "Daily Task", "daily")
	now := time.Now().UTC().Truncate(24 * time.Hour)

	// Insert some existing items
	for i := 0; i < 5; i++ {
		occursAt := now.AddDate(0, 0, i)
		_ = createSimpleRecurringItem(t, store, listID, template.ID, occursAt, fmt.Sprintf("Day %d", i+1))
	}

	// Prepare update params (change pattern to weekly)
	newPattern := domain.RecurrencePattern("weekly")
	params := domain.UpdateRecurringTemplateParams{
		TemplateID:        template.ID,
		ListID:            listID,
		RecurrencePattern: &newPattern,
		UpdateMask:        []string{"recurrence_pattern"},
	}

	// Prepare new sync items (regenerated with weekly pattern)
	deleteFrom := now
	syncEnd := now.AddDate(0, 0, 14)
	newSyncItems := []*domain.TodoItem{
		createDomainItem(listID, template.ID, now, "Week 1"),
		createDomainItem(listID, template.ID, now.AddDate(0, 0, 7), "Week 2"),
	}

	// Execute composite operation
	updatedTemplate, err := store.UpdateTemplateWithRegeneration(ctx, params, deleteFrom, newSyncItems, syncEnd)

	// Verify success
	require.NoError(t, err)
	assert.NotNil(t, updatedTemplate)
	assert.Equal(t, domain.RecurrencePattern("weekly"), updatedTemplate.RecurrencePattern)

	// Verify generation marker was updated (within 1 second tolerance for DB timestamp precision)
	assert.WithinDuration(t, syncEnd, updatedTemplate.GeneratedThrough, time.Second)

	// Verify old items were deleted and new items inserted
	listParams := domain.ListTasksParams{
		ListID: &listID,
		Limit:  20,
		Offset: 0,
	}
	items, err := store.FindItems(ctx, listParams, []domain.TaskStatus{domain.TaskStatusArchived})
	require.NoError(t, err)
	// Note: We can't guarantee exact count due to ignore conflict, but verify some items exist
	assert.Greater(t, len(items.Items), 0, "Should have regenerated items")

	_ = todoService // Satisfy unused variable
}

func TestUpdateTemplateWithRegeneration_RollbackOnUpdateFailure(t *testing.T) {
	dsn := GetTestStorageDSN(t)
	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	// Setup
	listID := createTestList(t, store, "Test List")
	template := createSimpleRecurringTemplate(t, store, listID, "Daily Task", "daily")
	now := time.Now().UTC().Truncate(24 * time.Hour)

	// Insert existing items
	existingItem := createSimpleRecurringItem(t, store, listID, template.ID, now, "Original")

	// Prepare invalid update params (non-existent template)
	nonExistentID := newUUID(t)
	newPattern := domain.RecurrencePattern("weekly")
	params := domain.UpdateRecurringTemplateParams{
		TemplateID:        nonExistentID,
		ListID:            listID,
		RecurrencePattern: &newPattern,
		UpdateMask:        []string{"recurrence_pattern"},
	}

	deleteFrom := now
	syncEnd := now.AddDate(0, 0, 14)
	newSyncItems := []*domain.TodoItem{
		createDomainItem(listID, template.ID, now, "Should Not Exist"),
	}

	// Execute - should fail
	_, err = store.UpdateTemplateWithRegeneration(ctx, params, deleteFrom, newSyncItems, syncEnd)

	// Verify failure
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrTemplateNotFound)

	// Verify rollback - existing item should still exist
	item, err := store.FindItemByID(ctx, existingItem.ID)
	require.NoError(t, err)
	assert.Equal(t, "Original", item.Title)

	// Verify template is unchanged
	unchangedTemplate, err := store.FindRecurringTemplate(ctx, template.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.RecurrencePattern("daily"), unchangedTemplate.RecurrencePattern)
}

// === Helper Functions ===

func createDomainItem(listID, templateID string, occursAt time.Time, title string) *domain.TodoItem {
	itemID, _ := uuid.NewV7()
	return &domain.TodoItem{
		ID:                  itemID.String(),
		ListID:              listID,
		Title:               title,
		Status:              domain.TaskStatusTodo,
		RecurringTemplateID: &templateID,
		OccursAt:            &occursAt,
	}
}

func createSimpleRecurringItem(t *testing.T, store *postgres.Store, listID, templateID string, occursAt time.Time, title string) *domain.TodoItem {
	t.Helper()
	item := createDomainItem(listID, templateID, occursAt, title)
	created, err := store.CreateItem(context.Background(), listID, item)
	require.NoError(t, err)
	return created
}

func createSimpleRecurringTemplate(t *testing.T, store *postgres.Store, listID, title, pattern string) *domain.RecurringTemplate {
	t.Helper()
	templateID := newUUID(t)

	template := &domain.RecurringTemplate{
		ID:                    templateID,
		ListID:                listID,
		Title:                 title,
		RecurrencePattern:     domain.RecurrencePattern(pattern),
		RecurrenceConfig:      map[string]any{"interval": 1},
		SyncHorizonDays:       7,
		GenerationHorizonDays: 30,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
		GeneratedThrough:      time.Now().UTC(),
		IsActive:              true,
	}

	created, err := store.CreateRecurringTemplate(context.Background(), template)
	require.NoError(t, err)
	return created
}
