package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateException(t *testing.T) {
	store, ctx := SetupTestStore(t)

	// Create test template
	template := createTestRecurringTemplate(t, store, "Test Template")

	// Create exception
	excID, _ := uuid.NewV7()
	occursAt := time.Now().UTC().Truncate(time.Second)
	exception := &domain.RecurringTemplateException{
		ID:            excID.String(),
		TemplateID:    template.ID,
		OccursAt:      occursAt,
		ExceptionType: domain.ExceptionTypeDeleted,
		ItemID:        nil,
		CreatedAt:     time.Now().UTC(),
	}

	created, err := store.CreateException(ctx, exception)

	require.NoError(t, err)
	assert.Equal(t, exception.ID, created.ID)
	assert.Equal(t, exception.TemplateID, created.TemplateID)
	assert.Equal(t, exception.ExceptionType, created.ExceptionType)
	assert.WithinDuration(t, exception.OccursAt, created.OccursAt, time.Second)
}

func TestCreateException_DuplicateOccurrence(t *testing.T) {
	store, ctx := SetupTestStore(t)

	template := createTestRecurringTemplate(t, store, "Test Template")
	occursAt := time.Now().UTC().Truncate(time.Second)

	// Create first exception
	exc1ID, _ := uuid.NewV7()
	exc1 := &domain.RecurringTemplateException{
		ID:            exc1ID.String(),
		TemplateID:    template.ID,
		OccursAt:      occursAt,
		ExceptionType: domain.ExceptionTypeDeleted,
		CreatedAt:     time.Now().UTC(),
	}
	_, err := store.CreateException(ctx, exc1)
	require.NoError(t, err)

	// Attempt duplicate
	exc2ID, _ := uuid.NewV7()
	exc2 := &domain.RecurringTemplateException{
		ID:            exc2ID.String(),
		TemplateID:    template.ID,
		OccursAt:      occursAt, // Same occurrence
		ExceptionType: domain.ExceptionTypeEdited,
		CreatedAt:     time.Now().UTC(),
	}
	_, err = store.CreateException(ctx, exc2)

	assert.ErrorIs(t, err, domain.ErrExceptionAlreadyExists)
}

func TestFindItems_ExcludesDeletedExceptions(t *testing.T) {
	store, ctx := SetupTestStore(t)

	// Create template and generate items
	template := createTestRecurringTemplate(t, store, "Daily Task")
	listID := template.ListID

	// Generate 5 items manually for testing
	now := time.Now().UTC().Truncate(24 * time.Hour)
	items := make([]*domain.TodoItem, 5)
	for i := range 5 {
		itemID, _ := uuid.NewV7()
		occursAt := now.Add(time.Duration(i) * 24 * time.Hour)
		startsAt := occursAt

		item := &domain.TodoItem{
			ID:                  itemID.String(),
			Title:               "Generated Task",
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

	// Create exception for 3rd item (index 2)
	excID, _ := uuid.NewV7()
	exception := &domain.RecurringTemplateException{
		ID:            excID.String(),
		TemplateID:    template.ID,
		OccursAt:      *items[2].OccursAt,
		ExceptionType: domain.ExceptionTypeDeleted,
		ItemID:        &items[2].ID,
		CreatedAt:     time.Now().UTC(),
	}
	_, err := store.CreateException(ctx, exception)
	require.NoError(t, err)

	// Query items - should exclude item with exception
	result, err := store.FindItems(ctx, domain.ListTasksParams{
		ListID: &listID,
		Limit:  10,
	}, nil)

	require.NoError(t, err)
	assert.Equal(t, 4, len(result.Items)) // 5 - 1 deleted = 4

	// Verify deleted item is not in results
	for _, item := range result.Items {
		assert.NotEqual(t, items[2].ID, item.ID, "Deleted item should not appear in results")
	}
}

func createTestRecurringTemplate(t *testing.T, store *postgres.Store, title string) *domain.RecurringTemplate {
	t.Helper()
	listID := createTestList(t, store, "Test List")

	templateID, _ := uuid.NewV7()
	template := &domain.RecurringTemplate{
		ID:                    templateID.String(),
		ListID:                listID,
		Title:                 title,
		RecurrencePattern:     domain.RecurrenceDaily,
		RecurrenceConfig:      map[string]any{"interval": 1},
		IsActive:              true,
		GeneratedThrough:      time.Now().UTC(),
		SyncHorizonDays:       14,
		GenerationHorizonDays: 365,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
	}
	created, err := store.CreateRecurringTemplate(context.Background(), template)
	require.NoError(t, err)
	return created
}
