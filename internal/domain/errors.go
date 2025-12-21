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
	ErrTitleRequired                 = errors.New("title is required")
	ErrTitleTooLong                  = errors.New("title must be 255 characters or less")
	ErrInvalidTaskStatus             = errors.New("invalid task status")
	ErrInvalidTaskPriority           = errors.New("invalid task priority")
	ErrInvalidRecurrencePattern      = errors.New("invalid recurrence pattern")
	ErrRecurringTaskRequiresTemplate = errors.New("recurring task must have template ID")
	ErrInvalidGenerationWindow       = errors.New("generation window must be 1-365 days")

	// Business logic errors
	ErrItemNotFound     = errors.New("item not found")
	ErrTemplateNotFound = errors.New("recurring template not found")
	ErrUnauthorized     = errors.New("unauthorized")

	// Concurrency errors
	ErrVersionConflict = errors.New("resource was modified by another request")
)
