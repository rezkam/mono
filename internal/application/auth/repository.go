package auth

import (
	"context"
	"time"

	"github.com/rezkam/mono/internal/domain"
)

// Repository defines storage operations for authentication.
type Repository interface {
	// FindByShortToken retrieves an API key by its short token for validation.
	// Returns error if key not found.
	FindByShortToken(ctx context.Context, shortToken string) (*domain.APIKey, error)

	// UpdateLastUsed updates the last used timestamp for an API key.
	UpdateLastUsed(ctx context.Context, keyID string, timestamp time.Time) error

	// Create creates a new API key.
	// Returns error if creation fails (e.g., duplicate key).
	Create(ctx context.Context, key *domain.APIKey) error
}
