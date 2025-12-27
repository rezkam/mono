package domain

import (
	"fmt"
	"time"
)

// TodoList is an aggregate root representing a collection of tasks.
//
// Items may be populated or empty depending on the query:
//   - Detail view (GetList): Items populated with full details, counts are 0
//   - List view (ListLists/FindLists): Items empty, counts populated
type TodoList struct {
	ID         string
	Title      string
	Items      []TodoItem // Populated only by GetList. Empty in ListLists/FindLists.
	CreateTime time.Time

	// Count fields populated only in list views (ListLists/FindLists) for performance.
	// These are 0 in detail views (GetList) which don't compute counts.
	TotalItems  int // Total number of items in the list
	UndoneItems int // Number of active items (TODO, IN_PROGRESS, BLOCKED)
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
			l.Items[i].UpdatedAt = time.Now().UTC()
			return true
		}
	}
	return false
}

// TodoItem is an entity within the TodoList aggregate.
// It represents a single task with rich metadata.
type TodoItem struct {
	ID    string
	Title string

	// List relationship
	ListID string // Foreign key to TodoList

	// Status and priority
	Status   TaskStatus
	Priority *TaskPriority // Optional

	// Time tracking
	EstimatedDuration *time.Duration
	ActualDuration    *time.Duration

	// Timestamps
	CreateTime time.Time
	UpdatedAt  time.Time
	DueTime    *time.Time // Optional

	// Tags stored as array
	Tags []string

	// Recurring task link (if this is an instance of a recurring task)
	RecurringTemplateID *string    // Optional link to RecurringTemplate
	InstanceDate        *time.Time // Optional date this instance represents

	// Timezone for due_time interpretation
	// nil = floating time (9am stays 9am in user's current timezone)
	// non-nil = fixed timezone (absolute moment in IANA timezone like 'Europe/Stockholm')
	Timezone *string

	// Optimistic locking version for concurrent update protection
	Version int
}

// Etag returns the entity tag for this item.
// The etag is based on the version number and is used for optimistic concurrency control.
// Returns the version as a string: "1", "2", "42", etc.
func (item *TodoItem) Etag() string {
	return fmt.Sprintf("%d", item.Version)
}

// UpdateItemParams contains parameters for updating a todo item with field mask support.
// Uses client-side optimistic concurrency control via etag (AIP-154).
type UpdateItemParams struct {
	ItemID string
	ListID string

	// Etag for optimistic concurrency control.
	// Format: quoted string, e.g., `"1"`, `"2"`.
	// If doesn't match current version, returns ErrVersionConflict.
	Etag *string

	// UpdateMask specifies which fields to update.
	// Only fields in this list will be modified.
	UpdateMask []string

	// Field values (only applied if field is in UpdateMask)
	Title             *string
	Status            *TaskStatus
	Priority          *TaskPriority
	DueTime           *time.Time
	Tags              *[]string
	Timezone          *string
	EstimatedDuration *time.Duration
	ActualDuration    *time.Duration
}

// UpdateListParams contains parameters for updating a todo list with field mask support.
type UpdateListParams struct {
	ListID string

	// UpdateMask specifies which fields to update.
	// Only fields in this list will be modified.
	UpdateMask []string

	// Field values (only applied if field is in UpdateMask)
	Title *string
}

// UpdateRecurringTemplateParams contains parameters for updating a recurring template with field mask support.
type UpdateRecurringTemplateParams struct {
	TemplateID string
	ListID     string // Required for ownership validation

	// UpdateMask specifies which fields to update.
	// Only fields in this list will be modified.
	UpdateMask []string

	// Field values (only applied if field is in UpdateMask)
	Title                *string
	Tags                 *[]string
	Priority             *TaskPriority
	EstimatedDuration    *time.Duration
	RecurrencePattern    *RecurrencePattern
	RecurrenceConfig     map[string]interface{}
	DueOffset            *time.Duration
	IsActive             *bool
	GenerationWindowDays *int
}

// RecurringTemplate is an aggregate root representing a template for generating recurring task instances.
//
// Recurring tasks are implemented via a template pattern:
//  1. User creates a RecurringTemplate with pattern (DAILY, WEEKLY, etc.)
//  2. Background worker generates TodoItem instances from the template
//  3. Each generated TodoItem links back to its template via RecurringTemplateID
//
// This design allows:
//   - Modifying template without affecting existing items
//   - Viewing history of generated items
//   - Tracking which items came from which template
type RecurringTemplate struct {
	ID     string
	ListID string

	// Template fields (same as TodoItem)
	Title             string
	Tags              []string
	Priority          *TaskPriority
	EstimatedDuration *time.Duration

	// Recurrence configuration
	RecurrencePattern RecurrencePattern
	RecurrenceConfig  map[string]interface{} // Pattern-specific config as JSON
	DueOffset         *time.Duration         // Optional offset for due time

	// Template state
	IsActive             bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
	LastGeneratedUntil   time.Time // Last date we generated tasks up to
	GenerationWindowDays int       // How many days ahead to generate
}

// GenerationJob is an entity representing a background job for generating recurring task instances.
//
// The worker uses these jobs to:
//  1. Schedule when to generate tasks
//  2. Track job status (pending, running, completed, failed)
//  3. Record errors and retry attempts
type GenerationJob struct {
	ID           string
	TemplateID   string
	ScheduledFor time.Time
	Status       string // pending, running, completed, failed

	GenerateFrom  time.Time
	GenerateUntil time.Time
	CreatedAt     time.Time
	StartedAt     *time.Time
	CompletedAt   *time.Time
	FailedAt      *time.Time

	ErrorMessage *string
	RetryCount   int
}

// APIKey is an aggregate root representing an API key for authentication.
//
// API keys use a split-token pattern:
//   - ShortToken: indexed portion for lookup
//   - LongSecretHash: cryptographic hash for verification
//   - FullKey: only shown once at creation (short + long)
//
// This design provides:
//   - Secure verification via constant-time hash comparison
//   - No storage of plaintext secrets
type APIKey struct {
	ID             string
	KeyType        string // "sk" = secret key, "pk" = public key
	Service        string // Service name (e.g., "mono")
	Version        string // API version (e.g., "v1")
	ShortToken     string // Indexed portion for fast lookup
	LongSecretHash string // BLAKE2b-256 hash of long secret
	Name           string // Human-readable name/description
	IsActive       bool
	CreatedAt      time.Time
	LastUsedAt     *time.Time
	ExpiresAt      *time.Time
}
