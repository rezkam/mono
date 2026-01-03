package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOrderBy_WithNullValues verifies that items with null priority or due_at are handled correctly in ordering.
func TestOrderBy_WithNullValues(t *testing.T) {
	env := newListTasksTestEnv(t)

	list, err := env.Service().CreateList(env.Context(), "Null Values Test List")
	require.NoError(t, err)
	listID := list.ID

	now := time.Now().UTC()
	items := []struct {
		title    string
		priority *domain.TaskPriority
		dueTime  *time.Time
	}{
		{"Item with HIGH priority and due_at", ptrTaskPriority(domain.TaskPriorityHigh), ptrTime(now.Add(1 * time.Hour))},
		{"Item with LOW priority, no due_at", ptrTaskPriority(domain.TaskPriorityLow), nil},
		{"Item with no priority, has due_at", nil, ptrTime(now.Add(2 * time.Hour))},
		{"Item with MEDIUM priority and due_at", ptrTaskPriority(domain.TaskPriorityMedium), ptrTime(now.Add(3 * time.Hour))},
		{"Item with no priority, no due_at", nil, nil},
	}

	for _, data := range items {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)

		item := &domain.TodoItem{
			ID:        itemUUID.String(),
			Title:     data.title,
			Status:    domain.TaskStatusTodo,
			Priority:  data.priority,
			DueAt:     data.dueTime,
			CreatedAt: now,
			UpdatedAt: now,
		}
		_, err = env.Store().CreateItem(env.Context(), listID, item)
		require.NoError(t, err)
	}

	t.Run("order_by_priority_asc_nulls_last", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			OrderBy:  ptrString("priority"),
			OrderDir: ptrString("asc"),
		})
		require.NoError(t, err)

		params := domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		}
		result, err := env.Service().ListItems(env.Context(), params)
		require.NoError(t, err)
		assert.Equal(t, 5, len(result.Items))

		priorityCount := 0
		nullCount := 0
		for _, item := range result.Items {
			if item.Priority != nil {
				priorityCount++
			} else {
				nullCount++
			}
		}
		assert.Equal(t, 3, priorityCount, "Should have 3 items with priority")
		assert.Equal(t, 2, nullCount, "Should have 2 items without priority")
	})

	t.Run("order_by_due_time_asc_nulls_last", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			OrderBy:  ptrString("due_at"),
			OrderDir: ptrString("asc"),
		})
		require.NoError(t, err)

		params := domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		}
		result, err := env.Service().ListItems(env.Context(), params)
		require.NoError(t, err)
		assert.Equal(t, 5, len(result.Items))

		var withDueAt []string
		var withoutDueAt []string

		for _, item := range result.Items {
			if item.DueAt != nil {
				withDueAt = append(withDueAt, item.Title)
			} else {
				withoutDueAt = append(withoutDueAt, item.Title)
			}
		}

		assert.Equal(t, 3, len(withDueAt), "Should have 3 items with due_at")
		assert.Equal(t, 2, len(withoutDueAt), "Should have 2 items without due_at")
	})

	t.Run("order_by_priority_desc_nulls_last", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			OrderBy:  ptrString("priority"),
			OrderDir: ptrString("desc"),
		})
		require.NoError(t, err)

		params := domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		}
		result, err := env.Service().ListItems(env.Context(), params)
		require.NoError(t, err)
		assert.Equal(t, 5, len(result.Items))

		foundNull := false
		for _, item := range result.Items {
			if item.Priority == nil {
				foundNull = true
			} else if foundNull {
				t.Error("Found non-null priority after null priority - ordering is incorrect")
			}
		}
	})
}
