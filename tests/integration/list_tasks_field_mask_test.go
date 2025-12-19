package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// TestFieldMask_MultipleFields verifies that multiple fields can be updated at once with a field mask.
func TestFieldMask_MultipleFields(t *testing.T) {
	env := newListTasksTestEnv(t)

	createListResp, err := env.Service().CreateList(env.Context(), &monov1.CreateListRequest{
		Title: "Multi-Field Update Test",
	})
	require.NoError(t, err)
	listID := createListResp.List.Id

	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	priorityLow := domain.TaskPriorityLow
	timezone := "America/New_York"
	dueTime := time.Now().Add(48 * time.Hour).UTC()

	item := &domain.TodoItem{
		ID:         itemID,
		Title:      "Original Title",
		Status:     domain.TaskStatusTodo,
		Priority:   &priorityLow,
		Tags:       []string{"old-tag"},
		DueTime:    &dueTime,
		Timezone:   &timezone,
		CreateTime: time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	require.NoError(t, env.Store().CreateItem(env.Context(), listID, item))

	newDueTime := time.Now().Add(72 * time.Hour).UTC()
	resp, err := env.Service().UpdateItem(env.Context(), &monov1.UpdateItemRequest{
		ListId: listID,
		Item: &monov1.TodoItem{
			Id:       itemID,
			Title:    "Updated Title",
			Status:   monov1.TaskStatus_TASK_STATUS_IN_PROGRESS,
			Priority: monov1.TaskPriority_TASK_PRIORITY_HIGH,
			Tags:     []string{"new-tag-1", "new-tag-2"},
			DueTime:  timestampProto(newDueTime),
		},
		UpdateMask: &fieldmaskpb.FieldMask{
			Paths: []string{"title", "status", "priority", "tags", "due_time"},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "Updated Title", resp.Item.Title)
	assert.Equal(t, monov1.TaskStatus_TASK_STATUS_IN_PROGRESS, resp.Item.Status)
	assert.Equal(t, monov1.TaskPriority_TASK_PRIORITY_HIGH, resp.Item.Priority)
	assert.ElementsMatch(t, []string{"new-tag-1", "new-tag-2"}, resp.Item.Tags)
	assert.Equal(t, newDueTime.Unix(), resp.Item.DueTime.AsTime().Unix())

	fetchedItem, err := env.Store().FindItemByID(env.Context(), itemID)
	require.NoError(t, err)
	assert.NotNil(t, fetchedItem.Timezone)
	assert.Equal(t, "America/New_York", *fetchedItem.Timezone,
		"Timezone should be preserved (not in field mask)")
}
