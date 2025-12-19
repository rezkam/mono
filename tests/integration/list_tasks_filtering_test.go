package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListTasks_FilteringAndOrdering verifies that filtering and ordering work together correctly.
func TestListTasks_FilteringAndOrdering(t *testing.T) {
	env := newListTasksTestEnv(t)

	createListResp, err := env.Service().CreateList(env.Context(), &monov1.CreateListRequest{
		Title: "Combined Filter Test List",
	})
	require.NoError(t, err)
	listID := createListResp.List.Id

	now := time.Now().UTC()
	items := []struct {
		title    string
		status   domain.TaskStatus
		priority *domain.TaskPriority
		tags     []string
		dueTime  *time.Time
	}{
		{"High Priority Todo 1", domain.TaskStatusTodo, ptrTaskPriority(domain.TaskPriorityHigh), []string{"work"}, ptrTime(now.Add(1 * time.Hour))},
		{"High Priority Todo 2", domain.TaskStatusTodo, ptrTaskPriority(domain.TaskPriorityHigh), []string{"work"}, ptrTime(now.Add(2 * time.Hour))},
		{"Medium Priority InProgress", domain.TaskStatusInProgress, ptrTaskPriority(domain.TaskPriorityMedium), []string{"work", "urgent"}, ptrTime(now.Add(3 * time.Hour))},
		{"Low Priority Todo", domain.TaskStatusTodo, ptrTaskPriority(domain.TaskPriorityLow), []string{"personal"}, ptrTime(now.Add(4 * time.Hour))},
		{"High Priority Done", domain.TaskStatusDone, ptrTaskPriority(domain.TaskPriorityHigh), []string{"work"}, ptrTime(now.Add(5 * time.Hour))},
	}

	for _, data := range items {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)

		item := &domain.TodoItem{
			ID:         itemUUID.String(),
			Title:      data.title,
			Status:     data.status,
			Priority:   data.priority,
			Tags:       data.tags,
			DueTime:    data.dueTime,
			CreateTime: now,
			UpdatedAt:  now,
		}
		require.NoError(t, env.Store().CreateItem(env.Context(), listID, item))
	}

	t.Run("filter_by_status_order_by_priority", func(t *testing.T) {
		resp, err := env.Service().ListTasks(env.Context(), &monov1.ListTasksRequest{
			Parent:  listID,
			Filter:  "status:TODO",
			OrderBy: "priority desc",
		})
		require.NoError(t, err)

		assert.Equal(t, 3, len(resp.Items))
		for _, item := range resp.Items {
			assert.Equal(t, monov1.TaskStatus_TASK_STATUS_TODO, item.Status)
		}
		assert.Equal(t, monov1.TaskPriority_TASK_PRIORITY_HIGH, resp.Items[0].Priority)
		assert.Equal(t, monov1.TaskPriority_TASK_PRIORITY_HIGH, resp.Items[1].Priority)
		assert.Equal(t, monov1.TaskPriority_TASK_PRIORITY_LOW, resp.Items[2].Priority)
	})

	t.Run("filter_by_tag_order_by_due_time", func(t *testing.T) {
		resp, err := env.Service().ListTasks(env.Context(), &monov1.ListTasksRequest{
			Parent:  listID,
			Filter:  "tags:work",
			OrderBy: "due_time asc",
		})
		require.NoError(t, err)

		assert.Equal(t, 4, len(resp.Items))
		for i := 0; i < len(resp.Items)-1; i++ {
			assert.True(t, resp.Items[i].DueTime.AsTime().Before(resp.Items[i+1].DueTime.AsTime()),
				"Items should be ordered by due_time ascending")
			assert.Contains(t, resp.Items[i].Tags, "work")
		}
		assert.Contains(t, resp.Items[len(resp.Items)-1].Tags, "work")
	})

	t.Run("filter_by_priority_order_by_updated_at", func(t *testing.T) {
		resp, err := env.Service().ListTasks(env.Context(), &monov1.ListTasksRequest{
			Parent:  listID,
			Filter:  "priority:HIGH",
			OrderBy: "updated_at desc",
		})
		require.NoError(t, err)

		assert.Equal(t, 3, len(resp.Items))
		for _, item := range resp.Items {
			assert.Equal(t, monov1.TaskPriority_TASK_PRIORITY_HIGH, item.Priority)
		}
	})
}
