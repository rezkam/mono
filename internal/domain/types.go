package domain

import "time"

// ListTasksParams contains parameters for listing tasks with filtering, sorting, and pagination.
//
// Common use cases:
//   - "My overdue tasks": DueBefore=now(), OrderBy="due_time"
//   - "Tasks in list X": ListID=X, default ordering
//   - "High priority TODO items": Priority=HIGH, Status=TODO
//   - Paginated search: Limit=50, Offset=100 for page 3
type ListTasksParams struct {
	// Optional filters (nil = no filter applied)
	ListID    *string       // Filter by specific list (nil = search all lists)
	Status    *TaskStatus   // Filter by status
	Priority  *TaskPriority // Filter by priority
	Tag       *string       // Filter by tag (JSONB array contains)
	DueBefore *time.Time    // Filter tasks due before this time
	DueAfter  *time.Time    // Filter tasks due after this time

	// Sorting (empty uses defaults: created_at field, desc direction)
	OrderBy  string // Supported: "due_time", "priority", "created_at", "updated_at"
	OrderDir string // Sort direction: "asc" or "desc" (empty = field's default)

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
//   - "Recent lists": OrderBy="create_time", OrderDir="desc"
//   - "Lists created after date": CreateTimeAfter=time
//   - "Lists with title matching": TitleContains="project"
type ListListsParams struct {
	// Optional filters (nil = no filter applied)
	TitleContains    *string    // Filter by title substring (case-insensitive)
	CreateTimeAfter  *time.Time // Filter lists created after this time
	CreateTimeBefore *time.Time // Filter lists created before this time

	// Sorting (empty uses defaults: create_time field, desc direction)
	OrderBy  string // Supported: "create_time", "title"
	OrderDir string // Sort direction: "asc" or "desc" (empty = "desc")

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
