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
)
