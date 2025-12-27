package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/infrastructure/http/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateList(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Prepare request
	reqBody := openapi.CreateListRequest{
		Title: "Shopping List",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/lists", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	// Execute request
	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusCreated, w.Code)

	var resp openapi.CreateListResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotNil(t, resp.List)
	assert.NotNil(t, resp.List.Id)
	assert.Equal(t, "Shopping List", *resp.List.Title)
	assert.Equal(t, 0, *resp.List.TotalItems)
	assert.Equal(t, 0, *resp.List.UndoneItems)
}

func TestCreateList_InvalidTitle(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	tests := []struct {
		name       string
		title      string
		wantStatus int
	}{
		{
			name:       "empty title",
			title:      "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "title too long",
			title:      string(make([]byte, 300)), // > 255 chars
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := openapi.CreateListRequest{
				Title: tt.title,
			}
			body, _ := json.Marshal(reqBody)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/lists", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+ts.APIKey)

			w := httptest.NewRecorder()
			ts.Router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestGetList(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create a list first
	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "Test List")
	require.NoError(t, err)

	// Get the list
	req := httptest.NewRequest(http.MethodGet, "/api/v1/lists/"+list.ID, nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var resp openapi.GetListResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotNil(t, resp.List)
	assert.Equal(t, list.ID, resp.List.Id.String())
	assert.Equal(t, "Test List", *resp.List.Title)
}

func TestGetList_NotFound(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	missingID, err := uuid.NewV7()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lists/"+missingID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp openapi.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotNil(t, resp.Error.Code)
	assert.Equal(t, "NOT_FOUND", *resp.Error.Code)
}

func TestListLists(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create multiple lists
	ctx := context.Background()
	_, err := ts.TodoService.CreateList(ctx, "List 1")
	require.NoError(t, err)
	_, err = ts.TodoService.CreateList(ctx, "List 2")
	require.NoError(t, err)
	_, err = ts.TodoService.CreateList(ctx, "List 3")
	require.NoError(t, err)

	// List all lists
	req := httptest.NewRequest(http.MethodGet, "/api/v1/lists", nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var resp openapi.ListListsResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotNil(t, resp.Lists)
	assert.GreaterOrEqual(t, len(*resp.Lists), 3)
}

func TestListLists_Pagination(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create many lists to test pagination
	ctx := context.Background()
	for i := 0; i < 60; i++ {
		_, err := ts.TodoService.CreateList(ctx, fmt.Sprintf("List %d", i))
		require.NoError(t, err)
	}

	// Request with page size
	req := httptest.NewRequest(http.MethodGet, "/api/v1/lists?page_size=20", nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var resp openapi.ListListsResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotNil(t, resp.Lists)
	assert.Equal(t, 20, len(*resp.Lists))
	assert.NotNil(t, resp.NextPageToken) // Should have more pages
}

func TestListLists_PageTokenPagination(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	titles := []string{"HTTP Pagination A", "HTTP Pagination B", "HTTP Pagination C"}
	for _, title := range titles {
		_, err := ts.TodoService.CreateList(ctx, title)
		require.NoError(t, err)
	}

	// Request first page
	req := httptest.NewRequest(http.MethodGet, "/api/v1/lists?page_size=2", nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp openapi.ListListsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Lists)
	require.Len(t, *resp.Lists, 2)
	require.NotNil(t, resp.NextPageToken)

	page1 := *resp.Lists
	// Lists are returned in reverse chronological order, so expect C then B.
	require.NotNil(t, page1[0].Title)
	require.Equal(t, "HTTP Pagination C", *page1[0].Title)
	require.NotNil(t, page1[1].Title)
	require.Equal(t, "HTTP Pagination B", *page1[1].Title)

	// Fetch second page using returned token
	token := url.QueryEscape(*resp.NextPageToken)
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/lists?page_size=2&page_token="+token, nil)
	req2.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w2 := httptest.NewRecorder()
	ts.Router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	var resp2 openapi.ListListsResponse
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	require.NotNil(t, resp2.Lists)

	page2 := *resp2.Lists
	found := false
	for _, list := range page2 {
		if list.Title != nil && *list.Title == "HTTP Pagination A" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected to find pagination entry on second page")
}

func TestAuth_MissingAPIKey(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lists", nil)
	// No authorization header

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_InvalidAPIKey(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lists", nil)
	req.Header.Set("Authorization", "Bearer sk_invalid_key")

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
