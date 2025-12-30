package domain

import "time"

// ListTasksParams contains parameters for listing tasks with filtering, sorting, and pagination.
// Uses ItemsFilter value object for validated filtering and sorting.
//
// Common use cases:
//   - "My overdue tasks": DueBefore=now(), Filter with OrderBy="due_time"
//   - "Tasks in list X": ListID=X, default ordering
//   - "High priority TODO items": Filter with Priorities=[high], Statuses=[todo]
//   - "Active work": Filter with Statuses=[todo, in_progress]
//   - Paginated search: Limit=50, Offset=100 for page 3
type ListTasksParams struct {
	// Filter by specific list (nil = search all lists)
	ListID *string

	// Validated filter (statuses, priorities, tags, orderBy, orderDir)
	// Created via NewItemsFilter which validates all fields at construction.
	Filter ItemsFilter

	// Date filters (nil = no filter applied)
	DueBefore *time.Time // Filter tasks due before this time
	DueAfter  *time.Time // Filter tasks due after this time

	// Pagination (both required for correct pagination)
	Limit  int // Maximum number of items to return (page size)
	Offset int // Number of items to skip (for page N: offset = (N-1) * limit)
}

// PagedResult contains items matching the query parameters.
// Result of applying ListTasksParams (filtering, sorting, and pagination).
type PagedResult struct {
	Items      []TodoItem // Items matching the ListTasksParams criteria
	TotalCount int        // Total matching items across all pages
	HasMore    bool       // Whether there are more pages
}

// ListListsParams contains parameters for listing todo lists with filtering, sorting, and pagination.
//
// Common use cases:
//   - "Recent lists": Sorting with OrderBy="create_time", OrderDir="desc"
//   - "Lists created after date": CreateTimeAfter=time
//   - "Lists with title matching": TitleContains="project"
type ListListsParams struct {
	// Optional filters (nil = no filter applied)
	TitleContains    *string    // Filter by title substring (case-insensitive)
	CreateTimeAfter  *time.Time // Filter lists created after this time
	CreateTimeBefore *time.Time // Filter lists created before this time

	// Validated sorting configuration (created via NewListsSorting)
	Sorting ListsSorting

	// Pagination (both required for correct pagination)
	Limit  int // Maximum number of items to return (page size)
	Offset int // Number of items to skip (for page N: offset = (N-1) * limit)
}

// PagedListResult contains lists matching the query parameters.
type PagedListResult struct {
	Lists      []*TodoList // Lists matching the ListListsParams criteria
	TotalCount int         // Total matching lists across all pages
	HasMore    bool        // Whether there are more pages
}
