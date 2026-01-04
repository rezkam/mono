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

// Atomic executes callback without transaction (tests don't need real transactions)
func (m *mockDeleteItemRepo) Atomic(ctx context.Context, fn func(tx Repository) error) error {
	if m.transactionFn != nil {
		return m.transactionFn(ctx, fn)
	}
	// Default: execute without actual transaction
	return fn(m)
}

// AtomicRecurring executes callback without transaction (tests don't need real transactions)
func (m *mockDeleteItemRepo) AtomicRecurring(ctx context.Context, fn func(ops RecurringOperations) error) error {
	// Not used in delete item tests
	panic("AtomicRecurring not used in delete item tests")
}

// Stub all other methods
func (m *mockDeleteItemRepo) CreateList(ctx context.Context, list *domain.TodoList) (*domain.TodoList, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) FindListByID(ctx context.Context, id string) (*domain.TodoList, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) FindLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error) {
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

func (m *mockDeleteItemRepo) DeleteItem(ctx context.Context, id string) error {
	panic("not used")
}

func (m *mockDeleteItemRepo) CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) (*domain.RecurringTemplate, error) {
	panic("not used")
}

func (m *mockDeleteItemRepo) FindRecurringTemplateByID(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
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
