package repository

import "errors"

// Sentinel errors for distinguishing between different error types.
// These allow service layer to map repository errors to appropriate gRPC status codes.
var (
	// ErrNotFound indicates the requested resource does not exist.
	// Service layer should map this to codes.NotFound.
	ErrNotFound = errors.New("resource not found")

	// ErrInvalidID indicates the provided ID format is invalid.
	// Service layer should map this to codes.InvalidArgument.
	ErrInvalidID = errors.New("invalid ID format")
)
