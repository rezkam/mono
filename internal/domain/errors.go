package domain

import "errors"

// Domain errors returned by repository implementations.

var (
	// ErrNotFound indicates the requested resource does not exist.
	ErrNotFound = errors.New("resource not found")

	// ErrListNotFound indicates the specified list does not exist.
	ErrListNotFound = errors.New("list not found")

	// ErrInvalidID indicates the provided ID format is invalid.
	ErrInvalidID = errors.New("invalid ID format")

	// Validation errors
	ErrEmptyUpdateMask               = errors.New("update_mask cannot be empty")
	ErrInvalidRequest                = errors.New("invalid request payload")
	ErrUnknownField                  = errors.New("unknown field in update_mask")
	ErrTitleRequired                 = errors.New("title is required")
	ErrTitleTooLong                  = errors.New("title must be 255 characters or less")
	ErrStatusRequired                = errors.New("status value is required when status is in update_mask")
	ErrRecurrencePatternRequired     = errors.New("recurrence_pattern value is required when recurrence_pattern is in update_mask")
	ErrRecurrenceConfigRequired      = errors.New("recurrence_config value is required when recurrence_config is in update_mask")
	ErrInvalidTaskStatus             = errors.New("invalid task status")
	ErrInvalidTaskPriority           = errors.New("invalid task priority")
	ErrInvalidRecurrencePattern      = errors.New("invalid recurrence pattern")
	ErrRecurringTaskRequiresTemplate = errors.New("recurring task must have template ID")
	ErrInvalidGenerationWindow       = errors.New("generation window must be 1-365 days")
	ErrInvalidEtagFormat             = errors.New("etag must be a numeric string (e.g., \"1\", \"2\")")
	ErrInvalidOrderByField           = errors.New("invalid order_by field")
	ErrInvalidListsOrderByField      = errors.New("invalid order_by field for lists")
	ErrInvalidDurationFormat         = errors.New("invalid duration format")
	ErrDurationEmpty                 = errors.New("duration cannot be empty")
	ErrInvalidRecurrenceDayOfWeek    = errors.New("invalid recurrence day of week")
	ErrInvalidTimezone               = errors.New("invalid timezone")
	ErrInvalidPageToken              = errors.New("invalid page token")
	ErrInvalidLimit                  = errors.New("invalid limit value")
	ErrInvalidCursorFormat           = errors.New("invalid cursor format")
	ErrUnsupportedFilterOperator     = errors.New("unsupported filter operator")
	ErrUnsupportedFieldType          = errors.New("unsupported field type")
	ErrInvalidVersionFormat          = errors.New("invalid version format")
	ErrInvalidSortDirection          = errors.New("invalid sort direction")
	ErrTooManyStatuses               = errors.New("too many statuses in filter")
	ErrTooManyPriorities             = errors.New("too many priorities in filter")
	ErrTooManyTags                   = errors.New("too many tags in filter")
	ErrFilterParsingNotImplemented   = errors.New("filter parsing not yet implemented")

	// Business logic errors
	ErrItemNotFound     = errors.New("item not found")
	ErrTemplateNotFound = errors.New("recurring template not found")
	ErrUnauthorized     = errors.New("unauthorized")

	// Exception errors
	ErrInvalidExceptionType   = errors.New("invalid exception type")
	ErrExceptionNotFound      = errors.New("exception not found")
	ErrExceptionAlreadyExists = errors.New("exception already exists for this occurrence")

	// Job coordination errors
	ErrJobNotFound        = errors.New("generation job not found")
	ErrJobAlreadyClaimed  = errors.New("job already claimed by another worker")
	ErrJobNotCancellable  = errors.New("job is not in a cancellable state")
	ErrDeadLetterNotFound = errors.New("dead letter job not found")
	ErrJobOwnershipLost   = errors.New("job ownership lost to another worker")

	// Concurrency errors
	ErrVersionConflict = errors.New("resource was modified by another request")

	// Configuration errors
	ErrNameRequired              = errors.New("name is required")
	ErrInvalidDays               = errors.New("days must be >= 0")
	ErrUnsupportedStorageType    = errors.New("unsupported storage type")
	ErrStorageDSNRequired        = errors.New("storage DSN is required")
	ErrInvalidOperationTimeout   = errors.New("operation timeout must be at least 1 second")
	ErrValidationTargetNotStruct = errors.New("validation target must be a struct pointer")
	ErrUnsupportedType           = errors.New("unsupported type")
	ErrInvalidAPIKeyFormat       = errors.New("invalid API key format")
	ErrAPIKeyParsingFailed       = errors.New("could not parse API key from tool output")

	// Test-only errors
	ErrNotImplemented      = errors.New("not implemented")
	ErrDatabaseUnavailable = errors.New("database unavailable")
	ErrFailedToCreateTask  = errors.New("failed to create task")
	ErrCompletionFailed    = errors.New("completion failed")

	// Generation configuration errors
	ErrHardDeleteNotImplemented  = errors.New("hard delete for non-recurring items not implemented")
	ErrSyncHorizonMustBePositive = errors.New("sync_horizon_days must be positive")
	ErrUnsupportedJobStatus      = errors.New("unsupported job status")
)
