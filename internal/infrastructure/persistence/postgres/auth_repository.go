package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres/sqlcgen"
)

// === Auth Repository Implementation ===
// Implements application/auth.Repository interface (3 methods)

// FindByShortToken retrieves an API key by its short token for validation.
func (s *Store) FindByShortToken(ctx context.Context, shortToken string) (*domain.APIKey, error) {
	dbKey, err := s.queries.GetAPIKeyByShortToken(ctx, shortToken)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: API key", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	return dbAPIKeyToDomain(dbKey), nil
}

// UpdateLastUsed updates the last used timestamp for an API key.
func (s *Store) UpdateLastUsed(ctx context.Context, keyID string, timestamp time.Time) error {
	id, err := uuid.Parse(keyID)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	params := sqlcgen.UpdateAPIKeyLastUsedParams{
		ID: id,
		LastUsedAt: sql.NullTime{
			Time:  timestamp,
			Valid: true,
		},
	}

	// Single-query pattern: check rowsAffected to detect revoked/deleted API key
	rowsAffected, err := s.queries.UpdateAPIKeyLastUsed(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to update last used: %w", err)
	}

	return checkRowsAffected(rowsAffected, "API key", keyID)
}

// Create creates a new API key in storage.
func (s *Store) Create(ctx context.Context, key *domain.APIKey) error {
	id, err := uuid.Parse(key.ID)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	params := sqlcgen.CreateAPIKeyParams{
		ID:             id,
		KeyType:        key.KeyType,
		Service:        key.Service,
		Version:        key.Version,
		ShortToken:     key.ShortToken,
		LongSecretHash: key.LongSecretHash,
		Name:           key.Name,
		IsActive:       key.IsActive,
		CreatedAt:      key.CreatedAt,
	}

	if key.ExpiresAt != nil {
		params.ExpiresAt = sql.NullTime{
			Time:  *key.ExpiresAt,
			Valid: true,
		}
	}

	if err := s.queries.CreateAPIKey(ctx, params); err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	return nil
}
