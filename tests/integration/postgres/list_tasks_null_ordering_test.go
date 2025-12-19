package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"
	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOrderBy_WithNullValues verifies that items with null priority or due_time are handled correctly in ordering.
func TestOrderBy_WithNullValues(t *testing.T) {
	env := newListTasksTestEnv(t)

	createListResp, err := env.Service().CreateList(env.Context(), &monov1.CreateListRequest{
		Title: "Null Values Test List",
	})
	require.NoError(t, err)
	listID := createListResp.List.Id

	now := time.Now().UTC()
	items := []struct {
		title    string
		priority *domain.TaskPriority
		dueTime  *time.Time
	}{
		{"Item with HIGH priority and due_time", ptrTaskPriority(domain.TaskPriorityHigh), ptrTime(now.Add(1 * time.Hour))},
		{"Item with LOW priority, no due_time", ptrTaskPriority(domain.TaskPriorityLow), nil},
		{"Item with no priority, has due_time", nil, ptrTime(now.Add(2 * time.Hour))},
		{"Item with MEDIUM priority and due_time", ptrTaskPriority(domain.TaskPriorityMedium), ptrTime(now.Add(3 * time.Hour))},
		{"Item with no priority, no due_time", nil, nil},
	}

	for _, data := range items {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)

		item := &domain.TodoItem{
			ID:         itemUUID.String(),
			Title:      data.title,
			Status:     domain.TaskStatusTodo,
			Priority:   data.priority,
			DueTime:    data.dueTime,
			CreateTime: now,
			UpdatedAt:  now,
		}
		require.NoError(t, env.Store().CreateItem(env.Context(), listID, item))
	}

	t.Run("order_by_priority_asc_nulls_last", func(t *testing.T) {
		resp, err := env.Service().ListTasks(env.Context(), &monov1.ListTasksRequest{
			Parent:  listID,
			OrderBy: "priority asc",
		})
		require.NoError(t, err)
		assert.Equal(t, 5, len(resp.Items))

		priorityCount := 0
		nullCount := 0
		for _, item := range resp.Items {
			if item.Priority != monov1.TaskPriority_TASK_PRIORITY_UNSPECIFIED {
				priorityCount++
			} else {
				nullCount++
			}
		}
		assert.Equal(t, 3, priorityCount, "Should have 3 items with priority")
		assert.Equal(t, 2, nullCount, "Should have 2 items without priority")
	})

	t.Run("order_by_due_time_asc_nulls_last", func(t *testing.T) {
		resp, err := env.Service().ListTasks(env.Context(), &monov1.ListTasksRequest{
			Parent:  listID,
			OrderBy: "due_time asc",
		})
		require.NoError(t, err)
		assert.Equal(t, 5, len(resp.Items))

		var withDueTime []string
		var withoutDueTime []string

		for _, item := range resp.Items {
			if item.DueTime != nil {
				withDueTime = append(withDueTime, item.Title)
			} else {
				withoutDueTime = append(withoutDueTime, item.Title)
			}
		}

		assert.Equal(t, 3, len(withDueTime), "Should have 3 items with due_time")
		assert.Equal(t, 2, len(withoutDueTime), "Should have 2 items without due_time")
	})

	t.Run("order_by_priority_desc_nulls_last", func(t *testing.T) {
		resp, err := env.Service().ListTasks(env.Context(), &monov1.ListTasksRequest{
			Parent:  listID,
			OrderBy: "priority desc",
		})
		require.NoError(t, err)
		assert.Equal(t, 5, len(resp.Items))

		foundNull := false
		for _, item := range resp.Items {
			if item.Priority == monov1.TaskPriority_TASK_PRIORITY_UNSPECIFIED {
				foundNull = true
			} else if foundNull {
				t.Error("Found non-null priority after null priority - ordering is incorrect")
			}
		}
	})
}
