package todo

import (
	"context"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockListListsRepo is a minimal mock for testing ListLists logic
type mockListListsRepo struct {
	capturedParams domain.ListListsParams
	resultToReturn *domain.PagedListResult
}

func (m *mockListListsRepo) CreateList(ctx context.Context, list *domain.TodoList) (*domain.TodoList, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) FindListByID(ctx context.Context, id string) (*domain.TodoList, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) ListLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error) {
	// Capture params for assertion
	m.capturedParams = params
	if m.resultToReturn != nil {
		return m.resultToReturn, nil
	}
	return &domain.PagedListResult{
		Lists:      []*domain.TodoList{},
		TotalCount: 0,
		HasMore:    false,
	}, nil
}

func (m *mockListListsRepo) UpdateList(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) CreateItem(ctx context.Context, listID string, item *domain.TodoItem) (*domain.TodoItem, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) FindItemByID(ctx context.Context, id string) (*domain.TodoItem, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) UpdateItem(ctx context.Context, params domain.UpdateItemParams) (*domain.TodoItem, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) FindItems(ctx context.Context, params domain.ListTasksParams, excludedStatuses []domain.TaskStatus) (*domain.PagedResult, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) (*domain.RecurringTemplate, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) FindRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) UpdateRecurringTemplate(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) DeleteRecurringTemplate(ctx context.Context, id string) error {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) FindRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*domain.RecurringTemplate, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) BatchInsertItemsIgnoreConflict(ctx context.Context, items []*domain.TodoItem) (int, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) DeleteFuturePendingItems(ctx context.Context, templateID string, fromDate time.Time) (int64, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) FindStaleTemplates(ctx context.Context, listID string, untilDate time.Time) ([]*domain.RecurringTemplate, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) SetGeneratedThrough(ctx context.Context, templateID string, generatedThrough time.Time) error {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) CreateGenerationJob(ctx context.Context, job *domain.GenerationJob) error {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) CreateException(ctx context.Context, exception *domain.RecurringTemplateException) (*domain.RecurringTemplateException, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) ListExceptions(ctx context.Context, templateID string, from, until time.Time) ([]*domain.RecurringTemplateException, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) FindExceptionByOccurrence(ctx context.Context, templateID string, occursAt time.Time) (*domain.RecurringTemplateException, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) DeleteException(ctx context.Context, templateID string, occursAt time.Time) error {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) ListAllExceptionsByTemplate(ctx context.Context, templateID string) ([]*domain.RecurringTemplateException, error) {
	panic("not used in ListLists tests")
}

func (m *mockListListsRepo) Transaction(ctx context.Context, fn func(tx Repository) error) error {
	// Execute the function with the same mock (no actual transaction needed for validation tests)
	return fn(m)
}

// === Composite Operations (stub implementations) ===
// These tests don't exercise composite operations, so we provide stubs.
// Integration tests in tests/integration/postgres/composite_operations_test.go
// provide comprehensive coverage of these operations.

func (m *mockListListsRepo) UpdateItemWithException(ctx context.Context, params domain.UpdateItemParams, exception *domain.RecurringTemplateException) (*domain.TodoItem, error) {
	panic("not used in these tests - see integration tests for coverage")
}

func (m *mockListListsRepo) DeleteItemWithException(ctx context.Context, listID string, itemID string, exception *domain.RecurringTemplateException) error {
	panic("not used in these tests - see integration tests for coverage")
}

func (m *mockListListsRepo) CreateTemplateWithInitialGeneration(ctx context.Context, template *domain.RecurringTemplate, syncItems []*domain.TodoItem, syncEnd time.Time, asyncJob *domain.GenerationJob) (*domain.RecurringTemplate, error) {
	panic("not used in these tests - see integration tests for coverage")
}

func (m *mockListListsRepo) UpdateTemplateWithRegeneration(ctx context.Context, params domain.UpdateRecurringTemplateParams, deleteFrom time.Time, syncItems []*domain.TodoItem, syncEnd time.Time) (*domain.RecurringTemplate, error) {
	panic("not used in these tests - see integration tests for coverage")
}

// mockUpdateItemRepo is a minimal mock for testing UpdateItem logic
type mockUpdateItemRepo struct {
	mockListListsRepo // embed for interface satisfaction
}

func (m *mockUpdateItemRepo) UpdateItem(ctx context.Context, params domain.UpdateItemParams) (*domain.TodoItem, error) {
	return &domain.TodoItem{}, nil
}

func (m *mockUpdateItemRepo) FindItemByID(ctx context.Context, id string) (*domain.TodoItem, error) {
	return &domain.TodoItem{}, nil
}

// TestUpdateItem_RejectsTrailingNonNumericEtag verifies that etags with trailing
// non-numeric characters (e.g., "123abc") are rejected. This is a regression test
// for fmt.Sscanf("%d") which permissively parses "123abc" as 123.
func TestUpdateItem_RejectsTrailingNonNumericEtag(t *testing.T) {
	repo := &mockUpdateItemRepo{}
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{DefaultPageSize: 25, MaxPageSize: 100})

	testCases := []struct {
		name  string
		etag  string
		valid bool
	}{
		{"valid numeric", "123", true},
		{"valid single digit", "1", true},
		{"trailing letters", "123abc", false},
		{"leading letters", "abc123", false},
		{"mixed", "1a2b3c", false},
		{"with spaces", "123 ", false},
		{"leading spaces", " 123", false},
		{"decimal", "1.5", false},
		{"negative", "-1", false},
		{"zero", "0", false},
		{"empty", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			etag := tc.etag
			params := domain.UpdateItemParams{
				ListID: "list-123",
				ItemID: "item-456",
				Etag:   &etag,
			}

			_, err := service.UpdateItem(context.Background(), params)

			if tc.valid {
				// Valid etags should not return ErrInvalidEtagFormat
				assert.NotErrorIs(t, err, domain.ErrInvalidEtagFormat)
			} else {
				// Invalid etags should return ErrInvalidEtagFormat
				assert.ErrorIs(t, err, domain.ErrInvalidEtagFormat,
					"etag %q should be rejected as invalid", tc.etag)
			}
		})
	}
}

// TestListLists_ClampsNegativeOffset tests that FindLists rejects negative
// offsets by clamping them to 0, preventing PostgreSQL errors.
func TestListLists_ClampsNegativeOffset(t *testing.T) {
	repo := &mockListListsRepo{}
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{DefaultPageSize: 25, MaxPageSize: 100})

	params := domain.ListListsParams{
		Offset: -5, // Negative offset should be clamped to 0
		Limit:  10,
	}

	_, err := service.ListLists(context.Background(), params)
	require.NoError(t, err)

	// Verify that the offset passed to the repository was clamped to 0
	assert.Equal(t, 0, repo.capturedParams.Offset, "negative offset should be clamped to 0")
}

// TestListLists_UsesConfiguredDefaultPageSize verifies that when no limit is specified,
// the service uses the configured DefaultPageSize, not the compile-time constant.
// This ensures MONO_DEFAULT_PAGE_SIZE env var takes effect.
func TestListLists_UsesConfiguredDefaultPageSize(t *testing.T) {
	repo := &mockListListsRepo{}

	// Use non-default values to prove config is used
	customDefault := 50
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{DefaultPageSize: customDefault, MaxPageSize: 200})

	params := domain.ListListsParams{
		Limit: 0, // Zero means "use default"
	}

	_, err := service.ListLists(context.Background(), params)
	require.NoError(t, err)

	assert.Equal(t, customDefault, repo.capturedParams.Limit,
		"should use configured DefaultPageSize (%d), not compile-time constant", customDefault)
}

// TestListLists_UsesConfiguredMaxPageSize verifies that when limit exceeds max,
// the service clamps to the configured MaxPageSize, not the compile-time constant.
// This ensures MONO_MAX_PAGE_SIZE env var takes effect.
func TestListLists_UsesConfiguredMaxPageSize(t *testing.T) {
	repo := &mockListListsRepo{}

	// Use non-default values to prove config is used
	customMax := 50
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{DefaultPageSize: 10, MaxPageSize: customMax})

	params := domain.ListListsParams{
		Limit: 1000, // Exceeds max
	}

	_, err := service.ListLists(context.Background(), params)
	require.NoError(t, err)

	assert.Equal(t, customMax, repo.capturedParams.Limit,
		"should clamp to configured MaxPageSize (%d), not compile-time constant", customMax)
}

// TestListLists_RespectsValidLimit verifies that valid limits within range are passed through.
func TestListLists_RespectsValidLimit(t *testing.T) {
	repo := &mockListListsRepo{}
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{DefaultPageSize: 25, MaxPageSize: 100})

	params := domain.ListListsParams{
		Limit: 42, // Within valid range
	}

	_, err := service.ListLists(context.Background(), params)
	require.NoError(t, err)

	assert.Equal(t, 42, repo.capturedParams.Limit, "valid limit should be passed through unchanged")
}

// TestUpdateItem_RejectsTitleInMaskWithNilValue verifies that when "title" is in
// update_mask but Title is nil, the service returns ErrTitleRequired.
// This prevents a nil-dereference panic in the repository layer.
func TestUpdateItem_RejectsTitleInMaskWithNilValue(t *testing.T) {
	repo := &mockUpdateItemRepo{}
	generator := &mockTaskGenerator{}
	service := NewService(repo, generator, Config{DefaultPageSize: 25, MaxPageSize: 100})

	params := domain.UpdateItemParams{
		ListID:     "list-123",
		ItemID:     "item-456",
		UpdateMask: []string{"title"}, // title in mask
		Title:      nil,               // but Title is nil - should be rejected
	}

	_, err := service.UpdateItem(context.Background(), params)

	assert.ErrorIs(t, err, domain.ErrTitleRequired,
		"should return ErrTitleRequired when title is in update_mask but Title is nil")
}
