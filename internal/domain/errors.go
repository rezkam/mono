package domain

import "errors"

// Domain errors - these are returned by repository implementations
// and checked by the service layer.

var (
	// ErrNotFound indicates the requested resource does not exist.
	// Service layer should map this to codes.NotFound.
	ErrNotFound = errors.New("resource not found")

	// ErrListNotFound indicates the specified list does not exist.
	// Service layer should map this to codes.NotFound.
	ErrListNotFound = errors.New("list not found")

	// ErrInvalidID indicates the provided ID format is invalid.
	// Service layer should map this to codes.InvalidArgument.
	ErrInvalidID = errors.New("invalid ID format")
)
