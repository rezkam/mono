package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres/sqlcgen"
)

// === Auth Repository Implementation ===
// Implements application/auth.Repository interface (3 methods)

// FindByShortToken retrieves an API key by its short token for validation.
func (s *Store) FindByShortToken(ctx context.Context, shortToken string) (*domain.APIKey, error) {
	dbKey, err := s.queries.GetAPIKeyByShortToken(ctx, shortToken)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: API key", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	return dbAPIKeyToDomain(dbKey), nil
}

// UpdateLastUsed updates the last used timestamp for an API key.
// Only updates if the new timestamp is later than the current value (or current value is NULL).
// Returns success (nil) if timestamp is not later (idempotent behavior).
// Returns ErrNotFound if the API key doesn't exist.
func (s *Store) UpdateLastUsed(ctx context.Context, keyID string, timestamp time.Time) error {
	id, err := uuid.Parse(keyID)
	if err != nil {
		return fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	pgID := uuidToPgtype(id)
	params := sqlcgen.UpdateAPIKeyLastUsedParams{
		ID:         pgID,
		LastUsedAt: timeToPgtype(timestamp),
	}

	rowsAffected, err := s.queries.UpdateAPIKeyLastUsed(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to update last used: %w", err)
	}

	if rowsAffected == 0 {
		// Either key doesn't exist OR timestamp wasn't later
		// Check existence to distinguish these cases
		exists, err := s.queries.CheckAPIKeyExists(ctx, pgID)
		if err != nil {
			return fmt.Errorf("failed to check key existence: %w", err)
		}
		if !exists {
			return fmt.Errorf("%w: API key", domain.ErrNotFound)
		}
		// Key exists, timestamp just wasn't later - idempotent success
		return nil
	}

	return nil
}

// Create creates a new API key in storage.
func (s *Store) Create(ctx context.Context, key *domain.APIKey) error {
	id, err := uuid.Parse(key.ID)
	if err != nil {
		return fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	params := sqlcgen.CreateAPIKeyParams{
		ID:             uuidToPgtype(id),
		KeyType:        key.KeyType,
		Service:        key.Service,
		Version:        key.Version,
		ShortToken:     key.ShortToken,
		LongSecretHash: key.LongSecretHash,
		Name:           key.Name,
		IsActive:       key.IsActive,
		CreatedAt:      timeToPgtype(key.CreatedAt),
		ExpiresAt:      timePtrToPgtype(key.ExpiresAt),
	}

	if err := s.queries.CreateAPIKey(ctx, params); err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	return nil
}
