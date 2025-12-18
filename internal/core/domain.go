package core

import (
	"context"
	"time"
)

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	TaskStatusTodo       TaskStatus = "TODO"
	TaskStatusInProgress TaskStatus = "IN_PROGRESS"
	TaskStatusBlocked    TaskStatus = "BLOCKED"
	TaskStatusDone       TaskStatus = "DONE"
	TaskStatusArchived   TaskStatus = "ARCHIVED"
	TaskStatusCancelled  TaskStatus = "CANCELLED"
)

// TaskPriority represents the priority level of a task.
type TaskPriority string

const (
	TaskPriorityLow    TaskPriority = "LOW"
	TaskPriorityMedium TaskPriority = "MEDIUM"
	TaskPriorityHigh   TaskPriority = "HIGH"
	TaskPriorityUrgent TaskPriority = "URGENT"
)

// RecurrencePattern represents the type of recurrence for recurring tasks.
type RecurrencePattern string

const (
	RecurrenceDaily     RecurrencePattern = "DAILY"
	RecurrenceWeekly    RecurrencePattern = "WEEKLY"
	RecurrenceBiweekly  RecurrencePattern = "BIWEEKLY"
	RecurrenceMonthly   RecurrencePattern = "MONTHLY"
	RecurrenceYearly    RecurrencePattern = "YEARLY"
	RecurrenceQuarterly RecurrencePattern = "QUARTERLY"
	RecurrenceWeekdays  RecurrencePattern = "WEEKDAYS"
)

// TodoItem represents a single task within a list.
type TodoItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`

	// Status and priority
	Status   TaskStatus    `json:"status"`
	Priority *TaskPriority `json:"priority,omitempty"` // Optional

	// Time tracking
	EstimatedDuration *time.Duration `json:"estimated_duration,omitempty"`
	ActualDuration    *time.Duration `json:"actual_duration,omitempty"`

	// Timestamps
	CreateTime time.Time  `json:"create_time"`
	UpdatedAt  time.Time  `json:"updated_at"`
	DueTime    *time.Time `json:"due_time,omitempty"`

	// Tags stored as array
	Tags []string `json:"tags,omitempty"`

	// Recurring task link (if this is an instance of a recurring task)
	RecurringTemplateID *string    `json:"recurring_template_id,omitempty"`
	InstanceDate        *time.Time `json:"instance_date,omitempty"`

	// Timezone for due_time interpretation
	// nil/empty = floating time (9am stays 9am in user's current timezone)
	// non-empty = fixed timezone (absolute moment in IANA timezone like 'Europe/Stockholm')
	Timezone *string `json:"timezone,omitempty"`
}

// TodoList represents a collection of tasks.
//
// ACCESS PATTERNS:
//  1. LIST VIEW (dashboard): Use ListLists() with counts, Items will be empty slice
//  2. DETAIL VIEW (single list): Use GetList() which populates Items array
//
// The separation of concerns allows efficient queries:
//   - List view: SELECT with aggregation, no item loading
//   - Detail view: SELECT with JOIN to load full item details
type TodoList struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Items      []TodoItem `json:"items"`
	CreateTime time.Time  `json:"create_time"`

	// Count fields (optional, populated only in list views for performance)
	// These are 0 when not populated (detail views don't compute counts).
	TotalItems  int `json:"total_items,omitempty"`  // Total number of items in the list
	UndoneItems int `json:"undone_items,omitempty"` // Number of active items (TODO, IN_PROGRESS, BLOCKED)
}

// RecurringTaskTemplate represents a template for generating recurring task instances.
type RecurringTaskTemplate struct {
	ID     string `json:"id"`
	ListID string `json:"list_id"`

	// Template fields (same as TodoItem)
	Title             string         `json:"title"`
	Tags              []string       `json:"tags,omitempty"`
	Priority          *TaskPriority  `json:"priority,omitempty"`
	EstimatedDuration *time.Duration `json:"estimated_duration,omitempty"`

	// Recurrence configuration
	RecurrencePattern RecurrencePattern      `json:"recurrence_pattern"`
	RecurrenceConfig  map[string]interface{} `json:"recurrence_config"` // Pattern-specific config as JSON
	DueOffset         *time.Duration         `json:"due_offset,omitempty"`

	// Template state
	IsActive             bool      `json:"is_active"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	LastGeneratedUntil   time.Time `json:"last_generated_until"`
	GenerationWindowDays int       `json:"generation_window_days"`
}

// GenerationJob represents a background job for generating recurring task instances.
type GenerationJob struct {
	ID           string    `json:"id"`
	TemplateID   string    `json:"template_id"`
	ScheduledFor time.Time `json:"scheduled_for"`
	Status       string    `json:"status"` // PENDING, RUNNING, COMPLETED, FAILED

	GenerateFrom  time.Time  `json:"generate_from"`
	GenerateUntil time.Time  `json:"generate_until"`
	CreatedAt     time.Time  `json:"created_at"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	FailedAt      *time.Time `json:"failed_at,omitempty"`

	ErrorMessage *string `json:"error_message,omitempty"`
	RetryCount   int     `json:"retry_count"`
}

// ListTasksParams contains parameters for listing tasks with filtering, sorting, and pagination.
//
// ACCESS PATTERN: Used by ListTasks() to implement efficient database-level operations.
// All filtering, sorting, and pagination happens in PostgreSQL, not in application memory.
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

	// Sorting (empty string uses default: created_at DESC)
	OrderBy string // Supported: "due_time", "priority", "created_at", "updated_at"

	// Pagination (both required for correct pagination)
	Limit  int // Maximum number of items to return (page size)
	Offset int // Number of items to skip (for page N: offset = (N-1) * limit)
}

// ListTasksResult contains the result of listing tasks.
type ListTasksResult struct {
	Items      []TodoItem // The items
	TotalCount int        // Total count (for pagination)
	HasMore    bool       // Whether there are more items
}

// AddItem adds a new item to the list.
func (l *TodoList) AddItem(item TodoItem) {
	l.Items = append(l.Items, item)
}

// UpdateItemStatus updates the status of an item in the list.
// Returns true if the item was found and updated.
func (l *TodoList) UpdateItemStatus(itemID string, status TaskStatus) bool {
	for i, item := range l.Items {
		if item.ID == itemID {
			l.Items[i].Status = status
			l.Items[i].UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

// Storage defines the interface for persisting TodoLists.
type Storage interface {
	// CreateList creates a new TodoList.
	CreateList(ctx context.Context, list *TodoList) error

	// GetList retrieves a TodoList by its ID.
	GetList(ctx context.Context, id string) (*TodoList, error)

	// UpdateList updates an existing TodoList (e.g. adding items, changing status).
	UpdateList(ctx context.Context, list *TodoList) error

	// CreateTodoItem creates a new item in a list, preserving status history.
	CreateTodoItem(ctx context.Context, listID string, item TodoItem) error

	// UpdateTodoItem updates an existing item, preserving status history.
	UpdateTodoItem(ctx context.Context, item TodoItem) error

	// ListLists returns all available TodoLists.
	ListLists(ctx context.Context) ([]*TodoList, error)

	// ListTasks returns tasks with filtering, sorting, and pagination.
	ListTasks(ctx context.Context, params ListTasksParams) (*ListTasksResult, error)

	// Recurring template management
	CreateRecurringTemplate(ctx context.Context, template *RecurringTaskTemplate) error
	GetRecurringTemplate(ctx context.Context, id string) (*RecurringTaskTemplate, error)
	UpdateRecurringTemplate(ctx context.Context, template *RecurringTaskTemplate) error
	UpdateRecurringTemplateGenerationWindow(ctx context.Context, id string, lastGeneratedUntil time.Time) error
	DeleteRecurringTemplate(ctx context.Context, id string) error
	ListRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*RecurringTaskTemplate, error)

	// Helper for background worker
	GetActiveTemplatesNeedingGeneration(ctx context.Context) ([]*RecurringTaskTemplate, error)

	// Generation job management
	// CreateGenerationJob creates a new job to generate recurring tasks.
	// For immediate scheduling, pass time.Time{} (zero value) for scheduledFor.
	// For future scheduling, pass a specific timestamp.
	CreateGenerationJob(ctx context.Context, templateID string, scheduledFor time.Time, generateFrom, generateUntil time.Time) (string, error)

	// ClaimNextGenerationJob atomically claims the next available job for processing.
	// Returns empty string if no jobs are available.
	ClaimNextGenerationJob(ctx context.Context) (string, error)

	GetGenerationJob(ctx context.Context, jobID string) (*GenerationJob, error)
	UpdateGenerationJobStatus(ctx context.Context, jobID string, status string, errorMessage *string) error

	// Close closes the storage connection.
	Close() error
}
