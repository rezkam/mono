package todo

import (
	"context"
	"testing"

	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFindListsRepo is a minimal mock for testing FindLists logic
type mockFindListsRepo struct {
	capturedParams domain.ListListsParams
	resultToReturn *domain.PagedListResult
}

func (m *mockFindListsRepo) CreateList(ctx context.Context, list *domain.TodoList) error {
	panic("not used in FindLists tests")
}

func (m *mockFindListsRepo) FindListByID(ctx context.Context, id string) (*domain.TodoList, error) {
	panic("not used in FindLists tests")
}

func (m *mockFindListsRepo) FindAllLists(ctx context.Context) ([]*domain.TodoList, error) {
	panic("not used in FindLists tests")
}

func (m *mockFindListsRepo) FindLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error) {
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

func (m *mockFindListsRepo) UpdateList(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error) {
	panic("not used in FindLists tests")
}

func (m *mockFindListsRepo) CreateItem(ctx context.Context, listID string, item *domain.TodoItem) error {
	panic("not used in FindLists tests")
}

func (m *mockFindListsRepo) FindItemByID(ctx context.Context, id string) (*domain.TodoItem, error) {
	panic("not used in FindLists tests")
}

func (m *mockFindListsRepo) UpdateItem(ctx context.Context, params domain.UpdateItemParams) (*domain.TodoItem, error) {
	panic("not used in FindLists tests")
}

func (m *mockFindListsRepo) FindItems(ctx context.Context, params domain.ListTasksParams, excludedStatuses []domain.TaskStatus) (*domain.PagedResult, error) {
	panic("not used in FindLists tests")
}

func (m *mockFindListsRepo) CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) error {
	panic("not used in FindLists tests")
}

func (m *mockFindListsRepo) FindRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	panic("not used in FindLists tests")
}

func (m *mockFindListsRepo) UpdateRecurringTemplate(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error) {
	panic("not used in FindLists tests")
}

func (m *mockFindListsRepo) DeleteRecurringTemplate(ctx context.Context, id string) error {
	panic("not used in FindLists tests")
}

func (m *mockFindListsRepo) FindRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*domain.RecurringTemplate, error) {
	panic("not used in FindLists tests")
}

// mockUpdateItemRepo is a minimal mock for testing UpdateItem logic
type mockUpdateItemRepo struct {
	mockFindListsRepo // embed for interface satisfaction
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
	service := NewService(repo, Config{DefaultPageSize: 25, MaxPageSize: 100})

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

// TestFindLists_ClampsNegativeOffset tests that FindLists rejects negative
// offsets by clamping them to 0, preventing PostgreSQL errors.
func TestFindLists_ClampsNegativeOffset(t *testing.T) {
	repo := &mockFindListsRepo{}
	service := NewService(repo, Config{DefaultPageSize: 25, MaxPageSize: 100})

	params := domain.ListListsParams{
		Offset: -5, // Negative offset should be clamped to 0
		Limit:  10,
	}

	_, err := service.FindLists(context.Background(), params)
	require.NoError(t, err)

	// Verify that the offset passed to the repository was clamped to 0
	assert.Equal(t, 0, repo.capturedParams.Offset, "negative offset should be clamped to 0")
}
