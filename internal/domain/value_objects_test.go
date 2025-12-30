package domain

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewItemsFilter_EmptyInput(t *testing.T) {
	filter, err := NewItemsFilter(ItemsFilterInput{})

	require.NoError(t, err)
	assert.Empty(t, filter.Statuses())
	assert.Empty(t, filter.Priorities())
	assert.Empty(t, filter.Tags())
	assert.Equal(t, DefaultOrderBy, filter.OrderBy())
	assert.Equal(t, DefaultOrderDir, filter.OrderDir())
	assert.False(t, filter.HasStatusFilter())
}

func TestNewItemsFilter_ValidSingleStatus(t *testing.T) {
	filter, err := NewItemsFilter(ItemsFilterInput{
		Statuses: []string{"todo"},
	})

	require.NoError(t, err)
	require.Len(t, filter.Statuses(), 1)
	assert.Equal(t, TaskStatusTodo, filter.Statuses()[0])
	assert.True(t, filter.HasStatusFilter())
}

func TestNewItemsFilter_ValidMultipleStatuses(t *testing.T) {
	filter, err := NewItemsFilter(ItemsFilterInput{
		Statuses: []string{"todo", "in_progress", "blocked"},
	})

	require.NoError(t, err)
	require.Len(t, filter.Statuses(), 3)
	assert.Equal(t, TaskStatusTodo, filter.Statuses()[0])
	assert.Equal(t, TaskStatusInProgress, filter.Statuses()[1])
	assert.Equal(t, TaskStatusBlocked, filter.Statuses()[2])
	assert.True(t, filter.HasStatusFilter())
}

func TestNewItemsFilter_AllValidStatuses(t *testing.T) {
	filter, err := NewItemsFilter(ItemsFilterInput{
		Statuses: []string{"todo", "in_progress", "blocked", "done", "archived", "cancelled"},
	})

	require.NoError(t, err)
	assert.Len(t, filter.Statuses(), 6)
}

func TestNewItemsFilter_InvalidStatus(t *testing.T) {
	_, err := NewItemsFilter(ItemsFilterInput{
		Statuses: []string{"invalid_status"},
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidTaskStatus))
}

func TestNewItemsFilter_MixedValidInvalidStatuses(t *testing.T) {
	// First valid, second invalid - should fail
	_, err := NewItemsFilter(ItemsFilterInput{
		Statuses: []string{"todo", "invalid"},
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidTaskStatus))
}

func TestNewItemsFilter_TooManyStatuses(t *testing.T) {
	_, err := NewItemsFilter(ItemsFilterInput{
		Statuses: []string{"todo", "in_progress", "blocked", "done", "archived", "cancelled", "extra"},
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTooManyStatuses))
}

func TestNewItemsFilter_StatusCaseInsensitive(t *testing.T) {
	testCases := []struct {
		input    string
		expected TaskStatus
	}{
		{"TODO", TaskStatusTodo},
		{"Todo", TaskStatusTodo},
		{"IN_PROGRESS", TaskStatusInProgress},
		{"In_Progress", TaskStatusInProgress},
		{"DONE", TaskStatusDone},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			filter, err := NewItemsFilter(ItemsFilterInput{
				Statuses: []string{tc.input},
			})

			require.NoError(t, err)
			require.Len(t, filter.Statuses(), 1)
			assert.Equal(t, tc.expected, filter.Statuses()[0])
		})
	}
}

func TestNewItemsFilter_ValidSinglePriority(t *testing.T) {
	filter, err := NewItemsFilter(ItemsFilterInput{
		Priorities: []string{"high"},
	})

	require.NoError(t, err)
	require.Len(t, filter.Priorities(), 1)
	assert.Equal(t, TaskPriorityHigh, filter.Priorities()[0])
}

func TestNewItemsFilter_ValidMultiplePriorities(t *testing.T) {
	filter, err := NewItemsFilter(ItemsFilterInput{
		Priorities: []string{"high", "urgent"},
	})

	require.NoError(t, err)
	require.Len(t, filter.Priorities(), 2)
	assert.Equal(t, TaskPriorityHigh, filter.Priorities()[0])
	assert.Equal(t, TaskPriorityUrgent, filter.Priorities()[1])
}

func TestNewItemsFilter_AllValidPriorities(t *testing.T) {
	filter, err := NewItemsFilter(ItemsFilterInput{
		Priorities: []string{"low", "medium", "high", "urgent"},
	})

	require.NoError(t, err)
	assert.Len(t, filter.Priorities(), 4)
}

func TestNewItemsFilter_InvalidPriority(t *testing.T) {
	_, err := NewItemsFilter(ItemsFilterInput{
		Priorities: []string{"critical"},
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidTaskPriority))
}

func TestNewItemsFilter_TooManyPriorities(t *testing.T) {
	_, err := NewItemsFilter(ItemsFilterInput{
		Priorities: []string{"low", "medium", "high", "urgent", "extra"},
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTooManyPriorities))
}

func TestNewItemsFilter_PriorityCaseInsensitive(t *testing.T) {
	testCases := []struct {
		input    string
		expected TaskPriority
	}{
		{"HIGH", TaskPriorityHigh},
		{"High", TaskPriorityHigh},
		{"URGENT", TaskPriorityUrgent},
		{"Urgent", TaskPriorityUrgent},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			filter, err := NewItemsFilter(ItemsFilterInput{
				Priorities: []string{tc.input},
			})

			require.NoError(t, err)
			require.Len(t, filter.Priorities(), 1)
			assert.Equal(t, tc.expected, filter.Priorities()[0])
		})
	}
}

func TestNewItemsFilter_ValidTags(t *testing.T) {
	filter, err := NewItemsFilter(ItemsFilterInput{
		Tags: []string{"work", "urgent", "project-x"},
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"work", "urgent", "project-x"}, filter.Tags())
}

func TestNewItemsFilter_MaxTags(t *testing.T) {
	filter, err := NewItemsFilter(ItemsFilterInput{
		Tags: []string{"tag1", "tag2", "tag3", "tag4", "tag5"},
	})

	require.NoError(t, err)
	assert.Len(t, filter.Tags(), 5)
}

func TestNewItemsFilter_TooManyTags(t *testing.T) {
	_, err := NewItemsFilter(ItemsFilterInput{
		Tags: []string{"tag1", "tag2", "tag3", "tag4", "tag5", "tag6"},
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTooManyTags))
}

func TestNewItemsFilter_EmptyTagsSlice(t *testing.T) {
	filter, err := NewItemsFilter(ItemsFilterInput{
		Tags: []string{},
	})

	require.NoError(t, err)
	assert.Empty(t, filter.Tags())
}

func TestNewItemsFilter_ValidOrderBy(t *testing.T) {
	testCases := []string{"due_time", "priority", "created_at", "updated_at"}

	for _, orderBy := range testCases {
		t.Run(orderBy, func(t *testing.T) {
			ob := orderBy
			filter, err := NewItemsFilter(ItemsFilterInput{
				OrderBy: &ob,
			})

			require.NoError(t, err)
			assert.Equal(t, orderBy, filter.OrderBy())
		})
	}
}

func TestNewItemsFilter_InvalidOrderBy(t *testing.T) {
	invalidField := "invalid_field"
	_, err := NewItemsFilter(ItemsFilterInput{
		OrderBy: &invalidField,
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidOrderByField))
	assert.Contains(t, err.Error(), "invalid_field")
	assert.Contains(t, err.Error(), "supported:")
}

func TestNewItemsFilter_EmptyOrderByUsesDefault(t *testing.T) {
	empty := ""
	filter, err := NewItemsFilter(ItemsFilterInput{
		OrderBy: &empty,
	})

	require.NoError(t, err)
	assert.Equal(t, DefaultOrderBy, filter.OrderBy())
}

func TestNewItemsFilter_NilOrderByUsesDefault(t *testing.T) {
	filter, err := NewItemsFilter(ItemsFilterInput{
		OrderBy: nil,
	})

	require.NoError(t, err)
	assert.Equal(t, DefaultOrderBy, filter.OrderBy())
}

func TestNewItemsFilter_ValidOrderDir(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"asc", "asc"},
		{"desc", "desc"},
		{"ASC", "asc"},
		{"DESC", "desc"},
		{"Asc", "asc"},
		{"Desc", "desc"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			dir := tc.input
			filter, err := NewItemsFilter(ItemsFilterInput{
				OrderDir: &dir,
			})

			require.NoError(t, err)
			assert.Equal(t, tc.expected, filter.OrderDir())
		})
	}
}

func TestNewItemsFilter_InvalidOrderDir(t *testing.T) {
	invalidDir := "ascending"
	_, err := NewItemsFilter(ItemsFilterInput{
		OrderDir: &invalidDir,
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidSortDirection))
}

func TestNewItemsFilter_EmptyOrderDirUsesDefault(t *testing.T) {
	empty := ""
	filter, err := NewItemsFilter(ItemsFilterInput{
		OrderDir: &empty,
	})

	require.NoError(t, err)
	assert.Equal(t, DefaultOrderDir, filter.OrderDir())
}

func TestNewItemsFilter_NilOrderDirUsesDefault(t *testing.T) {
	filter, err := NewItemsFilter(ItemsFilterInput{
		OrderDir: nil,
	})

	require.NoError(t, err)
	assert.Equal(t, DefaultOrderDir, filter.OrderDir())
}

func TestNewItemsFilter_CombinedFilters(t *testing.T) {
	orderBy := "priority"
	orderDir := "asc"

	filter, err := NewItemsFilter(ItemsFilterInput{
		Statuses:   []string{"todo", "in_progress"},
		Priorities: []string{"high", "urgent"},
		Tags:       []string{"work", "important"},
		OrderBy:    &orderBy,
		OrderDir:   &orderDir,
	})

	require.NoError(t, err)
	assert.Len(t, filter.Statuses(), 2)
	assert.Len(t, filter.Priorities(), 2)
	assert.Len(t, filter.Tags(), 2)
	assert.Equal(t, "priority", filter.OrderBy())
	assert.Equal(t, "asc", filter.OrderDir())
	assert.True(t, filter.HasStatusFilter())
}

func TestNewItemsFilter_ValidationOrder(t *testing.T) {
	// Verify that status validation happens before priority validation
	// by ensuring we get status error even when priority is also invalid
	_, err := NewItemsFilter(ItemsFilterInput{
		Statuses:   []string{"invalid_status"},
		Priorities: []string{"invalid_priority"},
	})

	require.Error(t, err)
	// Should fail on status first
	assert.True(t, errors.Is(err, ErrInvalidTaskStatus))
}

func TestNewItemsFilter_LimitValidationBeforeValueValidation(t *testing.T) {
	// Too many statuses should fail before checking individual values
	manyStatuses := make([]string, MaxStatusFilter+1)
	for i := range manyStatuses {
		manyStatuses[i] = "todo" // All valid values
	}

	_, err := NewItemsFilter(ItemsFilterInput{
		Statuses: manyStatuses,
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTooManyStatuses))
}

func TestNewItemsFilter_Immutability(t *testing.T) {
	input := ItemsFilterInput{
		Statuses: []string{"todo"},
		Tags:     []string{"work"},
	}

	filter, err := NewItemsFilter(input)
	require.NoError(t, err)

	// Modify original input
	input.Statuses[0] = "done"
	input.Tags[0] = "personal"

	// Filter should be unchanged
	assert.Equal(t, TaskStatusTodo, filter.Statuses()[0])
	assert.Equal(t, "work", filter.Tags()[0])
}

func TestNewItemsFilter_DefaultValues(t *testing.T) {
	assert.Equal(t, "created_at", DefaultOrderBy)
	assert.Equal(t, "desc", DefaultOrderDir)
}

func TestNewItemsFilter_FilterLimitsConstants(t *testing.T) {
	assert.Equal(t, 6, MaxStatusFilter)
	assert.Equal(t, 4, MaxPriorityFilter)
	assert.Equal(t, 5, MaxTagsFilter)
}

// TestNewTitle tests the Title value object
func TestNewTitle_Valid(t *testing.T) {
	title, err := NewTitle("My Task")
	require.NoError(t, err)
	assert.Equal(t, "My Task", title.String())
}

func TestNewTitle_TrimsWhitespace(t *testing.T) {
	title, err := NewTitle("  My Task  ")
	require.NoError(t, err)
	assert.Equal(t, "My Task", title.String())
}

func TestNewTitle_Empty(t *testing.T) {
	_, err := NewTitle("")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTitleRequired))
}

func TestNewTitle_OnlyWhitespace(t *testing.T) {
	_, err := NewTitle("   ")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTitleRequired))
}

func TestNewTitle_TooLong(t *testing.T) {
	longTitle := make([]byte, 256)
	for i := range longTitle {
		longTitle[i] = 'a'
	}

	_, err := NewTitle(string(longTitle))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTitleTooLong))
}

func TestNewTitle_MaxLength(t *testing.T) {
	maxTitle := make([]byte, 255)
	for i := range maxTitle {
		maxTitle[i] = 'a'
	}

	title, err := NewTitle(string(maxTitle))
	require.NoError(t, err)
	assert.Len(t, title.String(), 255)
}

// TestNewTaskStatus tests the TaskStatus value object
func TestNewTaskStatus_AllValid(t *testing.T) {
	testCases := []struct {
		input    string
		expected TaskStatus
	}{
		{"todo", TaskStatusTodo},
		{"in_progress", TaskStatusInProgress},
		{"blocked", TaskStatusBlocked},
		{"done", TaskStatusDone},
		{"archived", TaskStatusArchived},
		{"cancelled", TaskStatusCancelled},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			status, err := NewTaskStatus(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, status)
		})
	}
}

func TestNewTaskStatus_Invalid(t *testing.T) {
	_, err := NewTaskStatus("invalid")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidTaskStatus))
}

// TestNewTaskPriority tests the TaskPriority value object
func TestNewTaskPriority_AllValid(t *testing.T) {
	testCases := []struct {
		input    string
		expected TaskPriority
	}{
		{"low", TaskPriorityLow},
		{"medium", TaskPriorityMedium},
		{"high", TaskPriorityHigh},
		{"urgent", TaskPriorityUrgent},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			priority, err := NewTaskPriority(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, priority)
		})
	}
}

func TestNewTaskPriority_Empty_DefaultsToMedium(t *testing.T) {
	priority, err := NewTaskPriority("")
	require.NoError(t, err)
	assert.Equal(t, TaskPriorityMedium, priority)
}

func TestNewTaskPriority_Invalid(t *testing.T) {
	_, err := NewTaskPriority("critical")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidTaskPriority))
}

// TestNewRecurrencePattern tests the RecurrencePattern value object
func TestNewRecurrencePattern_AllValid(t *testing.T) {
	testCases := []struct {
		input    string
		expected RecurrencePattern
	}{
		{"daily", RecurrenceDaily},
		{"weekly", RecurrenceWeekly},
		{"biweekly", RecurrenceBiweekly},
		{"monthly", RecurrenceMonthly},
		{"yearly", RecurrenceYearly},
		{"quarterly", RecurrenceQuarterly},
		{"weekdays", RecurrenceWeekdays},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			pattern, err := NewRecurrencePattern(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, pattern)
		})
	}
}

func TestNewRecurrencePattern_Invalid(t *testing.T) {
	_, err := NewRecurrencePattern("hourly")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidRecurrencePattern))
}

// TestValidateGenerationWindowDays tests the generation window validation
func TestValidateGenerationWindowDays_Valid(t *testing.T) {
	testCases := []int{1, 30, 100, 365}

	for _, days := range testCases {
		t.Run("", func(t *testing.T) {
			err := ValidateGenerationWindowDays(days)
			require.NoError(t, err)
		})
	}
}

func TestValidateGenerationWindowDays_Invalid(t *testing.T) {
	testCases := []int{0, -1, 366, 1000}

	for _, days := range testCases {
		t.Run("", func(t *testing.T) {
			err := ValidateGenerationWindowDays(days)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidGenerationWindow))
		})
	}
}

// TestListsSorting tests the ListsSorting value object
func TestNewListsSorting_EmptyInput(t *testing.T) {
	sorting, err := NewListsSorting(ListsSortingInput{})

	require.NoError(t, err)
	assert.Equal(t, ListsDefaultOrderBy, sorting.OrderBy())
	assert.Equal(t, ListsDefaultOrderDir, sorting.OrderDir())
}

func TestNewListsSorting_ValidOrderBy(t *testing.T) {
	testCases := []string{"create_time", "title"}

	for _, orderBy := range testCases {
		t.Run(orderBy, func(t *testing.T) {
			ob := orderBy
			sorting, err := NewListsSorting(ListsSortingInput{
				OrderBy: &ob,
			})

			require.NoError(t, err)
			assert.Equal(t, orderBy, sorting.OrderBy())
		})
	}
}

func TestNewListsSorting_InvalidOrderBy(t *testing.T) {
	invalidField := "invalid_field"
	_, err := NewListsSorting(ListsSortingInput{
		OrderBy: &invalidField,
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidListsOrderByField))
	assert.Contains(t, err.Error(), "invalid_field")
	assert.Contains(t, err.Error(), "supported:")
}

func TestNewListsSorting_ItemsOrderByNotAllowed(t *testing.T) {
	// Order by fields valid for items but not for lists
	itemsOnlyFields := []string{"due_time", "priority", "created_at", "updated_at"}

	for _, field := range itemsOnlyFields {
		t.Run(field, func(t *testing.T) {
			f := field
			_, err := NewListsSorting(ListsSortingInput{
				OrderBy: &f,
			})

			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidListsOrderByField))
		})
	}
}

func TestNewListsSorting_EmptyOrderByUsesDefault(t *testing.T) {
	empty := ""
	sorting, err := NewListsSorting(ListsSortingInput{
		OrderBy: &empty,
	})

	require.NoError(t, err)
	assert.Equal(t, ListsDefaultOrderBy, sorting.OrderBy())
}

func TestNewListsSorting_NilOrderByUsesDefault(t *testing.T) {
	sorting, err := NewListsSorting(ListsSortingInput{
		OrderBy: nil,
	})

	require.NoError(t, err)
	assert.Equal(t, ListsDefaultOrderBy, sorting.OrderBy())
}

func TestNewListsSorting_ValidOrderDir(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"asc", "asc"},
		{"desc", "desc"},
		{"ASC", "asc"},
		{"DESC", "desc"},
		{"Asc", "asc"},
		{"Desc", "desc"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			dir := tc.input
			sorting, err := NewListsSorting(ListsSortingInput{
				OrderDir: &dir,
			})

			require.NoError(t, err)
			assert.Equal(t, tc.expected, sorting.OrderDir())
		})
	}
}

func TestNewListsSorting_InvalidOrderDir(t *testing.T) {
	invalidDir := "ascending"
	_, err := NewListsSorting(ListsSortingInput{
		OrderDir: &invalidDir,
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidSortDirection))
}

func TestNewListsSorting_EmptyOrderDirUsesDefault(t *testing.T) {
	empty := ""
	sorting, err := NewListsSorting(ListsSortingInput{
		OrderDir: &empty,
	})

	require.NoError(t, err)
	assert.Equal(t, ListsDefaultOrderDir, sorting.OrderDir())
}

func TestNewListsSorting_NilOrderDirUsesDefault(t *testing.T) {
	sorting, err := NewListsSorting(ListsSortingInput{
		OrderDir: nil,
	})

	require.NoError(t, err)
	assert.Equal(t, ListsDefaultOrderDir, sorting.OrderDir())
}

func TestNewListsSorting_CombinedSorting(t *testing.T) {
	orderBy := "title"
	orderDir := "asc"

	sorting, err := NewListsSorting(ListsSortingInput{
		OrderBy:  &orderBy,
		OrderDir: &orderDir,
	})

	require.NoError(t, err)
	assert.Equal(t, "title", sorting.OrderBy())
	assert.Equal(t, "asc", sorting.OrderDir())
}

func TestNewListsSorting_DefaultValues(t *testing.T) {
	assert.Equal(t, "create_time", ListsDefaultOrderBy)
	assert.Equal(t, "desc", ListsDefaultOrderDir)
}
