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

	// Verify item was archived (soft delete)
	archivedItem, err := store.FindItemByID(ctx, tasks[2].ID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusArchived, archivedItem.Status)
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

// TestExceptionFlow_QueryExcludesAllTypes verifies that FindItems query
// excludes items with ANY exception type (deleted, edited, rescheduled).
func TestExceptionFlow_QueryExcludesAllTypes(t *testing.T) {
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

	// Create exceptions with different types
	exceptions := []struct {
		itemIndex int
		excType   domain.ExceptionType
	}{
		{0, domain.ExceptionTypeDeleted},
		{1, domain.ExceptionTypeEdited},
		// Item 2 and 3 have no exceptions
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

	// Query items - should return items 2 and 3 (no exceptions)
	result, err := store.FindItems(ctx, domain.ListTasksParams{
		ListID: &listID,
		Limit:  10,
	}, nil)

	require.NoError(t, err)
	require.Len(t, result.Items, 2, "Should return 2 items without exceptions")

	// Verify correct items returned
	returnedIDs := []string{result.Items[0].ID, result.Items[1].ID}
	assert.Contains(t, returnedIDs, items[2].ID)
	assert.Contains(t, returnedIDs, items[3].ID)
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
