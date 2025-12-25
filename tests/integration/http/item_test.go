package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/http/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateItem_SetsStatusAndPriority(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	list, err := ts.TodoService.CreateList(ctx, "HTTP Update List")
	require.NoError(t, err)

	item, err := ts.TodoService.CreateItem(ctx, list.ID, &domain.TodoItem{
		Title: "Initial Item",
	})
	require.NoError(t, err)

	status := openapi.ItemStatus("done")
	priority := openapi.ItemPriority("high")
	updateMask := []string{"status", "priority"}
	reqBody := openapi.UpdateItemRequest{
		Item: &openapi.TodoItem{
			Status:   &status,
			Priority: &priority,
		},
		UpdateMask: &updateMask,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/lists/%s/items/%s", list.ID, item.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp openapi.UpdateItemResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Item)
	require.NotNil(t, resp.Item.Status)
	assert.Equal(t, status, *resp.Item.Status)
	require.NotNil(t, resp.Item.Priority)
	assert.Equal(t, priority, *resp.Item.Priority)
}

func TestUpdateItem_ListMismatchReturnsNotFound(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	sourceList, err := ts.TodoService.CreateList(ctx, "HTTP Update Source")
	require.NoError(t, err)
	otherList, err := ts.TodoService.CreateList(ctx, "HTTP Update Other")
	require.NoError(t, err)

	item, err := ts.TodoService.CreateItem(ctx, sourceList.ID, &domain.TodoItem{
		Title: "Item On Source List",
	})
	require.NoError(t, err)

	status := openapi.ItemStatus("blocked")
	updateMask := []string{"status"}
	reqBody := openapi.UpdateItemRequest{
		Item: &openapi.TodoItem{
			Status: &status,
		},
		UpdateMask: &updateMask,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/lists/%s/items/%s", otherList.ID, item.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
