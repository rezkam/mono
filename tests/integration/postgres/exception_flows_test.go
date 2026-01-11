package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/recurring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExceptionFlow_DeletePreventRegeneration verifies that deleted instances
// are not regenerated when the template generates new tasks.
func TestExceptionFlow_DeletePreventRegeneration(t *testing.T) {
	store, ctx := SetupTestStore(t)
	generator := recurring.NewDomainGenerator()
	service := todo.NewService(store, generator, todo.Config{})

	// Create template
	template := createTestRecurringTemplate(t, store, "Daily Task")

	// Generate initial instances (5 days)
	now := time.Now().UTC().Truncate(24 * time.Hour)
	endDate := now.AddDate(0, 0, 4)                      // 0-4 = 5 days (inclusive range)
	exceptions := []*domain.RecurringTemplateException{} // No exceptions yet

	tasks, err := generator.GenerateTasksForTemplateWithExceptions(ctx, template, now, endDate, exceptions)
	require.NoError(t, err)
	require.Len(t, tasks, 5) // 5 days

	// Insert instances
	count, err := store.BatchInsertItemsIgnoreConflict(ctx, tasks)
	require.NoError(t, err)
	assert.Equal(t, 5, count)

	// Delete 3rd instance via service (should create exception)
	err = service.DeleteItem(ctx, template.ListID, tasks[2].ID)
	require.NoError(t, err)

	// Verify exception created
	exceptions, err = store.ListAllExceptionsByTemplate(ctx, template.ID)
	require.NoError(t, err)
	require.Len(t, exceptions, 1)
	assert.Equal(t, domain.ExceptionTypeDeleted, exceptions[0].ExceptionType)
	assert.WithinDuration(t, *tasks[2].OccursAt, exceptions[0].OccursAt, time.Second)

	// Attempt regeneration - should skip deleted instance
	regeneratedTasks, err := generator.GenerateTasksForTemplateWithExceptions(ctx, template, now, endDate, exceptions)
	require.NoError(t, err)
	assert.Len(t, regeneratedTasks, 4) // 5 - 1 deleted = 4

	// Verify deleted occurs_at not in regenerated tasks
	deletedOccursAt := *tasks[2].OccursAt
	for _, task := range regeneratedTasks {
		assert.NotEqual(t, deletedOccursAt, *task.OccursAt, "Deleted occurrence should not regenerate")
	}

	// Verify item was HARD deleted (not in database)
	_, err = store.FindItemByID(ctx, tasks[2].ID)
	assert.ErrorIs(t, err, domain.ErrItemNotFound, "Deleted item should not exist in database (hard delete)")
}

// TestExceptionFlow_EditKeepsTemplateLink verifies that editing a recurring item
// creates an exception but keeps the template link intact.
func TestExceptionFlow_EditKeepsTemplateLink(t *testing.T) {
	store, ctx := SetupTestStore(t)
	generator := recurring.NewDomainGenerator()
	service := todo.NewService(store, generator, todo.Config{})

	// Create template and instance
	template := createTestRecurringTemplate(t, store, "Daily Task")

	itemID, _ := uuid.NewV7()
	occursAt := time.Now().UTC().Truncate(time.Second)
	startsAt := occursAt
	item := &domain.TodoItem{
		ID:                  itemID.String(),
		Title:               "Original Title",
		Status:              domain.TaskStatusTodo,
		RecurringTemplateID: &template.ID,
		StartsAt:            &startsAt,
		OccursAt:            &occursAt,
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
		Version:             1,
	}
	_, err := store.CreateItem(ctx, template.ListID, item)
	require.NoError(t, err)

	// Edit title via service
	newTitle := "Edited Title"
	params := domain.UpdateItemParams{
		ItemID:     item.ID,
		ListID:     template.ListID,
		UpdateMask: []string{domain.FieldItemTitle},
		Title:      &newTitle,
	}
	updated, err := service.UpdateItem(ctx, params)
	require.NoError(t, err)

	// Verify template link preserved
	assert.NotNil(t, updated.RecurringTemplateID)
	assert.Equal(t, template.ID, *updated.RecurringTemplateID)
	assert.Equal(t, newTitle, updated.Title)

	// Verify exception created
	exceptions, err := store.ListAllExceptionsByTemplate(ctx, template.ID)
	require.NoError(t, err)
	require.Len(t, exceptions, 1)
	assert.Equal(t, domain.ExceptionTypeEdited, exceptions[0].ExceptionType)
	assert.Equal(t, item.ID, *exceptions[0].ItemID)
	assert.WithinDuration(t, occursAt, exceptions[0].OccursAt, time.Second)
}

// TestExceptionFlow_RescheduleDetachesItem verifies that rescheduling a recurring item
// detaches it from the template and creates an exception to prevent regeneration.
// NOTE: Rescheduling is not yet supported via UpdateItem API.
// This test is commented out until schedule field updates are implemented.
/*
func TestExceptionFlow_RescheduleDetachesItem(t *testing.T) {
	store, ctx := SetupTestStore(t)
	generator := recurring.NewDomainGenerator()
	service := todo.NewService(store, generator, todo.Config{})

	// Create template and instance
	template := createTestRecurringTemplate(t, store, "Daily Task")

	itemID, _ := uuid.NewV7()
	occursAt := time.Now().UTC().Truncate(time.Second)
	startsAt := occursAt
	item := &domain.TodoItem{
		ID:                  itemID.String(),
		Title:               "Task",
		Status:              domain.TaskStatusTodo,
		RecurringTemplateID: &template.ID,
		StartsAt:            &startsAt,
		OccursAt:            &occursAt,
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
		Version:             1,
	}
	_, err := store.CreateItem(ctx, template.ListID, item)
	require.NoError(t, err)

	// Reschedule via service (move 2 hours later)
	newOccursAt := occursAt.Add(2 * time.Hour)
	newStartsAt := newOccursAt
	params := domain.UpdateItemParams{
		ItemID:     item.ID,
		ListID:     template.ListID,
		UpdateMask: []string{domain.FieldOccursAt, domain.FieldStartsAt},
		OccursAt:   &newOccursAt,
		StartsAt:   &newStartsAt,
	}
	updated, err := service.UpdateItem(ctx, params)
	require.NoError(t, err)

	// Verify detached from template
	assert.Nil(t, updated.RecurringTemplateID, "Item should be detached after reschedule")
	assert.Equal(t, newOccursAt, *updated.OccursAt)

	// Verify exception created with ORIGINAL occurs_at
	exceptions, err := store.ListAllExceptionsByTemplate(ctx, template.ID)
	require.NoError(t, err)
	require.Len(t, exceptions, 1)
	assert.Equal(t, domain.ExceptionTypeRescheduled, exceptions[0].ExceptionType)
	assert.Equal(t, occursAt, exceptions[0].OccursAt) // Original time, not new time
	assert.Equal(t, item.ID, *exceptions[0].ItemID)
}
*/

// TestExceptionFlow_QueryExcludesOnlyDeletedItems verifies that FindItems query
// only excludes items with 'deleted' exception type.
// Items with 'edited' or 'rescheduled' exceptions MUST appear in results.
func TestExceptionFlow_QueryExcludesOnlyDeletedItems(t *testing.T) {
	store, ctx := SetupTestStore(t)

	// Create template
	template := createTestRecurringTemplate(t, store, "Daily Task")
	listID := template.ListID

	// Create 4 items
	now := time.Now().UTC().Truncate(24 * time.Hour)
	items := make([]*domain.TodoItem, 4)
	for i := range 4 {
		itemID, _ := uuid.NewV7()
		occursAt := now.Add(time.Duration(i) * 24 * time.Hour)
		startsAt := occursAt

		item := &domain.TodoItem{
			ID:                  itemID.String(),
			Title:               "Task " + string(rune('A'+i)),
			Status:              domain.TaskStatusTodo,
			RecurringTemplateID: &template.ID,
			StartsAt:            &startsAt,
			OccursAt:            &occursAt,
			CreatedAt:           time.Now().UTC(),
			UpdatedAt:           time.Now().UTC(),
			Version:             1,
		}
		items[i], _ = store.CreateItem(ctx, listID, item)
	}

	// Create exceptions with different types:
	// - Item 0: deleted (should be excluded from query - but item won't exist after hard delete)
	// - Item 1: edited (should APPEAR in query results)
	// - Item 2, 3: no exceptions (should appear)
	exceptions := []struct {
		itemIndex int
		excType   domain.ExceptionType
	}{
		{0, domain.ExceptionTypeDeleted},
		{1, domain.ExceptionTypeEdited},
	}

	for _, exc := range exceptions {
		excID, _ := uuid.NewV7()
		exception := &domain.RecurringTemplateException{
			ID:            excID.String(),
			TemplateID:    template.ID,
			OccursAt:      *items[exc.itemIndex].OccursAt,
			ExceptionType: exc.excType,
			ItemID:        &items[exc.itemIndex].ID,
			CreatedAt:     time.Now().UTC(),
		}
		_, err := store.CreateException(ctx, exception)
		require.NoError(t, err)
	}

	// Simulate hard delete of item 0 (in production, DeleteItem would do this)
	// For this test, we manually delete to isolate the query behavior
	err := store.DeleteItem(ctx, items[0].ID)
	require.NoError(t, err)

	// Query items - should return items 1 (edited), 2, and 3
	// Item 0 doesn't appear because it's hard deleted (not because of exception filtering)
	result, err := store.FindItems(ctx, domain.ListTasksParams{
		ListID: &listID,
		Limit:  10,
	}, nil)

	require.NoError(t, err)
	require.Len(t, result.Items, 3, "Should return 3 items: edited item + 2 without exceptions")

	// Verify correct items returned
	returnedIDs := make([]string, len(result.Items))
	for i, item := range result.Items {
		returnedIDs[i] = item.ID
	}
	assert.Contains(t, returnedIDs, items[1].ID, "Edited item MUST appear in results")
	assert.Contains(t, returnedIDs, items[2].ID, "Item without exception should appear")
	assert.Contains(t, returnedIDs, items[3].ID, "Item without exception should appear")
	assert.NotContains(t, returnedIDs, items[0].ID, "Hard deleted item should not appear")
}

// TestExceptionFlow_MultipleEditsOnSameInstance verifies that attempting to create
// a second exception for the same occurrence returns ErrExceptionAlreadyExists.
func TestExceptionFlow_MultipleEditsOnSameInstance(t *testing.T) {
	store, ctx := SetupTestStore(t)

	template := createTestRecurringTemplate(t, store, "Daily Task")
	occursAt := time.Now().UTC().Truncate(time.Second)

	// Create first exception
	exc1ID, _ := uuid.NewV7()
	exc1 := &domain.RecurringTemplateException{
		ID:            exc1ID.String(),
		TemplateID:    template.ID,
		OccursAt:      occursAt,
		ExceptionType: domain.ExceptionTypeEdited,
		CreatedAt:     time.Now().UTC(),
	}
	_, err := store.CreateException(ctx, exc1)
	require.NoError(t, err)

	// Attempt second exception (should fail)
	exc2ID, _ := uuid.NewV7()
	exc2 := &domain.RecurringTemplateException{
		ID:            exc2ID.String(),
		TemplateID:    template.ID,
		OccursAt:      occursAt, // Same occurrence
		ExceptionType: domain.ExceptionTypeDeleted,
		CreatedAt:     time.Now().UTC(),
	}
	_, err = store.CreateException(ctx, exc2)

	assert.ErrorIs(t, err, domain.ErrExceptionAlreadyExists)
}

// TestExceptionFlow_DoubleEditSameInstance verifies that editing the same recurring
// instance twice should work - the second edit should succeed without trying to
// create a duplicate exception.
// BUG: Currently fails because UpdateItem always tries to create a new exception.
func TestExceptionFlow_DoubleEditSameInstance(t *testing.T) {
	store, ctx := SetupTestStore(t)
	generator := recurring.NewDomainGenerator()
	service := todo.NewService(store, generator, todo.Config{})

	// Create template and instance
	template := createTestRecurringTemplate(t, store, "Daily Task")

	itemID, _ := uuid.NewV7()
	occursAt := time.Now().UTC().Truncate(time.Second)
	startsAt := occursAt
	item := &domain.TodoItem{
		ID:                  itemID.String(),
		Title:               "Original Title",
		Status:              domain.TaskStatusTodo,
		RecurringTemplateID: &template.ID,
		StartsAt:            &startsAt,
		OccursAt:            &occursAt,
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
		Version:             1,
	}
	_, err := store.CreateItem(ctx, template.ListID, item)
	require.NoError(t, err)

	// First edit: Change title (should create exception)
	firstTitle := "First Edit"
	params1 := domain.UpdateItemParams{
		ItemID:     item.ID,
		ListID:     template.ListID,
		UpdateMask: []string{domain.FieldItemTitle},
		Title:      &firstTitle,
	}
	updated1, err := service.UpdateItem(ctx, params1)
	require.NoError(t, err)
	assert.Equal(t, firstTitle, updated1.Title)

	// Verify exception created
	exceptions, err := store.ListAllExceptionsByTemplate(ctx, template.ID)
	require.NoError(t, err)
	require.Len(t, exceptions, 1)
	assert.Equal(t, domain.ExceptionTypeEdited, exceptions[0].ExceptionType)

	// Second edit: Change title again (should succeed without creating duplicate exception)
	secondTitle := "Second Edit"
	params2 := domain.UpdateItemParams{
		ItemID:     item.ID,
		ListID:     template.ListID,
		UpdateMask: []string{domain.FieldItemTitle},
		Title:      &secondTitle,
	}
	updated2, err := service.UpdateItem(ctx, params2)
	require.NoError(t, err, "Second edit should succeed without creating duplicate exception")
	assert.Equal(t, secondTitle, updated2.Title)

	// Verify still only one exception (not two)
	exceptions, err = store.ListAllExceptionsByTemplate(ctx, template.ID)
	require.NoError(t, err)
	assert.Len(t, exceptions, 1, "Should still have only one exception after second edit")
}

// TestExceptionFlow_CountMatchesListResults verifies that the total_count from
// ListTasksWithFilters matches the actual number of items returned, including
// when edited items are present.
func TestExceptionFlow_CountMatchesListResults(t *testing.T) {
	store, ctx := SetupTestStore(t)

	// Create template
	template := createTestRecurringTemplate(t, store, "Daily Task")
	listID := template.ListID

	// Create 5 items
	now := time.Now().UTC().Truncate(24 * time.Hour)
	items := make([]*domain.TodoItem, 5)
	for i := range 5 {
		itemID, _ := uuid.NewV7()
		occursAt := now.Add(time.Duration(i) * 24 * time.Hour)
		startsAt := occursAt

		item := &domain.TodoItem{
			ID:                  itemID.String(),
			Title:               "Task " + string(rune('A'+i)),
			Status:              domain.TaskStatusTodo,
			RecurringTemplateID: &template.ID,
			StartsAt:            &startsAt,
			OccursAt:            &occursAt,
			CreatedAt:           time.Now().UTC(),
			UpdatedAt:           time.Now().UTC(),
			Version:             1,
		}
		items[i], _ = store.CreateItem(ctx, listID, item)
	}

	// Create an edited exception for item 1
	excID, _ := uuid.NewV7()
	exception := &domain.RecurringTemplateException{
		ID:            excID.String(),
		TemplateID:    template.ID,
		OccursAt:      *items[1].OccursAt,
		ExceptionType: domain.ExceptionTypeEdited,
		ItemID:        &items[1].ID,
		CreatedAt:     time.Now().UTC(),
	}
	_, err := store.CreateException(ctx, exception)
	require.NoError(t, err)

	// Query with limit 2 - should return 2 items but total_count should be 5 (all items)
	result, err := store.FindItems(ctx, domain.ListTasksParams{
		ListID: &listID,
		Limit:  2,
	}, nil)

	require.NoError(t, err)
	assert.Len(t, result.Items, 2, "Should return 2 items (limit)")
	assert.Equal(t, 5, result.TotalCount, "TotalCount should be 5 (all items including edited)")
}

// TestExceptionFlow_DeleteTemplateRemovesFutureEditedItems verifies that when
// deleting a template, ALL future connected items are hard deleted, including
// those with 'edited' or 'rescheduled' exceptions.
func TestExceptionFlow_DeleteTemplateRemovesFutureEditedItems(t *testing.T) {
	store, ctx := SetupTestStore(t)
	generator := recurring.NewDomainGenerator()
	service := todo.NewService(store, generator, todo.Config{})

	// Create template
	template := createTestRecurringTemplate(t, store, "Daily Task")
	listID := template.ListID

	// Create items: 2 past (completed) + 3 future (1 edited, 2 regular)
	now := time.Now().UTC().Truncate(24 * time.Hour)
	items := make([]*domain.TodoItem, 5)

	// Past items (yesterday and 2 days ago) - completed
	for i := 0; i < 2; i++ {
		itemID, _ := uuid.NewV7()
		occursAt := now.Add(time.Duration(-(i + 1)) * 24 * time.Hour) // -1 day, -2 days
		startsAt := occursAt

		item := &domain.TodoItem{
			ID:                  itemID.String(),
			Title:               "Past Task " + string(rune('A'+i)),
			Status:              domain.TaskStatusDone, // Completed
			RecurringTemplateID: &template.ID,
			StartsAt:            &startsAt,
			OccursAt:            &occursAt,
			CreatedAt:           time.Now().UTC().Add(-24 * time.Hour),
			UpdatedAt:           time.Now().UTC(),
			Version:             1,
		}
		items[i], _ = store.CreateItem(ctx, listID, item)
	}

	// Future items (tomorrow, +2 days, +3 days) - pending
	for i := 2; i < 5; i++ {
		itemID, _ := uuid.NewV7()
		occursAt := now.Add(time.Duration(i-1) * 24 * time.Hour) // +1, +2, +3 days
		startsAt := occursAt

		item := &domain.TodoItem{
			ID:                  itemID.String(),
			Title:               "Future Task " + string(rune('A'+i)),
			Status:              domain.TaskStatusTodo,
			RecurringTemplateID: &template.ID,
			StartsAt:            &startsAt,
			OccursAt:            &occursAt,
			CreatedAt:           time.Now().UTC(),
			UpdatedAt:           time.Now().UTC(),
			Version:             1,
		}
		items[i], _ = store.CreateItem(ctx, listID, item)
	}

	// Create an edited exception for future item 2 (index 2)
	excID, _ := uuid.NewV7()
	exception := &domain.RecurringTemplateException{
		ID:            excID.String(),
		TemplateID:    template.ID,
		OccursAt:      *items[2].OccursAt,
		ExceptionType: domain.ExceptionTypeEdited,
		ItemID:        &items[2].ID,
		CreatedAt:     time.Now().UTC(),
	}
	_, err := store.CreateException(ctx, exception)
	require.NoError(t, err)

	// Delete the template
	err = service.DeleteRecurringTemplate(ctx, listID, template.ID)
	require.NoError(t, err)

	// Verify: Past items should still exist (preserved for history)
	for i := 0; i < 2; i++ {
		item, err := store.FindItemByID(ctx, items[i].ID)
		require.NoError(t, err, "Past item %d should still exist", i)
		assert.NotNil(t, item)
	}

	// Verify: Future items should be HARD deleted, including the edited one
	for i := 2; i < 5; i++ {
		_, err := store.FindItemByID(ctx, items[i].ID)
		assert.ErrorIs(t, err, domain.ErrItemNotFound, "Future item %d should be hard deleted", i)
	}

	// Verify template is soft deleted (inactive)
	deletedTemplate, err := store.FindRecurringTemplateByID(ctx, template.ID)
	require.NoError(t, err)
	assert.False(t, deletedTemplate.IsActive, "Template should be marked inactive")
}
