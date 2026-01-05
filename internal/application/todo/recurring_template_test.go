package todo

import (
	"context"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTaskGenerator is a minimal mock for TaskGenerator
type mockTaskGenerator struct{}

func (m *mockTaskGenerator) GenerateTasksForTemplate(ctx context.Context, template *domain.RecurringTemplate, start, end time.Time) ([]*domain.TodoItem, error) {
	// For validation tests, we don't actually need to generate tasks
	return nil, nil
}

func (m *mockTaskGenerator) GenerateTasksForTemplateWithExceptions(ctx context.Context, template *domain.RecurringTemplate, start, end time.Time, exceptions []*domain.RecurringTemplateException) ([]*domain.TodoItem, error) {
	// For validation tests, we don't actually need to generate tasks
	return nil, nil
}

// mockRecurringRepo is a minimal mock for testing validation logic
type mockRecurringRepo struct {
	createTemplateFn func(ctx context.Context, template *domain.RecurringTemplate) (*domain.RecurringTemplate, error)
	findTemplateFn   func(ctx context.Context, id string) (*domain.RecurringTemplate, error)
	updateTemplateFn func(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error)
	updateListFn     func(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error)
}

func (m *mockRecurringRepo) CreateList(ctx context.Context, list *domain.TodoList) (*domain.TodoList, error) {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) FindListByID(ctx context.Context, id string) (*domain.TodoList, error) {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) FindLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error) {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) UpdateList(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error) {
	if m.updateListFn != nil {
		return m.updateListFn(ctx, params)
	}
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) CreateItem(ctx context.Context, listID string, item *domain.TodoItem) (*domain.TodoItem, error) {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) FindItemByID(ctx context.Context, id string) (*domain.TodoItem, error) {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) UpdateItem(ctx context.Context, params domain.UpdateItemParams) (*domain.TodoItem, error) {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) FindItems(ctx context.Context, params domain.ListTasksParams, excludedStatuses []domain.TaskStatus) (*domain.PagedResult, error) {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) DeleteItem(ctx context.Context, id string) error {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) (*domain.RecurringTemplate, error) {
	if m.createTemplateFn != nil {
		return m.createTemplateFn(ctx, template)
	}
	return template, nil
}

func (m *mockRecurringRepo) FindRecurringTemplateByID(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	if m.findTemplateFn != nil {
		return m.findTemplateFn(ctx, id)
	}
	panic("FindRecurringTemplate not mocked")
}

func (m *mockRecurringRepo) UpdateRecurringTemplate(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error) {
	if m.updateTemplateFn != nil {
		return m.updateTemplateFn(ctx, params)
	}
	return &domain.RecurringTemplate{ID: params.TemplateID}, nil
}

func (m *mockRecurringRepo) DeleteRecurringTemplate(ctx context.Context, id string) error {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) FindRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*domain.RecurringTemplate, error) {
	panic("not used in recurring template tests")
}

func (m *mockRecurringRepo) CreateException(ctx context.Context, exception *domain.RecurringTemplateException) (*domain.RecurringTemplateException, error) {
	panic("not used in recurring template tests")
}

// Atomic executes callback without transaction (tests don't need real transactions)
func (m *mockRecurringRepo) Atomic(ctx context.Context, fn func(tx Repository) error) error {
	// Execute the function with the same mock (no actual transaction needed for validation tests)
	return fn(m)
}

// AtomicRecurring executes callback without transaction (tests don't need real transactions)
func (m *mockRecurringRepo) AtomicRecurring(ctx context.Context, fn func(ops RecurringOperations) error) error {
	// Execute the function with the same mock (no actual transaction needed for validation tests)
	return fn(m)
}

// RecurringOperations methods (stub implementations for AtomicRecurring callbacks)
func (m *mockRecurringRepo) BatchInsertItemsIgnoreConflict(ctx context.Context, items []*domain.TodoItem) (int, error) {
	return len(items), nil // Return success
}

func (m *mockRecurringRepo) DeleteFuturePendingItems(ctx context.Context, templateID string, from time.Time) (int64, error) {
	return 0, nil // Return success
}

func (m *mockRecurringRepo) SetGeneratedThrough(ctx context.Context, templateID string, generatedThrough time.Time) error {
	return nil // Return success
}

func (m *mockRecurringRepo) ScheduleGenerationJob(ctx context.Context, templateID string, scheduledFor, from, until time.Time) (string, error) {
	return "job-123", nil // Return mock job ID
}

// TestCreateRecurringTemplate_RejectsInvalidRecurrencePattern tests that
// CreateRecurringTemplate validates recurrence_pattern against known values.
func TestCreateRecurringTemplate_RejectsInvalidRecurrencePattern(t *testing.T) {
	repo := &mockRecurringRepo{}
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{})

	template := &domain.RecurringTemplate{
		ListID:                "list-123",
		Title:                 "Weekly Team Meeting",
		RecurrencePattern:     "invalid-pattern", // Invalid pattern
		SyncHorizonDays:       14,
		GenerationHorizonDays: 365,
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
			var createdTemplate *domain.RecurringTemplate
			repo := &mockRecurringRepo{
				createTemplateFn: func(ctx context.Context, template *domain.RecurringTemplate) (*domain.RecurringTemplate, error) {
					createdTemplate = template
					return template, nil
				},
				findTemplateFn: func(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
					if createdTemplate != nil && createdTemplate.ID == id {
						return createdTemplate, nil
					}
					return nil, domain.ErrTemplateNotFound
				},
			}
			generator := &mockTaskGenerator{}
			service := NewService(repo, generator, Config{})

			template := &domain.RecurringTemplate{
				ListID:                "list-123",
				Title:                 "Test Template",
				RecurrencePattern:     pattern,
				SyncHorizonDays:       14,
				GenerationHorizonDays: 365,
			}

			_, err := service.CreateRecurringTemplate(context.Background(), template)

			require.NoError(t, err, "valid pattern %s should be accepted", pattern)
		})
	}
}

// TestUpdateRecurringTemplate_RejectsInvalidRecurrencePattern tests that
// UpdateRecurringTemplate validates recurrence_pattern when it's being updated.
func TestUpdateRecurringTemplate_RejectsInvalidRecurrencePattern(t *testing.T) {
	repo := &mockRecurringRepo{
		findTemplateFn: func(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
			return &domain.RecurringTemplate{
				ID:     id,
				ListID: "list-123",
			}, nil
		},
	}
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{})

	invalidPattern := domain.RecurrencePattern("invalid-pattern")
	params := domain.UpdateRecurringTemplateParams{
		TemplateID:        "template-123",
		ListID:            "list-123", // Must match template's list for ownership check
		UpdateMask:        []string{"recurrence_pattern"},
		RecurrencePattern: &invalidPattern,
	}

	_, err := service.UpdateRecurringTemplate(context.Background(), params)

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidRecurrencePattern)
}

// ============================================================================
// VALIDATION BYPASS PREVENTION TESTS
// ============================================================================
// These tests verify that empty strings are properly validated when sent via
// pointer fields (defense in depth). Previously, the service layer used
// `if title != ""` which skipped validation for empty strings. Now with
// pointer semantics (`if params.Title != nil`), validation is always applied.

// TestUpdateList_RejectsEmptyTitle tests that UpdateList validates empty titles
// instead of silently skipping validation.
func TestUpdateList_RejectsEmptyTitle(t *testing.T) {
	repo := &mockRecurringRepo{
		updateListFn: func(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error) {
			t.Error("repository should not be called when validation fails")
			return nil, nil
		},
	}
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{})

	params := domain.UpdateListParams{
		ListID:     "list-123",
		UpdateMask: []string{"title"},
		Title:      ptr.To(""), // Empty string should be rejected
	}

	_, err := service.UpdateList(context.Background(), params)

	require.Error(t, err, "empty title should be rejected")
	assert.ErrorIs(t, err, domain.ErrTitleRequired)
}

// TestUpdateList_RejectsWhitespaceOnlyTitle tests that titles with only
// whitespace are rejected.
func TestUpdateList_RejectsWhitespaceOnlyTitle(t *testing.T) {
	repo := &mockRecurringRepo{
		updateListFn: func(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error) {
			t.Error("repository should not be called when validation fails")
			return nil, nil
		},
	}
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{})

	params := domain.UpdateListParams{
		ListID:     "list-123",
		UpdateMask: []string{"title"},
		Title:      ptr.To("   "), // Whitespace only should be rejected
	}

	_, err := service.UpdateList(context.Background(), params)

	require.Error(t, err, "whitespace-only title should be rejected")
	assert.ErrorIs(t, err, domain.ErrTitleRequired)
}

// TestUpdateList_AcceptsValidTitle verifies that valid titles pass validation.
func TestUpdateList_AcceptsValidTitle(t *testing.T) {
	var capturedTitle string
	repo := &mockRecurringRepo{
		updateListFn: func(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error) {
			capturedTitle = *params.Title
			return &domain.TodoList{ID: params.ListID, Title: capturedTitle}, nil
		},
	}
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{})

	params := domain.UpdateListParams{
		ListID:     "list-123",
		UpdateMask: []string{"title"},
		Title:      ptr.To("Valid Title"),
	}

	result, err := service.UpdateList(context.Background(), params)

	require.NoError(t, err)
	assert.Equal(t, "Valid Title", capturedTitle)
	assert.Equal(t, "Valid Title", result.Title)
}

// TestUpdateList_RejectsEmptyUpdateMask verifies that empty update_mask
// is rejected before reaching the repository.
func TestUpdateList_RejectsEmptyUpdateMask(t *testing.T) {
	repo := &mockRecurringRepo{
		updateListFn: func(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error) {
			t.Error("repository should not be called when update_mask is empty")
			return nil, nil
		},
	}
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{})

	params := domain.UpdateListParams{
		ListID:     "list-123",
		UpdateMask: []string{}, // Empty mask should be rejected
		Title:      nil,
	}

	_, err := service.UpdateList(context.Background(), params)

	require.ErrorIs(t, err, domain.ErrEmptyUpdateMask)
}

// TestUpdateRecurringTemplate_RejectsEmptyTitle tests that UpdateRecurringTemplate
// validates empty titles instead of silently skipping validation.
func TestUpdateRecurringTemplate_RejectsEmptyTitle(t *testing.T) {
	repo := &mockRecurringRepo{
		findTemplateFn: func(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
			return &domain.RecurringTemplate{ID: id, ListID: "list-123"}, nil
		},
		updateTemplateFn: func(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error) {
			t.Error("repository should not be called when validation fails")
			return nil, nil
		},
	}
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{})

	params := domain.UpdateRecurringTemplateParams{
		TemplateID: "template-123",
		ListID:     "list-123",
		UpdateMask: []string{"title"},
		Title:      ptr.To(""), // Empty string should be rejected
	}

	_, err := service.UpdateRecurringTemplate(context.Background(), params)

	require.Error(t, err, "empty title should be rejected")
	assert.ErrorIs(t, err, domain.ErrTitleRequired)
}

// TestUpdateRecurringTemplate_RejectsWhitespaceOnlyTitle tests that templates
// with whitespace-only titles are rejected.
func TestUpdateRecurringTemplate_RejectsWhitespaceOnlyTitle(t *testing.T) {
	repo := &mockRecurringRepo{
		findTemplateFn: func(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
			return &domain.RecurringTemplate{ID: id, ListID: "list-123"}, nil
		},
		updateTemplateFn: func(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error) {
			t.Error("repository should not be called when validation fails")
			return nil, nil
		},
	}
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{})

	params := domain.UpdateRecurringTemplateParams{
		TemplateID: "template-123",
		ListID:     "list-123",
		UpdateMask: []string{"title"},
		Title:      ptr.To("   "), // Whitespace only should be rejected
	}

	_, err := service.UpdateRecurringTemplate(context.Background(), params)

	require.Error(t, err, "whitespace-only title should be rejected")
	assert.ErrorIs(t, err, domain.ErrTitleRequired)
}

// TestUpdateRecurringTemplate_AcceptsValidTitle verifies that valid titles pass.
func TestUpdateRecurringTemplate_AcceptsValidTitle(t *testing.T) {
	var capturedTitle string
	repo := &mockRecurringRepo{
		findTemplateFn: func(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
			return &domain.RecurringTemplate{ID: id, ListID: "list-123"}, nil
		},
		updateTemplateFn: func(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error) {
			capturedTitle = *params.Title
			return &domain.RecurringTemplate{
				ID:    params.TemplateID,
				Title: capturedTitle,
			}, nil
		},
	}
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{})

	params := domain.UpdateRecurringTemplateParams{
		TemplateID: "template-123",
		ListID:     "list-123",
		UpdateMask: []string{"title"},
		Title:      ptr.To("Valid Template Title"),
	}

	result, err := service.UpdateRecurringTemplate(context.Background(), params)

	require.NoError(t, err)
	assert.Equal(t, "Valid Template Title", capturedTitle)
	assert.Equal(t, "Valid Template Title", result.Title)
}

// TestCreateRecurringTemplate_RejectsNegativeSyncHorizonDays tests that
// negative SyncHorizonDays values are rejected during template creation.
func TestCreateRecurringTemplate_RejectsNegativeSyncHorizonDays(t *testing.T) {
	repo := &mockRecurringRepo{}
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{})

	template := &domain.RecurringTemplate{
		ListID:                "list-123",
		Title:                 "Test Template",
		RecurrencePattern:     domain.RecurrenceDaily,
		SyncHorizonDays:       -5, // Negative value should be rejected
		GenerationHorizonDays: 365,
	}

	_, err := service.CreateRecurringTemplate(context.Background(), template)

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrSyncHorizonMustBePositive)
}

// TestUpdateRecurringTemplate_ValidatesMultipleFields tests that validation
// is applied to all fields in the update mask.
func TestUpdateRecurringTemplate_ValidatesMultipleFields(t *testing.T) {
	tests := []struct {
		name      string
		params    domain.UpdateRecurringTemplateParams
		wantError error
	}{
		{
			name: "empty title with valid pattern",
			params: domain.UpdateRecurringTemplateParams{
				TemplateID:        "template-123",
				ListID:            "list-123",
				UpdateMask:        []string{"title", "recurrence_pattern"},
				Title:             ptr.To(""),
				RecurrencePattern: ptr.To(domain.RecurrenceDaily),
			},
			wantError: domain.ErrTitleRequired,
		},
		{
			name: "valid title with invalid pattern",
			params: domain.UpdateRecurringTemplateParams{
				TemplateID:        "template-123",
				ListID:            "list-123",
				UpdateMask:        []string{"title", "recurrence_pattern"},
				Title:             ptr.To("Valid Title"),
				RecurrencePattern: ptr.To(domain.RecurrencePattern("invalid")),
			},
			wantError: domain.ErrInvalidRecurrencePattern,
		},
		{
			name: "negative sync_horizon_days",
			params: domain.UpdateRecurringTemplateParams{
				TemplateID:      "template-123",
				ListID:          "list-123",
				UpdateMask:      []string{"sync_horizon_days"},
				SyncHorizonDays: ptr.To(-5),
			},
			wantError: domain.ErrSyncHorizonMustBePositive,
		},
		{
			name: "zero sync_horizon_days",
			params: domain.UpdateRecurringTemplateParams{
				TemplateID:      "template-123",
				ListID:          "list-123",
				UpdateMask:      []string{"sync_horizon_days"},
				SyncHorizonDays: ptr.To(0),
			},
			wantError: domain.ErrSyncHorizonMustBePositive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRecurringRepo{
				findTemplateFn: func(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
					return &domain.RecurringTemplate{ID: id, ListID: "list-123"}, nil
				},
				updateTemplateFn: func(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error) {
					t.Error("repository should not be called when validation fails")
					return nil, nil
				},
			}
			generator := &mockTaskGenerator{}
			service := NewService(repo, generator, Config{})

			_, err := service.UpdateRecurringTemplate(context.Background(), tt.params)

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantError)
		})
	}
}
