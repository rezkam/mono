package domain

import (
	"fmt"
	"strings"
)

// Title is a validated title value object (1-255 characters).
type Title struct {
	value string
}

// NewTitle creates a new Title, validating the input.
func NewTitle(s string) (Title, error) {
	s = strings.TrimSpace(s)

	if s == "" {
		return Title{}, ErrTitleRequired
	}

	if len(s) > 255 {
		return Title{}, ErrTitleTooLong
	}

	return Title{value: s}, nil
}

// String returns the title value.
func (t Title) String() string {
	return t.value
}

// NewTaskStatus validates and creates a TaskStatus.
func NewTaskStatus(s string) (TaskStatus, error) {
	status := TaskStatus(strings.ToLower(s))

	switch status {
	case TaskStatusTodo, TaskStatusInProgress, TaskStatusBlocked,
		TaskStatusDone, TaskStatusArchived, TaskStatusCancelled:
		return status, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidTaskStatus, s)
	}
}

// NewTaskPriority validates and creates a TaskPriority.
// Returns error for invalid values.
func NewTaskPriority(s string) (TaskPriority, error) {
	if s == "" {
		return TaskPriorityMedium, nil
	}

	priority := TaskPriority(strings.ToLower(s))

	switch priority {
	case TaskPriorityLow, TaskPriorityMedium, TaskPriorityHigh, TaskPriorityUrgent:
		return priority, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidTaskPriority, s)
	}
}

// NewRecurrencePattern validates and creates a RecurrencePattern.
func NewRecurrencePattern(s string) (RecurrencePattern, error) {
	pattern := RecurrencePattern(strings.ToLower(s))

	switch pattern {
	case RecurrenceDaily, RecurrenceWeekly, RecurrenceBiweekly,
		RecurrenceMonthly, RecurrenceYearly, RecurrenceQuarterly,
		RecurrenceWeekdays:
		return pattern, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidRecurrencePattern, s)
	}
}
