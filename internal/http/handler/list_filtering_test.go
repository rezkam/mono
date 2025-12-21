package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/http/openapi"
)

// mockListRepository implements todo.Repository for testing ListLists endpoint
type mockListRepository struct {
	stubRepository  // Embed stub for unimplemented methods
	findListsResult *domain.PagedListResult
	findListsParams domain.ListListsParams
	findListsError  error
}

func (m *mockListRepository) FindLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error) {
	m.findListsParams = params
	if m.findListsError != nil {
		return nil, m.findListsError
	}
	return m.findListsResult, nil
}

func TestListLists_WithTitleFilter(t *testing.T) {
	now := time.Now().UTC()

	mockRepo := &mockListRepository{
		findListsResult: &domain.PagedListResult{
			Lists: []*domain.TodoList{
				{
					ID:          "list-1",
					Title:       "Project Alpha",
					CreateTime:  now,
					TotalItems:  5,
					UndoneItems: 3,
				},
				{
					ID:          "list-2",
					Title:       "Project Beta",
					CreateTime:  now.Add(-1 * time.Hour),
					TotalItems:  2,
					UndoneItems: 1,
				},
			},
			TotalCount: 2,
			HasMore:    false,
		},
	}

	service := todo.NewService(mockRepo, todo.Config{})
	server := NewServer(service)

	// Create request with title filter
	filterStr := "title:\"project\""
	req := httptest.NewRequest(http.MethodGet, "/v1/lists", nil)
	w := httptest.NewRecorder()

	params := openapi.ListListsParams{}
	params.Filter = &filterStr

	server.ListLists(w, req, params)

	// Verify response
	require.Equal(t, http.StatusOK, w.Code)

	var resp openapi.ListListsResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.NotNil(t, resp.Lists)
	assert.Len(t, *resp.Lists, 2)

	// Verify the repository was called with correct filter params
	assert.NotNil(t, mockRepo.findListsParams.TitleContains)
	assert.Equal(t, "project", *mockRepo.findListsParams.TitleContains)
}

func TestListLists_WithCreateTimeAfterFilter(t *testing.T) {
	now := time.Now().UTC()
	cutoffTime := now.Add(-24 * time.Hour)

	mockRepo := &mockListRepository{
		findListsResult: &domain.PagedListResult{
			Lists: []*domain.TodoList{
				{
					ID:          "list-1",
					Title:       "Recent List",
					CreateTime:  now,
					TotalItems:  3,
					UndoneItems: 2,
				},
			},
			TotalCount: 1,
			HasMore:    false,
		},
	}

	service := todo.NewService(mockRepo, todo.Config{})
	server := NewServer(service)

	// Create request with create_time filter
	filterStr := "create_time > '" + cutoffTime.Format(time.RFC3339) + "'"
	req := httptest.NewRequest(http.MethodGet, "/v1/lists", nil)
	w := httptest.NewRecorder()

	params := openapi.ListListsParams{}
	params.Filter = &filterStr

	server.ListLists(w, req, params)

	// Verify response
	require.Equal(t, http.StatusOK, w.Code)

	// Verify the repository was called with correct filter params
	assert.NotNil(t, mockRepo.findListsParams.CreateTimeAfter)
	// Allow for sub-second precision differences due to parsing
	assert.WithinDuration(t, cutoffTime, *mockRepo.findListsParams.CreateTimeAfter, time.Second)
}

func TestListLists_WithOrderByCreateTimeDesc(t *testing.T) {
	now := time.Now().UTC()

	mockRepo := &mockListRepository{
		findListsResult: &domain.PagedListResult{
			Lists: []*domain.TodoList{
				{
					ID:         "list-1",
					Title:      "Newest List",
					CreateTime: now,
				},
				{
					ID:         "list-2",
					Title:      "Older List",
					CreateTime: now.Add(-1 * time.Hour),
				},
			},
			TotalCount: 2,
			HasMore:    false,
		},
	}

	service := todo.NewService(mockRepo, todo.Config{})
	server := NewServer(service)

	// Create request with order_by
	orderByStr := "create_time desc"
	req := httptest.NewRequest(http.MethodGet, "/v1/lists", nil)
	w := httptest.NewRecorder()

	params := openapi.ListListsParams{}
	params.OrderBy = &orderByStr

	server.ListLists(w, req, params)

	// Verify response
	require.Equal(t, http.StatusOK, w.Code)

	var resp openapi.ListListsResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.NotNil(t, resp.Lists)
	assert.Len(t, *resp.Lists, 2)

	// Verify the repository was called with correct sort params
	assert.Equal(t, "create_time", mockRepo.findListsParams.OrderBy)
	assert.Equal(t, "desc", mockRepo.findListsParams.OrderDir)
}

func TestListLists_WithOrderByTitleAsc(t *testing.T) {
	mockRepo := &mockListRepository{
		findListsResult: &domain.PagedListResult{
			Lists: []*domain.TodoList{
				{
					ID:    "list-1",
					Title: "Alpha",
				},
				{
					ID:    "list-2",
					Title: "Beta",
				},
				{
					ID:    "list-3",
					Title: "Gamma",
				},
			},
			TotalCount: 3,
			HasMore:    false,
		},
	}

	service := todo.NewService(mockRepo, todo.Config{})
	server := NewServer(service)

	// Create request with order_by title asc
	orderByStr := "title asc"
	req := httptest.NewRequest(http.MethodGet, "/v1/lists", nil)
	w := httptest.NewRecorder()

	params := openapi.ListListsParams{}
	params.OrderBy = &orderByStr

	server.ListLists(w, req, params)

	// Verify response
	require.Equal(t, http.StatusOK, w.Code)

	// Verify the repository was called with correct sort params
	assert.Equal(t, "title", mockRepo.findListsParams.OrderBy)
	assert.Equal(t, "asc", mockRepo.findListsParams.OrderDir)
}

func TestListLists_WithPaginationAndFilters(t *testing.T) {
	now := time.Now().UTC()

	mockRepo := &mockListRepository{
		findListsResult: &domain.PagedListResult{
			Lists: []*domain.TodoList{
				{
					ID:         "list-1",
					Title:      "Project Alpha",
					CreateTime: now,
				},
			},
			TotalCount: 10,
			HasMore:    true,
		},
	}

	service := todo.NewService(mockRepo, todo.Config{})
	server := NewServer(service)

	// Create request with filter, order_by, and page_size
	filterStr := "title:\"project\""
	orderByStr := "create_time desc"
	pageSize := 1

	req := httptest.NewRequest(http.MethodGet, "/v1/lists", nil)
	w := httptest.NewRecorder()

	params := openapi.ListListsParams{}
	params.Filter = &filterStr
	params.OrderBy = &orderByStr
	params.PageSize = &pageSize

	server.ListLists(w, req, params)

	// Verify response
	require.Equal(t, http.StatusOK, w.Code)

	var resp openapi.ListListsResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	// Verify pagination params were passed correctly
	assert.Equal(t, 1, mockRepo.findListsParams.Limit)
	assert.Equal(t, 0, mockRepo.findListsParams.Offset)

	// Verify next page token is present since HasMore=true
	assert.NotNil(t, resp.NextPageToken)
}

func TestListLists_InvalidFilter(t *testing.T) {
	mockRepo := &mockListRepository{}
	service := todo.NewService(mockRepo, todo.Config{})
	server := NewServer(service)

	// Create request with invalid filter (invalid time format)
	filterStr := "create_time > 'invalid-date'"
	req := httptest.NewRequest(http.MethodGet, "/v1/lists", nil)
	w := httptest.NewRecorder()

	params := openapi.ListListsParams{}
	params.Filter = &filterStr

	server.ListLists(w, req, params)

	// Verify error response
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListLists_InvalidOrderBy(t *testing.T) {
	mockRepo := &mockListRepository{}
	service := todo.NewService(mockRepo, todo.Config{})
	server := NewServer(service)

	// Create request with invalid order direction
	orderByStr := "create_time invalid"
	req := httptest.NewRequest(http.MethodGet, "/v1/lists", nil)
	w := httptest.NewRecorder()

	params := openapi.ListListsParams{}
	params.OrderBy = &orderByStr

	server.ListLists(w, req, params)

	// Verify error response
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListLists_CombinedFilters(t *testing.T) {
	now := time.Now().UTC()
	startTime := now.Add(-48 * time.Hour)
	endTime := now.Add(-24 * time.Hour)

	mockRepo := &mockListRepository{
		findListsResult: &domain.PagedListResult{
			Lists:      []*domain.TodoList{},
			TotalCount: 0,
			HasMore:    false,
		},
	}

	service := todo.NewService(mockRepo, todo.Config{})
	server := NewServer(service)

	// Create request with multiple filters combined with AND
	filterStr := "title:\"project\" AND create_time > '" + startTime.Format(time.RFC3339) +
		"' AND create_time < '" + endTime.Format(time.RFC3339) + "'"

	req := httptest.NewRequest(http.MethodGet, "/v1/lists", nil)
	w := httptest.NewRecorder()

	params := openapi.ListListsParams{}
	params.Filter = &filterStr

	server.ListLists(w, req, params)

	// Verify response
	require.Equal(t, http.StatusOK, w.Code)

	// Verify all filters were parsed correctly
	assert.NotNil(t, mockRepo.findListsParams.TitleContains)
	assert.Equal(t, "project", *mockRepo.findListsParams.TitleContains)

	assert.NotNil(t, mockRepo.findListsParams.CreateTimeAfter)
	assert.WithinDuration(t, startTime, *mockRepo.findListsParams.CreateTimeAfter, time.Second)

	assert.NotNil(t, mockRepo.findListsParams.CreateTimeBefore)
	assert.WithinDuration(t, endTime, *mockRepo.findListsParams.CreateTimeBefore, time.Second)
}

func TestListLists_DefaultOrderBy(t *testing.T) {
	mockRepo := &mockListRepository{
		findListsResult: &domain.PagedListResult{
			Lists:      []*domain.TodoList{},
			TotalCount: 0,
			HasMore:    false,
		},
	}

	service := todo.NewService(mockRepo, todo.Config{})
	server := NewServer(service)

	// Create request without order_by (should use defaults)
	req := httptest.NewRequest(http.MethodGet, "/v1/lists", nil)
	w := httptest.NewRecorder()

	params := openapi.ListListsParams{}

	server.ListLists(w, req, params)

	// Verify response
	require.Equal(t, http.StatusOK, w.Code)

	// Verify default order_by was applied
	assert.Equal(t, "create_time", mockRepo.findListsParams.OrderBy)
	assert.Equal(t, "desc", mockRepo.findListsParams.OrderDir)
}
