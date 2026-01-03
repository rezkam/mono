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
			ID:        itemUUID.String(),
			Title:     data.title,
			Status:    data.status,
			Priority:  data.priority,
			Tags:      data.tags,
			DueAt:     data.dueTime,
			CreatedAt: now,
			UpdatedAt: now,
		}
		_, err = env.Store().CreateItem(env.Context(), listID, item)
		require.NoError(t, err)
	}

	t.Run("filter_by_status_order_by_priority", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Statuses: []string{"todo"},
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

		assert.Equal(t, 3, len(result.Items))
		for _, item := range result.Items {
			assert.Equal(t, domain.TaskStatusTodo, item.Status)
		}
		assert.Equal(t, domain.TaskPriorityHigh, *result.Items[0].Priority)
		assert.Equal(t, domain.TaskPriorityHigh, *result.Items[1].Priority)
		assert.Equal(t, domain.TaskPriorityLow, *result.Items[2].Priority)
	})

	t.Run("filter_by_tag_order_by_due_time", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Tags:     []string{"work"},
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

		assert.Equal(t, 4, len(result.Items))
		for i := 0; i < len(result.Items)-1; i++ {
			assert.True(t, result.Items[i].DueAt.Before(*result.Items[i+1].DueAt),
				"Items should be ordered by due_at ascending")
			assert.Contains(t, result.Items[i].Tags, "work")
		}
		assert.Contains(t, result.Items[len(result.Items)-1].Tags, "work")
	})

	t.Run("filter_by_priority_order_by_updated_at", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Priorities: []string{"high"},
			OrderBy:    ptrString("updated_at"),
			OrderDir:   ptrString("desc"),
		})
		require.NoError(t, err)

		params := domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		}
		result, err := env.Service().ListItems(env.Context(), params)
		require.NoError(t, err)

		assert.Equal(t, 3, len(result.Items))
		for _, item := range result.Items {
			assert.Equal(t, domain.TaskPriorityHigh, *item.Priority)
		}
	})
}

// TestListTasks_MultiValueStatusFilter tests OR logic for multiple status values.
func TestListTasks_MultiValueStatusFilter(t *testing.T) {
	env := newListTasksTestEnv(t)

	list, err := env.Service().CreateList(env.Context(), "Multi-Status Filter Test")
	require.NoError(t, err)
	listID := list.ID

	// Create items with different statuses
	statuses := []domain.TaskStatus{
		domain.TaskStatusTodo,
		domain.TaskStatusInProgress,
		domain.TaskStatusBlocked,
		domain.TaskStatusDone,
		domain.TaskStatusArchived,
		domain.TaskStatusCancelled,
	}

	for i, status := range statuses {
		item := &domain.TodoItem{
			Title:  "Item " + string(rune('A'+i)),
			Status: status,
		}
		_, err := env.Service().CreateItem(env.Context(), listID, item)
		require.NoError(t, err)
	}

	t.Run("single_status_filter", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Statuses: []string{"todo"},
		})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		assert.Len(t, result.Items, 1)
		assert.Equal(t, domain.TaskStatusTodo, result.Items[0].Status)
	})

	t.Run("multiple_statuses_or_logic", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Statuses: []string{"todo", "in_progress", "blocked"},
		})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		assert.Len(t, result.Items, 3)
		foundStatuses := make(map[domain.TaskStatus]bool)
		for _, item := range result.Items {
			foundStatuses[item.Status] = true
		}
		assert.True(t, foundStatuses[domain.TaskStatusTodo])
		assert.True(t, foundStatuses[domain.TaskStatusInProgress])
		assert.True(t, foundStatuses[domain.TaskStatusBlocked])
	})

	t.Run("all_statuses_filter", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Statuses: []string{"todo", "in_progress", "blocked", "done", "archived", "cancelled"},
		})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		assert.Len(t, result.Items, 6)
	})

	t.Run("no_filter_excludes_archived_and_cancelled", func(t *testing.T) {
		// No status filter should apply default exclusions
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		// Should return only non-archived, non-cancelled items (4 total)
		assert.Len(t, result.Items, 4)
		for _, item := range result.Items {
			assert.NotEqual(t, domain.TaskStatusArchived, item.Status)
			assert.NotEqual(t, domain.TaskStatusCancelled, item.Status)
		}
	})

	t.Run("explicit_archived_filter_returns_archived", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Statuses: []string{"archived"},
		})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		assert.Len(t, result.Items, 1)
		assert.Equal(t, domain.TaskStatusArchived, result.Items[0].Status)
	})
}

// TestListTasks_MultiValuePriorityFilter tests OR logic for multiple priority values.
func TestListTasks_MultiValuePriorityFilter(t *testing.T) {
	env := newListTasksTestEnv(t)

	list, err := env.Service().CreateList(env.Context(), "Multi-Priority Filter Test")
	require.NoError(t, err)
	listID := list.ID

	// Create items with different priorities
	priorities := []domain.TaskPriority{
		domain.TaskPriorityLow,
		domain.TaskPriorityMedium,
		domain.TaskPriorityHigh,
		domain.TaskPriorityUrgent,
	}

	for i, priority := range priorities {
		p := priority
		item := &domain.TodoItem{
			Title:    "Item " + string(rune('A'+i)),
			Priority: &p,
		}
		_, err := env.Service().CreateItem(env.Context(), listID, item)
		require.NoError(t, err)
	}

	t.Run("single_priority_filter", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Priorities: []string{"high"},
		})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		assert.Len(t, result.Items, 1)
		assert.Equal(t, domain.TaskPriorityHigh, *result.Items[0].Priority)
	})

	t.Run("multiple_priorities_or_logic", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Priorities: []string{"high", "urgent"},
		})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		assert.Len(t, result.Items, 2)
		foundPriorities := make(map[domain.TaskPriority]bool)
		for _, item := range result.Items {
			foundPriorities[*item.Priority] = true
		}
		assert.True(t, foundPriorities[domain.TaskPriorityHigh])
		assert.True(t, foundPriorities[domain.TaskPriorityUrgent])
	})

	t.Run("all_priorities_filter", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Priorities: []string{"low", "medium", "high", "urgent"},
		})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		assert.Len(t, result.Items, 4)
	})
}

// TestListTasks_MultiValueTagFilter tests AND logic for multiple tag values.
func TestListTasks_MultiValueTagFilter(t *testing.T) {
	env := newListTasksTestEnv(t)

	list, err := env.Service().CreateList(env.Context(), "Multi-Tag Filter Test")
	require.NoError(t, err)
	listID := list.ID

	// Create items with different tag combinations
	itemsData := []struct {
		title string
		tags  []string
	}{
		{"Item A", []string{"work", "urgent"}},
		{"Item B", []string{"work", "personal"}},
		{"Item C", []string{"personal", "urgent"}},
		{"Item D", []string{"work"}},
		{"Item E", []string{"personal"}},
		{"Item F", []string{"work", "urgent", "important"}},
	}

	for _, data := range itemsData {
		item := &domain.TodoItem{
			Title: data.title,
			Tags:  data.tags,
		}
		_, err := env.Service().CreateItem(env.Context(), listID, item)
		require.NoError(t, err)
	}

	t.Run("single_tag_filter", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Tags: []string{"work"},
		})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		assert.Len(t, result.Items, 4) // Items A, B, D, F
		for _, item := range result.Items {
			assert.Contains(t, item.Tags, "work")
		}
	})

	t.Run("multiple_tags_and_logic", func(t *testing.T) {
		// Items must have BOTH "work" AND "urgent"
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Tags: []string{"work", "urgent"},
		})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		assert.Len(t, result.Items, 2) // Items A and F
		for _, item := range result.Items {
			assert.Contains(t, item.Tags, "work")
			assert.Contains(t, item.Tags, "urgent")
		}
	})

	t.Run("three_tags_and_logic", func(t *testing.T) {
		// Items must have "work", "urgent", AND "important"
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Tags: []string{"work", "urgent", "important"},
		})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		assert.Len(t, result.Items, 1) // Only Item F
		assert.Contains(t, result.Items[0].Tags, "work")
		assert.Contains(t, result.Items[0].Tags, "urgent")
		assert.Contains(t, result.Items[0].Tags, "important")
	})

	t.Run("nonexistent_tag_returns_empty", func(t *testing.T) {
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Tags: []string{"nonexistent"},
		})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		assert.Len(t, result.Items, 0)
	})
}

// TestListTasks_CombinedFilters tests combining status, priority, and tag filters.
func TestListTasks_CombinedFilters(t *testing.T) {
	env := newListTasksTestEnv(t)

	list, err := env.Service().CreateList(env.Context(), "Combined Filters Test")
	require.NoError(t, err)
	listID := list.ID

	// Create items with various combinations
	itemsData := []struct {
		title    string
		status   domain.TaskStatus
		priority domain.TaskPriority
		tags     []string
	}{
		{"Todo High Work", domain.TaskStatusTodo, domain.TaskPriorityHigh, []string{"work"}},
		{"Todo Medium Personal", domain.TaskStatusTodo, domain.TaskPriorityMedium, []string{"personal"}},
		{"InProgress High Work", domain.TaskStatusInProgress, domain.TaskPriorityHigh, []string{"work", "urgent"}},
		{"Done High Work", domain.TaskStatusDone, domain.TaskPriorityHigh, []string{"work"}},
		{"Todo Low Work", domain.TaskStatusTodo, domain.TaskPriorityLow, []string{"work"}},
	}

	for _, data := range itemsData {
		p := data.priority
		item := &domain.TodoItem{
			Title:    data.title,
			Status:   data.status,
			Priority: &p,
			Tags:     data.tags,
		}
		_, err := env.Service().CreateItem(env.Context(), listID, item)
		require.NoError(t, err)
	}

	t.Run("status_and_priority_filter", func(t *testing.T) {
		// Todo status AND high priority
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Statuses:   []string{"todo"},
			Priorities: []string{"high"},
		})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		assert.Len(t, result.Items, 1)
		assert.Equal(t, "Todo High Work", result.Items[0].Title)
	})

	t.Run("status_and_tag_filter", func(t *testing.T) {
		// (Todo OR InProgress) AND "work" tag
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Statuses: []string{"todo", "in_progress"},
			Tags:     []string{"work"},
		})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		assert.Len(t, result.Items, 3) // Todo High Work, InProgress High Work, Todo Low Work
		for _, item := range result.Items {
			assert.Contains(t, []domain.TaskStatus{domain.TaskStatusTodo, domain.TaskStatusInProgress}, item.Status)
			assert.Contains(t, item.Tags, "work")
		}
	})

	t.Run("priority_and_tag_filter", func(t *testing.T) {
		// High priority AND "work" tag
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Priorities: []string{"high"},
			Tags:       []string{"work"},
		})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		assert.Len(t, result.Items, 3) // Todo High Work, InProgress High Work, Done High Work
		for _, item := range result.Items {
			assert.Equal(t, domain.TaskPriorityHigh, *item.Priority)
			assert.Contains(t, item.Tags, "work")
		}
	})

	t.Run("all_filters_combined", func(t *testing.T) {
		// (Todo OR InProgress) AND (High OR Medium) AND "work" tag
		filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{
			Statuses:   []string{"todo", "in_progress"},
			Priorities: []string{"high", "medium"},
			Tags:       []string{"work"},
		})
		require.NoError(t, err)

		result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
			ListID: &listID,
			Filter: filter,
		})
		require.NoError(t, err)

		// Todo High Work, InProgress High Work (not Todo Medium Personal - no work tag)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Contains(t, []domain.TaskStatus{domain.TaskStatusTodo, domain.TaskStatusInProgress}, item.Status)
			assert.Contains(t, []domain.TaskPriority{domain.TaskPriorityHigh, domain.TaskPriorityMedium}, *item.Priority)
			assert.Contains(t, item.Tags, "work")
		}
	})
}

// TestListTasks_DefaultSorting verifies default sort order is applied.
func TestListTasks_DefaultSorting(t *testing.T) {
	env := newListTasksTestEnv(t)

	list, err := env.Service().CreateList(env.Context(), "Default Sorting Test")
	require.NoError(t, err)
	listID := list.ID

	// Create items with specific creation order
	for i := range 5 {
		item := &domain.TodoItem{
			Title: "Item " + string(rune('A'+i)),
		}
		_, err := env.Service().CreateItem(env.Context(), listID, item)
		require.NoError(t, err)
		// Database assigns created_at automatically with microsecond precision
		// No artificial delay needed
	}

	// No filter specified - should use default order (created_at desc)
	filter, err := domain.NewItemsFilter(domain.ItemsFilterInput{})
	require.NoError(t, err)

	result, err := env.Service().ListItems(env.Context(), domain.ListTasksParams{
		ListID: &listID,
		Filter: filter,
	})
	require.NoError(t, err)

	assert.Len(t, result.Items, 5)
	// Items should be in reverse creation order (most recent first)
	for i := 0; i < len(result.Items)-1; i++ {
		assert.True(t, result.Items[i].CreatedAt.After(result.Items[i+1].CreatedAt),
			"Items should be ordered by created_at descending (default)")
	}
}

// ptrString helper creates a pointer to a string.
func ptrString(s string) *string {
	return &s
}
