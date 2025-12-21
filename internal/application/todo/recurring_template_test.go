package todo

import (
	"context"
	"testing"

	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRecurringRepo is a minimal mock for testing validation logic
type mockRecurringRepo struct {
	createTemplateFn func(ctx context.Context, template *domain.RecurringTemplate) error
	findTemplateFn   func(ctx context.Context, id string) (*domain.RecurringTemplate, error)
	updateTemplateFn func(ctx context.Context, template *domain.RecurringTemplate) error
}

func (m *mockRecurringRepo) CreateList(ctx context.Context, list *domain.TodoList) error {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) FindListByID(ctx context.Context, id string) (*domain.TodoList, error) {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) FindAllLists(ctx context.Context) ([]*domain.TodoList, error) {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) FindLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error) {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) UpdateList(ctx context.Context, list *domain.TodoList) error {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) CreateItem(ctx context.Context, listID string, item *domain.TodoItem) error {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) FindItemByID(ctx context.Context, id string) (*domain.TodoItem, error) {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) UpdateItem(ctx context.Context, listID string, item *domain.TodoItem) error {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) FindItems(ctx context.Context, params domain.ListTasksParams) (*domain.PagedResult, error) {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) error {
	if m.createTemplateFn != nil {
		return m.createTemplateFn(ctx, template)
	}
	return nil
}

func (m *mockRecurringRepo) FindRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	if m.findTemplateFn != nil {
		return m.findTemplateFn(ctx, id)
	}
	panic("FindRecurringTemplate not mocked")
}

func (m *mockRecurringRepo) UpdateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) error {
	if m.updateTemplateFn != nil {
		return m.updateTemplateFn(ctx, template)
	}
	return nil
}

func (m *mockRecurringRepo) DeleteRecurringTemplate(ctx context.Context, id string) error {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) FindRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*domain.RecurringTemplate, error) {
	panic("not used in recurring template tests")
}

// TestCreateRecurringTemplate_RejectsInvalidRecurrencePattern tests that
// CreateRecurringTemplate validates recurrence_pattern against known values.
func TestCreateRecurringTemplate_RejectsInvalidRecurrencePattern(t *testing.T) {
	repo := &mockRecurringRepo{}
	service := NewService(repo, Config{})

	template := &domain.RecurringTemplate{
		ListID:               "list-123",
		Title:                "Weekly Team Meeting",
		RecurrencePattern:    "invalid-pattern", // Invalid pattern
		GenerationWindowDays: 30,
	}

	_, err := service.CreateRecurringTemplate(context.Background(), template)

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidRecurrencePattern)
}

// TestCreateRecurringTemplate_AcceptsValidRecurrencePatterns tests that
// all valid recurrence patterns are accepted.
func TestCreateRecurringTemplate_AcceptsValidRecurrencePatterns(t *testing.T) {
	validPatterns := []domain.RecurrencePattern{
		domain.RecurrenceDaily,
		domain.RecurrenceWeekly,
		domain.RecurrenceBiweekly,
		domain.RecurrenceMonthly,
		domain.RecurrenceYearly,
		domain.RecurrenceQuarterly,
		domain.RecurrenceWeekdays,
	}

	for _, pattern := range validPatterns {
		t.Run(string(pattern), func(t *testing.T) {
			repo := &mockRecurringRepo{}
			service := NewService(repo, Config{})

			template := &domain.RecurringTemplate{
				ListID:               "list-123",
				Title:                "Test Template",
				RecurrencePattern:    pattern,
				GenerationWindowDays: 30,
			}

			_, err := service.CreateRecurringTemplate(context.Background(), template)

			require.NoError(t, err, "valid pattern %s should be accepted", pattern)
		})
	}
}

// TestUpdateRecurringTemplate_RejectsInvalidRecurrencePattern tests that
// UpdateRecurringTemplate validates recurrence_pattern when it's being updated.
func TestUpdateRecurringTemplate_RejectsInvalidRecurrencePattern(t *testing.T) {
	repo := &mockRecurringRepo{}
	service := NewService(repo, Config{})

	template := &domain.RecurringTemplate{
		ID:                "template-123",
		ListID:            "list-123",
		Title:             "Weekly Team Meeting",
		RecurrencePattern: "invalid-pattern", // Invalid pattern
	}

	err := service.UpdateRecurringTemplate(context.Background(), template)

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidRecurrencePattern)
}
