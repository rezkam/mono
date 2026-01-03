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

// ValidateGenerationWindowDays validates the generation window value.
// Returns ErrInvalidGenerationWindow if days is not in valid range (1-365).
func ValidateGenerationWindowDays(days int) error {
	if days < 1 || days > 365 {
		return ErrInvalidGenerationWindow
	}
	return nil
}

// Filter limits - business rules to prevent abuse.
const (
	MaxStatusFilter   = 6 // All possible statuses
	MaxPriorityFilter = 4 // All possible priorities
	MaxTagsFilter     = 5
)

// Default sorting values - business rules for item listing.
const (
	DefaultOrderBy  = "created_at"
	DefaultOrderDir = "desc"
)

// Valid order by fields.
var validOrderByFields = map[string]bool{
	"due_at":     true,
	"priority":   true,
	"created_at": true,
	"updated_at": true,
}

// ItemsFilter is a validated filter for listing items.
// Value object - all fields validated at construction time.
// All filter fields are slices (OR logic within field, AND logic across fields).
// Defaults are applied for orderBy/orderDir if not provided.
type ItemsFilter struct {
	statuses   []TaskStatus
	priorities []TaskPriority
	tags       []string
	orderBy    string
	orderDir   string
}

// ItemsFilterInput holds raw input for creating an ItemsFilter.
// All fields are raw strings that will be validated during construction.
type ItemsFilterInput struct {
	Statuses   []string
	Priorities []string
	Tags       []string
	OrderBy    *string
	OrderDir   *string
}

// NewItemsFilter creates a validated filter for listing items.
// All validation happens at construction time - returns error if any field is invalid.
// Empty slices mean "no filter" for that field.
// orderBy defaults to "created_at", orderDir defaults to "desc".
func NewItemsFilter(input ItemsFilterInput) (ItemsFilter, error) {
	filter := ItemsFilter{
		orderBy:  DefaultOrderBy,
		orderDir: DefaultOrderDir,
	}

	// Validate statuses
	if len(input.Statuses) > MaxStatusFilter {
		return ItemsFilter{}, ErrTooManyStatuses
	}
	for _, s := range input.Statuses {
		status, err := NewTaskStatus(s)
		if err != nil {
			return ItemsFilter{}, err
		}
		filter.statuses = append(filter.statuses, status)
	}

	// Validate priorities
	if len(input.Priorities) > MaxPriorityFilter {
		return ItemsFilter{}, ErrTooManyPriorities
	}
	for _, p := range input.Priorities {
		priority, err := NewTaskPriority(p)
		if err != nil {
			return ItemsFilter{}, err
		}
		filter.priorities = append(filter.priorities, priority)
	}

	// Validate tags - copy to ensure immutability
	if len(input.Tags) > MaxTagsFilter {
		return ItemsFilter{}, ErrTooManyTags
	}
	if len(input.Tags) > 0 {
		filter.tags = make([]string, len(input.Tags))
		copy(filter.tags, input.Tags)
	}

	// Validate and set orderBy if provided, otherwise keep default
	if input.OrderBy != nil && *input.OrderBy != "" {
		if !validOrderByFields[*input.OrderBy] {
			return ItemsFilter{}, fmt.Errorf("%w: %s (supported: due_at, priority, created_at, updated_at)", ErrInvalidOrderByField, *input.OrderBy)
		}
		filter.orderBy = *input.OrderBy
	}

	// Validate and set orderDir if provided, otherwise keep default
	if input.OrderDir != nil && *input.OrderDir != "" {
		dir := strings.ToLower(*input.OrderDir)
		if dir != "asc" && dir != "desc" {
			return ItemsFilter{}, ErrInvalidSortDirection
		}
		filter.orderDir = dir
	}

	return filter, nil
}

// Statuses returns the validated status filters (empty slice if not filtering).
func (f ItemsFilter) Statuses() []TaskStatus {
	return f.statuses
}

// Priorities returns the validated priority filters (empty slice if not filtering).
func (f ItemsFilter) Priorities() []TaskPriority {
	return f.priorities
}

// Tags returns the tags filter (empty slice if not filtering).
func (f ItemsFilter) Tags() []string {
	return f.tags
}

// OrderBy returns the order by field (defaults to "created_at").
func (f ItemsFilter) OrderBy() string {
	return f.orderBy
}

// OrderDir returns the sort direction (defaults to "desc").
func (f ItemsFilter) OrderDir() string {
	return f.orderDir
}

// HasStatusFilter returns true if a status filter is applied.
func (f ItemsFilter) HasStatusFilter() bool {
	return len(f.statuses) > 0
}

// Lists sorting defaults and valid fields.
const (
	ListsDefaultOrderBy  = "created_at"
	ListsDefaultOrderDir = "desc"
)

// Valid order by fields for lists.
var validListsOrderByFields = map[string]bool{
	"created_at": true,
	"title":      true,
}

// ListsSorting is a validated sorting configuration for listing lists.
// Value object - validates sorting parameters at construction time.
// Actual sorting is performed by the repository layer.
type ListsSorting struct {
	orderBy  string
	orderDir string
}

// ListsSortingInput holds raw input for creating a ListsSorting.
type ListsSortingInput struct {
	OrderBy  *string
	OrderDir *string
}

// NewListsSorting creates a validated sorting configuration for listing lists.
// Returns error if any field is invalid.
// orderBy defaults to "created_at", orderDir defaults to "desc".
func NewListsSorting(input ListsSortingInput) (ListsSorting, error) {
	sorting := ListsSorting{
		orderBy:  ListsDefaultOrderBy,
		orderDir: ListsDefaultOrderDir,
	}

	// Validate and set orderBy if provided, otherwise keep default
	if input.OrderBy != nil && *input.OrderBy != "" {
		if !validListsOrderByFields[*input.OrderBy] {
			return ListsSorting{}, fmt.Errorf("%w: %s (supported: created_at, title)", ErrInvalidListsOrderByField, *input.OrderBy)
		}
		sorting.orderBy = *input.OrderBy
	}

	// Validate and set orderDir if provided, otherwise keep default
	if input.OrderDir != nil && *input.OrderDir != "" {
		dir := strings.ToLower(*input.OrderDir)
		if dir != "asc" && dir != "desc" {
			return ListsSorting{}, ErrInvalidSortDirection
		}
		sorting.orderDir = dir
	}

	return sorting, nil
}

// OrderBy returns the order by field (defaults to "created_at").
func (s ListsSorting) OrderBy() string {
	return s.orderBy
}

// OrderDir returns the sort direction (defaults to "desc").
func (s ListsSorting) OrderDir() string {
	return s.orderDir
}
