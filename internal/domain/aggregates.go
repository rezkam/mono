package domain

import (
	"fmt"
	"time"
)

// TodoList is an aggregate root representing a collection of tasks.
//
// Items are NOT included in this aggregate. They are always fetched separately
// via FindItems (with pagination) to prevent unbounded data loading.
// Use GET /v1/lists/{list_id}/items to fetch items with filtering and pagination.
type TodoList struct {
	ID        string
	Title     string
	CreatedAt time.Time

	// Count fields are always populated from database aggregation.
	TotalItems  int // Total number of items in the list
	UndoneItems int // Number of active items (TODO, IN_PROGRESS, BLOCKED)

	// Optimistic locking version for concurrent update protection
	Version int
}

// Etag returns the entity tag for this list.
// The etag is based on the version number and is used for optimistic concurrency control.
func (list *TodoList) Etag() string {
	return fmt.Sprintf("%d", list.Version)
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
	CreatedAt time.Time
	UpdatedAt time.Time
	DueAt     *time.Time // Optional

	// Tags stored as array
	Tags []string

	// Recurring task link
	RecurringTemplateID *string // Optional link to RecurringTemplate

	// Scheduling fields
	StartsAt  *time.Time     // When task becomes active/visible
	OccursAt  *time.Time     // Exact timestamp for recurring instances (supports intra-day patterns)
	DueOffset *time.Duration // Duration from StartsAt to calculate DueAt

	// Timezone controls how task-related times (StartsAt, OccursAt, DueAt) are interpreted.
	// This field does NOT affect operational times (CreatedAt, UpdatedAt) which are always UTC.
	//
	// Two modes:
	//   nil = Floating time
	//     - Time values stay constant across timezones
	//     - 9am is always 9am regardless of user's location
	//     - Example: "Wake up at 9am" (9am local time wherever you are)
	//
	//   non-nil = Fixed timezone (IANA timezone like 'Europe/Stockholm')
	//     - Time is anchored to a specific timezone
	//     - Represents an absolute UTC moment
	//     - Example: "Stockholm office meeting at 9am" → 08:00 UTC (Stockholm is UTC+1)
	//     - Viewing in Helsinki (UTC+2): displays as 10am
	//     - Viewing in New York (UTC-5): displays as 4am
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

// SetDueFromOffset calculates DueAt from StartsAt + DueOffset if both are set.
func (item *TodoItem) SetDueFromOffset() {
	if item.StartsAt != nil && item.DueOffset != nil {
		due := item.StartsAt.Add(*item.DueOffset)
		item.DueAt = &due
	}
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
	DueAt             *time.Time
	Tags              *[]string
	Timezone          *string
	EstimatedDuration *time.Duration
	ActualDuration    *time.Duration

	// DetachFromTemplate indicates whether to detach this item from its recurring template.
	// Set by the service layer when content/schedule fields are modified on a recurring item.
	// Computed internally - not set by API clients.
	DetachFromTemplate bool
}

// UpdateListParams contains parameters for updating a todo list with field mask support.
// Uses client-side optimistic concurrency control via etag (AIP-154).
type UpdateListParams struct {
	ListID string

	// Etag for optimistic concurrency control.
	// Format: numeric string, e.g., "1", "2".
	// If provided and doesn't match current version, returns ErrVersionConflict.
	Etag *string

	// UpdateMask specifies which fields to update.
	// Only fields in this list will be modified.
	UpdateMask []string

	// Field values (only applied if field is in UpdateMask)
	Title *string
}

// Field names for RecurringTemplate update masks.
// These constants ensure type safety and prevent typos in field mask handling.
const (
	FieldTitle                 = "title"
	FieldTags                  = "tags"
	FieldPriority              = "priority"
	FieldEstimatedDuration     = "estimated_duration"
	FieldRecurrencePattern     = "recurrence_pattern"
	FieldRecurrenceConfig      = "recurrence_config"
	FieldDueOffset             = "due_offset"
	FieldIsActive              = "is_active"
	FieldSyncHorizonDays       = "sync_horizon_days"
	FieldGenerationHorizonDays = "generation_horizon_days"
)

// Field names for TodoItem update masks.
// These constants are used to identify which fields trigger detachment from recurring templates.
// Updating status does NOT trigger detachment (completing a recurring task is normal workflow).
const (
	// Content fields - trigger detachment
	FieldItemTitle             = "title" // Shares name with template
	FieldItemTags              = "tags"  // Shares name with template
	FieldItemPriority          = "priority"
	FieldItemEstimatedDuration = "estimated_duration"

	// Schedule fields - trigger detachment
	FieldDueAt    = "due_at"
	FieldStartsAt = "starts_at"
	FieldOccursAt = "occurs_at"

	// Status fields - do NOT trigger detachment
	FieldStatus         = "status"
	FieldActualDuration = "actual_duration"
	FieldTimezone       = "timezone"
)

// UpdateRecurringTemplateParams contains parameters for updating a recurring template with field mask support.
// Uses client-side optimistic concurrency control via etag (AIP-154).
type UpdateRecurringTemplateParams struct {
	TemplateID string
	ListID     string // Required for ownership validation

	// Etag for optimistic concurrency control.
	// Format: numeric string, e.g., "1", "2".
	// If provided and doesn't match current version, returns ErrVersionConflict.
	Etag *string

	// UpdateMask specifies which fields to update.
	// Only fields in this list will be modified.
	UpdateMask []string

	// Field values (only applied if field is in UpdateMask)
	Title                 *string
	Tags                  *[]string
	Priority              *TaskPriority
	EstimatedDuration     *time.Duration
	RecurrencePattern     *RecurrencePattern
	RecurrenceConfig      map[string]any
	DueOffset             *time.Duration
	IsActive              *bool
	SyncHorizonDays       *int
	GenerationHorizonDays *int
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
	RecurrenceConfig  map[string]any // Pattern-specific config as JSON
	DueOffset         *time.Duration // Optional offset for due time

	// Template state
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time

	// Generation tracking
	GeneratedThrough      time.Time // Last date we generated tasks through
	SyncHorizonDays       int       // Days to generate synchronously (default: 14)
	GenerationHorizonDays int       // Total async generation horizon (default: 365)

	// Optimistic locking version for concurrent update protection
	Version int
}

// Default generation horizons
const (
	DefaultSyncHorizonDays       = 14
	DefaultGenerationHorizonDays = 365
)

// Etag returns the entity tag for this template.
// The etag is based on the version number and is used for optimistic concurrency control.
func (t *RecurringTemplate) Etag() string {
	return fmt.Sprintf("%d", t.Version)
}

// RecurringTemplateException marks specific occurrences that should not be generated
// or should be treated differently from the template's standard pattern.
type RecurringTemplateException struct {
	ID            string
	TemplateID    string
	OccursAt      time.Time
	ExceptionType ExceptionType
	ItemID        *string // Reference to customized/detached item
	CreatedAt     time.Time
}

// ExceptionType indicates why this exception exists.
type ExceptionType string

const (
	// ExceptionTypeDeleted indicates user deleted this instance (soft delete).
	ExceptionTypeDeleted ExceptionType = "deleted"

	// ExceptionTypeRescheduled indicates user rescheduled this instance (detached).
	ExceptionTypeRescheduled ExceptionType = "rescheduled"

	// ExceptionTypeEdited indicates user customized this instance (keeps template link).
	ExceptionTypeEdited ExceptionType = "edited"
)

// Validate checks if the exception type is valid.
func (e ExceptionType) Validate() error {
	switch e {
	case ExceptionTypeDeleted, ExceptionTypeRescheduled, ExceptionTypeEdited:
		return nil
	default:
		return ErrInvalidExceptionType
	}
}

// GenerationJob is an entity representing a background job for generating recurring task instances.
//
// The worker uses these jobs to:
//  1. Schedule when to generate tasks
//  2. Track job status (pending, running, completed, failed, discarded)
//  3. Record errors and retry attempts
//  4. Handle stuck job recovery via availability timeout
type GenerationJob struct {
	ID           string
	TemplateID   string
	ScheduledFor time.Time
	Status       string // Use JobStatus* constants

	// Availability timeout for stuck job recovery
	AvailableAt time.Time  // When job becomes re-claimable if stuck
	ClaimedBy   *string    // Worker ID that claimed this job (nil if unclaimed)
	ClaimedAt   *time.Time // When job was claimed

	// Generation range
	GenerateFrom  time.Time
	GenerateUntil time.Time

	// Timestamps
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	FailedAt    *time.Time

	// Retry tracking
	ErrorMessage *string
	RetryCount   int // Current attempt count (starts at 0)
}

// Job status constants
const (
	JobStatusPending    = "pending"
	JobStatusScheduled  = "scheduled" // For future execution
	JobStatusRunning    = "running"
	JobStatusCompleted  = "completed"
	JobStatusFailed     = "failed"     // Transient, will retry
	JobStatusDiscarded  = "discarded"  // Permanently failed (max retries exceeded or permanent error)
	JobStatusCancelling = "cancelling" // Cancellation requested, waiting for worker
	JobStatusCancelled  = "cancelled"  // Successfully cancelled
)

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

// DeadLetterJob represents a permanently failed job that requires manual intervention.
//
// Dead letter jobs are created when:
//  1. Job exhausts all retry attempts (transient errors)
//  2. Job fails with permanent error (business logic error)
//  3. Job panics during execution (programming error)
//
// Admins can review dead letter jobs and either:
// - Retry: Create a new job from the dead letter entry
// - Discard: Mark as resolved without retrying
type DeadLetterJob struct {
	ID         string
	TemplateID string

	// Original job details
	GenerateFrom  time.Time
	GenerateUntil time.Time

	// Failure information
	ErrorType     string  // "permanent", "exhausted", or "panic"
	ErrorMessage  string  // Error message from the job
	StackTrace    *string // Stack trace for panics
	FailedAt      time.Time
	RetryCount    int    // Number of retries attempted before failure
	LastWorkerID  string // Worker that last processed the job
	OriginalJobID string // ID of the original job that failed

	// Resolution tracking
	ResolvedAt  *time.Time // When the dead letter was resolved
	ResolvedBy  *string    // Admin who resolved it
	Resolution  *string    // "retried" or "discarded"
	ReviewNotes *string    // Admin notes about resolution
	CreatedAt   time.Time
}
