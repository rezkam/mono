package todo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDeleteItemRepo struct {
	findItemFn        func(ctx context.Context, id string) (*domain.TodoItem, error)
	createExceptionFn func(ctx context.Context, exc *domain.RecurringTemplateException) (*domain.RecurringTemplateException, error)
	updateItemFn      func(ctx context.Context, params domain.UpdateItemParams) (*domain.TodoItem, error)
	transactionFn     func(ctx context.Context, fn func(tx Repository) error) error
}

func (m *mockDeleteItemRepo) FindItemByID(ctx context.Context, id string) (*domain.TodoItem, error) {
	if m.findItemFn != nil {
		return m.findItemFn(ctx, id)
	}
	panic("FindItemByID not implemented")
}

func (m *mockDeleteItemRepo) CreateException(ctx context.Context, exc *domain.RecurringTemplateException) (*domain.RecurringTemplateException, error) {
	if m.createExceptionFn != nil {
		return m.createExceptionFn(ctx, exc)
	}
	panic("CreateException not implemented")
}

func (m *mockDeleteItemRepo) UpdateItem(ctx context.Context, params domain.UpdateItemParams) (*domain.TodoItem, error) {
	if m.updateItemFn != nil {
		return m.updateItemFn(ctx, params)
	}
	panic("UpdateItem not implemented")
}

func (m *mockDeleteItemRepo) Transaction(ctx context.Context, fn func(tx Repository) error) error {
	if m.transactionFn != nil {
		return m.transactionFn(ctx, fn)
	}
	// Default: execute without actual transaction
	return fn(m)
}

// Stub all other methods
func (m *mockDeleteItemRepo) CreateList(ctx context.Context, list *domain.TodoList) (*domain.TodoList, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) FindListByID(ctx context.Context, id string) (*domain.TodoList, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) ListLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) UpdateList(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) CreateItem(ctx context.Context, listID string, item *domain.TodoItem) (*domain.TodoItem, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) FindItems(ctx context.Context, params domain.ListTasksParams, excludedStatuses []domain.TaskStatus) (*domain.PagedResult, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) (*domain.RecurringTemplate, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) FindRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) UpdateRecurringTemplate(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) DeleteRecurringTemplate(ctx context.Context, id string) error {
	panic("not used")
}

func (m *mockDeleteItemRepo) FindRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*domain.RecurringTemplate, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) BatchInsertItemsIgnoreConflict(ctx context.Context, items []*domain.TodoItem) (int, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) DeleteFuturePendingItems(ctx context.Context, templateID string, fromDate time.Time) (int64, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) FindStaleTemplates(ctx context.Context, listID string, untilDate time.Time) ([]*domain.RecurringTemplate, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) SetGeneratedThrough(ctx context.Context, templateID string, generatedThrough time.Time) error {
	panic("not used")
}

func (m *mockDeleteItemRepo) CreateGenerationJob(ctx context.Context, job *domain.GenerationJob) error {
	panic("not used")
}

func (m *mockDeleteItemRepo) ListExceptions(ctx context.Context, templateID string, from, until time.Time) ([]*domain.RecurringTemplateException, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) FindExceptionByOccurrence(ctx context.Context, templateID string, occursAt time.Time) (*domain.RecurringTemplateException, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) DeleteException(ctx context.Context, templateID string, occursAt time.Time) error {
	panic("not used")
}

func (m *mockDeleteItemRepo) ListAllExceptionsByTemplate(ctx context.Context, templateID string) ([]*domain.RecurringTemplateException, error) {
	panic("not used")
}

// === Composite Operations (stub implementations) ===
// These tests don't exercise composite operations, so we provide stubs.
// Integration tests in tests/integration/postgres/composite_operations_test.go
// provide comprehensive coverage of these operations.

func (m *mockDeleteItemRepo) UpdateItemWithException(ctx context.Context, params domain.UpdateItemParams, exception *domain.RecurringTemplateException) (*domain.TodoItem, error) {
	panic("not used in these tests - see integration tests for coverage")
}

func (m *mockDeleteItemRepo) DeleteItemWithException(ctx context.Context, listID string, itemID string, exception *domain.RecurringTemplateException) error {
	panic("not used in these tests - see integration tests for coverage")
}

func (m *mockDeleteItemRepo) CreateTemplateWithInitialGeneration(ctx context.Context, template *domain.RecurringTemplate, syncItems []*domain.TodoItem, syncEnd time.Time, asyncJob *domain.GenerationJob) (*domain.RecurringTemplate, error) {
	panic("not used in these tests - see integration tests for coverage")
}

func (m *mockDeleteItemRepo) UpdateTemplateWithRegeneration(ctx context.Context, params domain.UpdateRecurringTemplateParams, deleteFrom time.Time, syncItems []*domain.TodoItem, syncEnd time.Time) (*domain.RecurringTemplate, error) {
	panic("not used in these tests - see integration tests for coverage")
}

func TestDeleteItem_RecurringItem_CreatesException(t *testing.T) {
	templateID := uuid.NewString()
	occursAt := time.Now().UTC().Truncate(time.Second)
	itemID := uuid.NewString()
	listID := uuid.NewString()

	item := &domain.TodoItem{
		ID:                  itemID,
		ListID:              listID,
		Title:               "Recurring Task",
		Status:              domain.TaskStatusTodo,
		RecurringTemplateID: &templateID,
		OccursAt:            &occursAt,
	}

	var capturedExc *domain.RecurringTemplateException
	var capturedUpdateParams domain.UpdateItemParams

	repo := &mockDeleteItemRepo{
		findItemFn: func(ctx context.Context, id string) (*domain.TodoItem, error) {
			return item, nil
		},
		createExceptionFn: func(ctx context.Context, exc *domain.RecurringTemplateException) (*domain.RecurringTemplateException, error) {
			capturedExc = exc
			return exc, nil
		},
		updateItemFn: func(ctx context.Context, params domain.UpdateItemParams) (*domain.TodoItem, error) {
			capturedUpdateParams = params
			archived := domain.TaskStatusArchived
			item.Status = archived
			return item, nil
		},
	}

	service := NewService(repo, nil, Config{})

	err := service.DeleteItem(context.Background(), listID, itemID)

	require.NoError(t, err)

	// Verify exception created
	require.NotNil(t, capturedExc)
	assert.Equal(t, templateID, capturedExc.TemplateID)
	assert.Equal(t, occursAt, capturedExc.OccursAt)
	assert.Equal(t, domain.ExceptionTypeDeleted, capturedExc.ExceptionType)
	assert.NotNil(t, capturedExc.ItemID)
	assert.Equal(t, itemID, *capturedExc.ItemID)

	// Verify item archived
	assert.Equal(t, itemID, capturedUpdateParams.ItemID)
	assert.Equal(t, listID, capturedUpdateParams.ListID)
	require.Contains(t, capturedUpdateParams.UpdateMask, "status")
	assert.Equal(t, domain.TaskStatusArchived, *capturedUpdateParams.Status)
}

func TestUpdateItem_EditRecurringItem_CreatesException(t *testing.T) {
	templateID := uuid.NewString()
	occursAt := time.Now().UTC().Truncate(time.Second)
	itemID := uuid.NewString()
	listID := uuid.NewString()

	item := &domain.TodoItem{
		ID:                  itemID,
		ListID:              listID,
		Title:               "Original Title",
		Status:              domain.TaskStatusTodo,
		RecurringTemplateID: &templateID,
		OccursAt:            &occursAt,
		Version:             1,
	}

	var capturedExc *domain.RecurringTemplateException

	repo := &mockDeleteItemRepo{
		findItemFn: func(ctx context.Context, id string) (*domain.TodoItem, error) {
			return item, nil
		},
		createExceptionFn: func(ctx context.Context, exc *domain.RecurringTemplateException) (*domain.RecurringTemplateException, error) {
			capturedExc = exc
			return exc, nil
		},
		updateItemFn: func(ctx context.Context, params domain.UpdateItemParams) (*domain.TodoItem, error) {
			item.Title = *params.Title
			item.Version = 2
			return item, nil
		},
	}

	service := NewService(repo, nil, Config{})

	newTitle := "Edited Title"
	params := domain.UpdateItemParams{
		ItemID:     itemID,
		ListID:     listID,
		UpdateMask: []string{"title"},
		Title:      &newTitle,
	}

	_, err := service.UpdateItem(context.Background(), params)

	require.NoError(t, err)

	// Verify exception created
	require.NotNil(t, capturedExc)
	assert.Equal(t, templateID, capturedExc.TemplateID)
	assert.Equal(t, occursAt, capturedExc.OccursAt)
	assert.Equal(t, domain.ExceptionTypeEdited, capturedExc.ExceptionType)
	assert.Equal(t, itemID, *capturedExc.ItemID)

	// Verify item still linked to template (NOT detached)
	assert.NotNil(t, item.RecurringTemplateID)
	assert.Equal(t, templateID, *item.RecurringTemplateID)
}

// Note: Reschedule test commented out - UpdateItemParams doesn't currently support
// updating starts_at/occurs_at fields. This would need to be added to fully support
// the rescheduling flow. For now, the shouldDetachFromTemplate logic already handles
// this conceptually.
