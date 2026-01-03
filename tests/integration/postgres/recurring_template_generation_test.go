package integration

import (
	"context"
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

// setupRecurringTest initializes a test environment with storage and service.
func setupRecurringTest(t *testing.T) (*postgres.Store, *todo.Service, func()) {
	t.Helper()

	_, dbCleanup := SetupTestDB(t)

	ctx := context.Background()
	pgURL := GetTestStorageDSN(t)

	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)

	generator := recurring.NewDomainGenerator()
	todoService := todo.NewService(store, generator, todo.Config{
		DefaultPageSize: 25,
		MaxPageSize:     100,
	})

	cleanup := func() {
		store.Close()
		dbCleanup()
	}

	return store, todoService, cleanup
}

// TestSyncLayerGeneration verifies that creating a template immediately generates
// SYNC horizon tasks (14 days by default).
func TestSyncLayerGeneration(t *testing.T) {
	store, service, cleanup := setupRecurringTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a list
	listID, err := uuid.NewV7()
	require.NoError(t, err)
	list := &domain.TodoList{
		ID:    listID.String(),
		Title: "Test List",
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a daily recurring template
	template := &domain.RecurringTemplate{
		ListID:                list.ID,
		Title:                 "Daily Standup",
		RecurrencePattern:     domain.RecurrenceDaily,
		RecurrenceConfig:      map[string]any{"interval": float64(1)},
		SyncHorizonDays:       14, // SYNC layer: immediate generation
		GenerationHorizonDays: 365,
		IsActive:              true,
	}

	created, err := service.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)

	// Verify SYNC layer: Items should be immediately available for the next 14 days
	result, err := service.ListItems(ctx, domain.ListTasksParams{
		ListID: &list.ID,
		Limit:  50,
	})
	require.NoError(t, err)

	// Should have approximately 14 tasks (daily for 14 days)
	assert.GreaterOrEqual(t, len(result.Items), 13, "should have at least 13 daily tasks")
	assert.LessOrEqual(t, len(result.Items), 15, "should have at most 15 daily tasks")

	// Verify all tasks have recurring_template_id set
	for _, item := range result.Items {
		assert.NotNil(t, item.RecurringTemplateID, "task should have recurring_template_id")
		if item.RecurringTemplateID != nil {
			assert.Equal(t, created.ID, *item.RecurringTemplateID)
		}
	}
}

// TestOnDemandLayerGeneration verifies that querying far-future dates triggers
// ON-DEMAND generation.
func TestOnDemandLayerGeneration(t *testing.T) {
	store, service, cleanup := setupRecurringTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a list
	listID, err := uuid.NewV7()
	require.NoError(t, err)
	list := &domain.TodoList{
		ID:    listID.String(),
		Title: "Test List",
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a weekly recurring template with DueOffset so tasks have DueAt
	dueOffset := 24 * time.Hour // Due 1 day after occurrence
	template := &domain.RecurringTemplate{
		ListID:                list.ID,
		Title:                 "Weekly Review",
		RecurrencePattern:     domain.RecurrenceWeekly,
		RecurrenceConfig:      map[string]any{"interval": float64(1)},
		SyncHorizonDays:       14,  // SYNC: 14 days
		GenerationHorizonDays: 365, // ASYNC: 365 days total
		DueOffset:             &dueOffset,
		IsActive:              true,
	}

	created, err := service.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Query for tasks 200 days in the future (beyond ASYNC, requires ON-DEMAND)
	futureDate := time.Now().UTC().AddDate(0, 0, 200)
	endDate := futureDate.AddDate(0, 0, 30) // 30-day window

	result, err := service.ListItems(ctx, domain.ListTasksParams{
		ListID:    &list.ID,
		DueAfter:  &futureDate,
		DueBefore: &endDate,
		Limit:     50,
	})
	require.NoError(t, err)

	// Should have approximately 4-5 weekly tasks in a 30-day window
	assert.GreaterOrEqual(t, len(result.Items), 3, "should have at least 3 weekly tasks")
	assert.LessOrEqual(t, len(result.Items), 6, "should have at most 6 weekly tasks")

	// Verify ON-DEMAND tasks are linked to template
	for _, item := range result.Items {
		assert.NotNil(t, item.RecurringTemplateID)
		if item.RecurringTemplateID != nil {
			assert.Equal(t, created.ID, *item.RecurringTemplateID)
		}
	}
}

// TestPatternChange_DeletesOldAndGeneratesNew verifies that changing a template's
// recurrence pattern deletes future pending items and regenerates with new pattern.
func TestPatternChange_DeletesOldAndGeneratesNew(t *testing.T) {
	store, service, cleanup := setupRecurringTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a list
	listID, err := uuid.NewV7()
	require.NoError(t, err)
	list := &domain.TodoList{
		ID:    listID.String(),
		Title: "Test List",
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a daily template
	template := &domain.RecurringTemplate{
		ListID:                list.ID,
		Title:                 "Task",
		RecurrencePattern:     domain.RecurrenceDaily,
		RecurrenceConfig:      map[string]any{"interval": float64(1)},
		SyncHorizonDays:       14,
		GenerationHorizonDays: 365,
		IsActive:              true,
	}

	created, err := service.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Get initial task count (should be ~14 daily tasks)
	initialResult, err := service.ListItems(ctx, domain.ListTasksParams{
		ListID: &list.ID,
		Limit:  50,
	})
	require.NoError(t, err)
	initialCount := len(initialResult.Items)
	assert.GreaterOrEqual(t, initialCount, 13)
	assert.LessOrEqual(t, initialCount, 15)

	// Change pattern to weekly
	newPattern := domain.RecurrenceWeekly
	updateParams := domain.UpdateRecurringTemplateParams{
		TemplateID:        created.ID,
		ListID:            list.ID,
		UpdateMask:        []string{domain.FieldRecurrencePattern},
		RecurrencePattern: &newPattern,
	}

	_, err = service.UpdateRecurringTemplate(ctx, updateParams)
	require.NoError(t, err)

	// Get new task count (should be ~2 weekly tasks in 14 days)
	newResult, err := service.ListItems(ctx, domain.ListTasksParams{
		ListID: &list.ID,
		Limit:  50,
	})
	require.NoError(t, err)
	newCount := len(newResult.Items)

	// Should have fewer tasks now (weekly instead of daily)
	assert.Less(t, newCount, initialCount, "weekly tasks should be fewer than daily tasks")
	assert.GreaterOrEqual(t, newCount, 2, "should have at least 2 weekly tasks")
	assert.LessOrEqual(t, newCount, 4, "should have at most 4 weekly tasks in 14 days (boundary inclusive)")
}

// TestEditContent_CreatesExceptionButKeepsTemplateLink verifies that editing an item's content
// (title, priority) creates an "edited" exception but KEEPS the template link.
// This is the new exception-based design where content edits don't detach, they just mark
// the occurrence as edited so it's excluded from regeneration and list queries via exceptions.
func TestEditContent_CreatesExceptionButKeepsTemplateLink(t *testing.T) {
	store, service, cleanup := setupRecurringTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a list
	listID, err := uuid.NewV7()
	require.NoError(t, err)
	list := &domain.TodoList{
		ID:    listID.String(),
		Title: "Test List",
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a recurring template
	template := &domain.RecurringTemplate{
		ListID:                list.ID,
		Title:                 "Daily Task",
		RecurrencePattern:     domain.RecurrenceDaily,
		RecurrenceConfig:      map[string]any{"interval": float64(1)},
		SyncHorizonDays:       14,
		GenerationHorizonDays: 365,
		IsActive:              true,
	}

	created, err := service.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Get the first generated task
	result, err := service.ListItems(ctx, domain.ListTasksParams{
		ListID: &list.ID,
		Limit:  1,
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)

	originalItem := result.Items[0]
	require.NotNil(t, originalItem.RecurringTemplateID, "item should start with template link")
	require.NotNil(t, originalItem.OccursAt, "recurring item should have occurs_at")
	assert.Equal(t, created.ID, *originalItem.RecurringTemplateID)

	// Edit the item's title (content change)
	newTitle := "Custom Modified Title"
	updateParams := domain.UpdateItemParams{
		ItemID:     originalItem.ID,
		ListID:     list.ID,
		UpdateMask: []string{domain.FieldItemTitle},
		Title:      &newTitle,
	}

	updated, err := service.UpdateItem(ctx, updateParams)
	require.NoError(t, err)

	// Verify template link is KEPT (new behavior: content changes don't detach)
	assert.NotNil(t, updated.RecurringTemplateID, "content edit should KEEP template link")
	assert.Equal(t, created.ID, *updated.RecurringTemplateID)
	assert.Equal(t, newTitle, updated.Title)

	// Verify an exception was created for this occurrence
	exception, err := store.FindExceptionByOccurrence(ctx, created.ID, *originalItem.OccursAt)
	require.NoError(t, err)
	require.NotNil(t, exception, "exception should be created for edited item")
	assert.Equal(t, domain.ExceptionTypeEdited, exception.ExceptionType)
	assert.Equal(t, &originalItem.ID, exception.ItemID, "exception should reference the edited item")
}

// TestDetachOnEdit_StatusChangeDoesNotDetach verifies that changing status
// (completing a task) does NOT detach it from the template.
func TestDetachOnEdit_StatusChangeDoesNotDetach(t *testing.T) {
	store, service, cleanup := setupRecurringTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a list
	listID, err := uuid.NewV7()
	require.NoError(t, err)
	list := &domain.TodoList{
		ID:    listID.String(),
		Title: "Test List",
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a recurring template
	template := &domain.RecurringTemplate{
		ListID:                list.ID,
		Title:                 "Daily Task",
		RecurrencePattern:     domain.RecurrenceDaily,
		RecurrenceConfig:      map[string]any{"interval": float64(1)},
		SyncHorizonDays:       14,
		GenerationHorizonDays: 365,
		IsActive:              true,
	}

	created, err := service.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Get the first generated task
	result, err := service.ListItems(ctx, domain.ListTasksParams{
		ListID: &list.ID,
		Limit:  1,
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)

	originalItem := result.Items[0]
	require.NotNil(t, originalItem.RecurringTemplateID)

	// Complete the task (status change only)
	newStatus := domain.TaskStatusDone
	updateParams := domain.UpdateItemParams{
		ItemID:     originalItem.ID,
		ListID:     list.ID,
		UpdateMask: []string{domain.FieldStatus},
		Status:     &newStatus,
	}

	updated, err := service.UpdateItem(ctx, updateParams)
	require.NoError(t, err)

	// Verify NO detachment: recurring_template_id should still be set
	assert.NotNil(t, updated.RecurringTemplateID, "status change should NOT detach item")
	assert.Equal(t, created.ID, *updated.RecurringTemplateID)
	assert.Equal(t, domain.TaskStatusDone, updated.Status)
}

// TestIdempotency_DuplicatePrevent verifies that the database constraint
// on (recurring_template_id, occurs_at) prevents duplicate task creation.
func TestIdempotency_DuplicatePrevention(t *testing.T) {
	store, service, cleanup := setupRecurringTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a list
	listID, err := uuid.NewV7()
	require.NoError(t, err)
	list := &domain.TodoList{
		ID:    listID.String(),
		Title: "Test List",
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a recurring template
	template := &domain.RecurringTemplate{
		ListID:                list.ID,
		Title:                 "Daily Task",
		RecurrencePattern:     domain.RecurrenceDaily,
		RecurrenceConfig:      map[string]any{"interval": float64(1)},
		SyncHorizonDays:       7,
		GenerationHorizonDays: 365,
		IsActive:              true,
	}

	created, err := service.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Get initial count
	initialResult, err := service.ListItems(ctx, domain.ListTasksParams{
		ListID: &list.ID,
		Limit:  50,
	})
	require.NoError(t, err)
	initialCount := len(initialResult.Items)

	// Attempt to trigger regeneration by querying the same date range again
	// ON-DEMAND generation should not create duplicates
	result2, err := service.ListItems(ctx, domain.ListTasksParams{
		ListID: &list.ID,
		Limit:  50,
	})
	require.NoError(t, err)

	// Count should be the same (no duplicates)
	assert.Equal(t, initialCount, len(result2.Items), "idempotency: should not create duplicate tasks")

	// Verify by checking template ID uniqueness for each occurs_at
	occurrenceCounts := make(map[time.Time]int)
	for _, item := range result2.Items {
		if item.RecurringTemplateID != nil && *item.RecurringTemplateID == created.ID {
			if item.OccursAt != nil {
				occurrenceCounts[*item.OccursAt]++
			}
		}
	}

	// Each occurrence date should have exactly 1 task
	for occursAt, count := range occurrenceCounts {
		assert.Equal(t, 1, count, "each occurs_at (%s) should have exactly 1 task", occursAt.Format("2006-01-02"))
	}
}

// TestExceptionHandling_ListItemsExcludesDeletedInstances verifies that ListItems
// excludes recurring task instances that have exceptions (deleted/rescheduled).
func TestExceptionHandling_ListItemsExcludesDeletedInstances(t *testing.T) {
	store, service, cleanup := setupRecurringTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a list
	listID, err := uuid.NewV7()
	require.NoError(t, err)
	list := &domain.TodoList{
		ID:    listID.String(),
		Title: "Test List",
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a daily recurring template
	template := &domain.RecurringTemplate{
		ListID:                list.ID,
		Title:                 "Daily Task",
		RecurrencePattern:     domain.RecurrenceDaily,
		RecurrenceConfig:      map[string]any{"interval": float64(1)},
		SyncHorizonDays:       7,
		GenerationHorizonDays: 365,
		IsActive:              true,
	}

	created, err := service.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Get initial items (should be ~7 daily tasks)
	initialResult, err := service.ListItems(ctx, domain.ListTasksParams{
		ListID: &list.ID,
		Limit:  50,
	})
	require.NoError(t, err)
	initialCount := len(initialResult.Items)
	require.GreaterOrEqual(t, initialCount, 6, "should have at least 6 daily tasks")

	// Pick an item to "delete" (create exception for it)
	itemToExclude := initialResult.Items[2] // Pick the 3rd item
	require.NotNil(t, itemToExclude.OccursAt, "recurring item should have occurs_at")

	// Create exception for this occurrence (simulating user delete)
	excID, err := uuid.NewV7()
	require.NoError(t, err)
	exception := &domain.RecurringTemplateException{
		ID:            excID.String(),
		TemplateID:    created.ID,
		OccursAt:      *itemToExclude.OccursAt,
		ExceptionType: domain.ExceptionTypeDeleted,
		CreatedAt:     time.Now().UTC(),
	}
	_, err = store.CreateException(ctx, exception)
	require.NoError(t, err)

	// List items again - the excluded item should NOT appear
	afterResult, err := service.ListItems(ctx, domain.ListTasksParams{
		ListID: &list.ID,
		Limit:  50,
	})
	require.NoError(t, err)

	// Should have one fewer item
	assert.Equal(t, initialCount-1, len(afterResult.Items),
		"should have one fewer item after exception (was %d, now %d)",
		initialCount, len(afterResult.Items))

	// Verify the specific excluded item is not in results
	for _, item := range afterResult.Items {
		if item.ID == itemToExclude.ID {
			t.Errorf("excluded item %s should not appear in results", itemToExclude.ID)
		}
	}
}

// TestExceptionHandling_GenerationSkipsExceptionDates verifies that the generator
// does not create new tasks for dates that have exceptions.
func TestExceptionHandling_GenerationSkipsExceptionDates(t *testing.T) {
	store, service, cleanup := setupRecurringTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a list
	listID, err := uuid.NewV7()
	require.NoError(t, err)
	list := &domain.TodoList{
		ID:    listID.String(),
		Title: "Test List",
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a daily recurring template
	template := &domain.RecurringTemplate{
		ListID:                list.ID,
		Title:                 "Daily Task",
		RecurrencePattern:     domain.RecurrenceDaily,
		RecurrenceConfig:      map[string]any{"interval": float64(1)},
		SyncHorizonDays:       7,
		GenerationHorizonDays: 365,
		IsActive:              true,
	}

	created, err := service.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Get initial items to verify template was created
	initialResult, err := service.ListItems(ctx, domain.ListTasksParams{
		ListID: &list.ID,
		Limit:  50,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(initialResult.Items), 6, "should have at least 6 daily tasks")

	// Pick a future date to create an exception for (beyond current generated range)
	// This simulates pre-emptively blocking a future occurrence
	futureOccursAt := time.Now().UTC().AddDate(0, 0, 100).Truncate(24 * time.Hour)

	// Create exception for future date
	excID, err := uuid.NewV7()
	require.NoError(t, err)
	exception := &domain.RecurringTemplateException{
		ID:            excID.String(),
		TemplateID:    created.ID,
		OccursAt:      futureOccursAt,
		ExceptionType: domain.ExceptionTypeDeleted,
		CreatedAt:     time.Now().UTC(),
	}
	_, err = store.CreateException(ctx, exception)
	require.NoError(t, err)

	// Query for items in the future range (triggers ON-DEMAND generation)
	futureStart := futureOccursAt.AddDate(0, 0, -3) // 3 days before exception
	futureEnd := futureOccursAt.AddDate(0, 0, 4)    // 4 days after exception

	// Use DueOffset to make items have due_at for filtering
	dueOffset := 24 * time.Hour
	updateParams := domain.UpdateRecurringTemplateParams{
		TemplateID: created.ID,
		ListID:     list.ID,
		UpdateMask: []string{domain.FieldDueOffset},
		DueOffset:  &dueOffset,
	}
	_, err = service.UpdateRecurringTemplate(ctx, updateParams)
	require.NoError(t, err)

	// Query for future items
	futureResult, err := service.ListItems(ctx, domain.ListTasksParams{
		ListID:    &list.ID,
		DueAfter:  &futureStart,
		DueBefore: &futureEnd,
		Limit:     50,
	})
	require.NoError(t, err)

	// Should have ~7 days of tasks minus 1 for exception = ~6 tasks
	// But exact count depends on boundary conditions
	t.Logf("Found %d tasks in future range (expected ~6)", len(futureResult.Items))

	// Verify the exception date is NOT in results
	for _, item := range futureResult.Items {
		if item.OccursAt != nil {
			// Normalize both dates for comparison
			itemDate := item.OccursAt.UTC().Truncate(24 * time.Hour)
			excDate := futureOccursAt.UTC().Truncate(24 * time.Hour)
			if itemDate.Equal(excDate) {
				t.Errorf("exception date %s should not have a task generated",
					excDate.Format("2006-01-02"))
			}
		}
	}

	// Verify we have tasks on adjacent days (before and after exception)
	var hasDayBefore, hasDayAfter bool
	for _, item := range futureResult.Items {
		if item.OccursAt != nil {
			itemDate := item.OccursAt.UTC().Truncate(24 * time.Hour)
			dayBefore := futureOccursAt.AddDate(0, 0, -1).UTC().Truncate(24 * time.Hour)
			dayAfter := futureOccursAt.AddDate(0, 0, 1).UTC().Truncate(24 * time.Hour)

			if itemDate.Equal(dayBefore) {
				hasDayBefore = true
			}
			if itemDate.Equal(dayAfter) {
				hasDayAfter = true
			}
		}
	}
	assert.True(t, hasDayBefore, "should have task on day before exception")
	assert.True(t, hasDayAfter, "should have task on day after exception")
}
