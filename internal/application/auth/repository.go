package auth

import (
	"context"
	"time"

	"github.com/rezkam/mono/internal/domain"
)

// Repository defines storage operations for authentication.
//
// This interface is owned by the auth package (consumer), not by the storage package (provider).
// Following the Dependency Inversion Principle: high-level modules (auth) should not depend on
// low-level modules (storage); both should depend on abstractions (this interface).
//
// Interface Segregation: Only 3 methods needed by authenticator, not 25+.
type Repository interface {
	// FindByShortToken retrieves an API key by its short token for validation.
	// Returns error if key not found or database error occurs.
	//
	// Used by: validateAPIKey() during gRPC interceptor authentication
	FindByShortToken(ctx context.Context, shortToken string) (*domain.APIKey, error)

	// UpdateLastUsed updates the last used timestamp for an API key.
	// This is a non-critical operation - failures are logged but don't block authentication.
	//
	// Used by: processLastUsedUpdates() background worker
	UpdateLastUsed(ctx context.Context, keyID string, timestamp time.Time) error

	// Create creates a new API key in storage.
	// Returns error if creation fails (e.g., duplicate key, database error).
	//
	// Used by: CreateAPIKey() function and cmd/apikey CLI tool
	Create(ctx context.Context, key *domain.APIKey) error
}
