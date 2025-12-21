package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListTasks_FilteringAndOrdering verifies that filtering and ordering work together correctly.
func TestListTasks_FilteringAndOrdering(t *testing.T) {
	env := newListTasksTestEnv(t)

	list, err := env.Service().CreateList(env.Context(), "Combined Filter Test List")
	require.NoError(t, err)
	listID := list.ID

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
		status := domain.TaskStatusTodo
		params := domain.ListTasksParams{
			ListID:   &listID,
			Status:   &status,
			OrderBy:  "priority",
			OrderDir: "desc",
		}
		result, err := env.Service().ListTasks(env.Context(), params)
		require.NoError(t, err)

		assert.Equal(t, 3, len(result.Items))
		for _, item := range result.Items {
			assert.Equal(t, domain.TaskStatusTodo, item.Status)
		}
		assert.Equal(t, domain.TaskPriorityHigh, *result.Items[0].Priority)
		assert.Equal(t, domain.TaskPriorityHigh, *result.Items[1].Priority)
		assert.Equal(t, domain.TaskPriorityLow, *result.Items[2].Priority)
	})

	t.Run("filter_by_tag_order_by_due_time", func(t *testing.T) {
		tag := "work"
		params := domain.ListTasksParams{
			ListID:   &listID,
			Tag:      &tag,
			OrderBy:  "due_time",
			OrderDir: "asc",
		}
		result, err := env.Service().ListTasks(env.Context(), params)
		require.NoError(t, err)

		assert.Equal(t, 4, len(result.Items))
		for i := 0; i < len(result.Items)-1; i++ {
			assert.True(t, result.Items[i].DueTime.Before(*result.Items[i+1].DueTime),
				"Items should be ordered by due_time ascending")
			assert.Contains(t, result.Items[i].Tags, "work")
		}
		assert.Contains(t, result.Items[len(result.Items)-1].Tags, "work")
	})

	t.Run("filter_by_priority_order_by_updated_at", func(t *testing.T) {
		priority := domain.TaskPriorityHigh
		params := domain.ListTasksParams{
			ListID:   &listID,
			Priority: &priority,
			OrderBy:  "updated_at",
			OrderDir: "desc",
		}
		result, err := env.Service().ListTasks(env.Context(), params)
		require.NoError(t, err)

		assert.Equal(t, 3, len(result.Items))
		for _, item := range result.Items {
			assert.Equal(t, domain.TaskPriorityHigh, *item.Priority)
		}
	})
}
