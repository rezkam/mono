package integration

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/keygen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateLastUsed_FirstUsage verifies that UpdateLastUsed works when last_used_at is NULL.
// This is the first time an API key is used - should always update regardless of timestamp.
func TestUpdateLastUsed_FirstUsage(t *testing.T) {
	store, ctx := SetupTestStore(t)

	// Create API key (last_used_at will be NULL initially)
	fullKey, err := auth.CreateAPIKey(ctx, store, "sk", "mono", "v1", "Test Key", nil)
	require.NoError(t, err)

	// Parse key to get short token for lookup
	keyParts, err := keygen.ParseAPIKey(fullKey)
	require.NoError(t, err)

	// Verify initial state: last_used_at should be NULL
	apiKey, err := store.FindByShortToken(ctx, keyParts.ShortToken)
	require.NoError(t, err)
	assert.Nil(t, apiKey.LastUsedAt, "last_used_at should be NULL initially")

	// Update with a timestamp
	updateTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	err = store.UpdateLastUsed(ctx, apiKey.ID, updateTime)
	require.NoError(t, err)

	// Verify last_used_at is now set
	apiKey, err = store.FindByShortToken(ctx, keyParts.ShortToken)
	require.NoError(t, err)
	require.NotNil(t, apiKey.LastUsedAt, "last_used_at should be set after update")
	assert.Equal(t, updateTime, *apiKey.LastUsedAt, "last_used_at should match the update timestamp")
}

// TestUpdateLastUsed_EarlierTimestamp verifies that UpdateLastUsed does NOT update
// when the provided timestamp is earlier than the current value (idempotent behavior).
func TestUpdateLastUsed_EarlierTimestamp(t *testing.T) {
	store, ctx := SetupTestStore(t)

	// Create API key and set initial last_used_at
	fullKey, err := auth.CreateAPIKey(ctx, store, "sk", "mono", "v1", "Test Key", nil)
	require.NoError(t, err)

	keyParts, err := keygen.ParseAPIKey(fullKey)
	require.NoError(t, err)

	apiKey, err := store.FindByShortToken(ctx, keyParts.ShortToken)
	require.NoError(t, err)

	// Set initial timestamp (newer)
	newerTime := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	err = store.UpdateLastUsed(ctx, apiKey.ID, newerTime)
	require.NoError(t, err)

	// Verify it was set
	apiKey, err = store.FindByShortToken(ctx, keyParts.ShortToken)
	require.NoError(t, err)
	require.NotNil(t, apiKey.LastUsedAt)
	assert.Equal(t, newerTime, *apiKey.LastUsedAt)

	// Try to update with earlier timestamp (should be idempotent - no update)
	earlierTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	err = store.UpdateLastUsed(ctx, apiKey.ID, earlierTime)
	require.NoError(t, err, "should return success (idempotent)")

	// Verify last_used_at did NOT change (still the newer time)
	apiKey, err = store.FindByShortToken(ctx, keyParts.ShortToken)
	require.NoError(t, err)
	require.NotNil(t, apiKey.LastUsedAt)
	assert.Equal(t, newerTime, *apiKey.LastUsedAt, "last_used_at should remain unchanged when updating with earlier timestamp")
}

// TestUpdateLastUsed_LaterTimestamp verifies that UpdateLastUsed DOES update
// when the provided timestamp is later than the current value.
func TestUpdateLastUsed_LaterTimestamp(t *testing.T) {
	store, ctx := SetupTestStore(t)

	// Create API key and set initial last_used_at
	fullKey, err := auth.CreateAPIKey(ctx, store, "sk", "mono", "v1", "Test Key", nil)
	require.NoError(t, err)

	keyParts, err := keygen.ParseAPIKey(fullKey)
	require.NoError(t, err)

	apiKey, err := store.FindByShortToken(ctx, keyParts.ShortToken)
	require.NoError(t, err)

	// Set initial timestamp (earlier)
	earlierTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	err = store.UpdateLastUsed(ctx, apiKey.ID, earlierTime)
	require.NoError(t, err)

	// Verify it was set
	apiKey, err = store.FindByShortToken(ctx, keyParts.ShortToken)
	require.NoError(t, err)
	require.NotNil(t, apiKey.LastUsedAt)
	assert.Equal(t, earlierTime, *apiKey.LastUsedAt)

	// Update with later timestamp (should update)
	laterTime := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	err = store.UpdateLastUsed(ctx, apiKey.ID, laterTime)
	require.NoError(t, err)

	// Verify last_used_at was updated to the later time
	apiKey, err = store.FindByShortToken(ctx, keyParts.ShortToken)
	require.NoError(t, err)
	require.NotNil(t, apiKey.LastUsedAt)
	assert.Equal(t, laterTime, *apiKey.LastUsedAt, "last_used_at should be updated to later timestamp")
}

// TestUpdateLastUsed_EqualTimestamp verifies that UpdateLastUsed does NOT update
// when the provided timestamp equals the current value (edge case).
func TestUpdateLastUsed_EqualTimestamp(t *testing.T) {
	store, ctx := SetupTestStore(t)

	// Create API key and set initial last_used_at
	fullKey, err := auth.CreateAPIKey(ctx, store, "sk", "mono", "v1", "Test Key", nil)
	require.NoError(t, err)

	keyParts, err := keygen.ParseAPIKey(fullKey)
	require.NoError(t, err)

	apiKey, err := store.FindByShortToken(ctx, keyParts.ShortToken)
	require.NoError(t, err)

	// Set initial timestamp
	timestamp := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	err = store.UpdateLastUsed(ctx, apiKey.ID, timestamp)
	require.NoError(t, err)

	// Verify it was set
	apiKey, err = store.FindByShortToken(ctx, keyParts.ShortToken)
	require.NoError(t, err)
	require.NotNil(t, apiKey.LastUsedAt)
	assert.Equal(t, timestamp, *apiKey.LastUsedAt)

	// Try to update with same timestamp (should be idempotent - no update)
	err = store.UpdateLastUsed(ctx, apiKey.ID, timestamp)
	require.NoError(t, err, "should return success (idempotent)")

	// Verify last_used_at did NOT change
	apiKey, err = store.FindByShortToken(ctx, keyParts.ShortToken)
	require.NoError(t, err)
	require.NotNil(t, apiKey.LastUsedAt)
	assert.Equal(t, timestamp, *apiKey.LastUsedAt, "last_used_at should remain unchanged when updating with equal timestamp")
}

// TestUpdateLastUsed_NonExistentKey verifies that UpdateLastUsed returns ErrNotFound
// when attempting to update a non-existent API key.
func TestUpdateLastUsed_NonExistentKey(t *testing.T) {
	store, ctx := SetupTestStore(t)

	// Try to update a key that doesn't exist
	fakeID := "019b599a-0000-7000-8000-000000000000"
	timestamp := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	err := store.UpdateLastUsed(ctx, fakeID, timestamp)

	// Should return ErrNotFound
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound), "should return ErrNotFound for non-existent key, got: %v", err)
}

// TestUpdateLastUsed_InvalidUUID verifies that UpdateLastUsed returns ErrInvalidID
// when provided with an invalid UUID string.
func TestUpdateLastUsed_InvalidUUID(t *testing.T) {
	store, ctx := SetupTestStore(t)

	// Try to update with invalid UUID
	invalidID := "not-a-valid-uuid"
	timestamp := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	err := store.UpdateLastUsed(ctx, invalidID, timestamp)

	// Should return ErrInvalidID
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidID), "should return ErrInvalidID for invalid UUID, got: %v", err)
}

// TestUpdateLastUsed_ConcurrentUpdates verifies that concurrent updates with different timestamps
// result in the maximum timestamp being stored (no data loss or race conditions).
func TestUpdateLastUsed_ConcurrentUpdates(t *testing.T) {
	store, ctx := SetupTestStore(t)

	// Create API key
	fullKey, err := auth.CreateAPIKey(ctx, store, "sk", "mono", "v1", "Test Key", nil)
	require.NoError(t, err)

	keyParts, err := keygen.ParseAPIKey(fullKey)
	require.NoError(t, err)

	apiKey, err := store.FindByShortToken(ctx, keyParts.ShortToken)
	require.NoError(t, err)

	// Set initial timestamp
	initialTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	err = store.UpdateLastUsed(ctx, apiKey.ID, initialTime)
	require.NoError(t, err)

	// Prepare timestamps for concurrent updates
	timestamps := []time.Time{
		time.Date(2025, 1, 15, 11, 0, 0, 0, time.UTC),  // T1
		time.Date(2025, 1, 15, 12, 30, 0, 0, time.UTC), // T2
		time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC),  // T3
		time.Date(2025, 1, 15, 13, 0, 0, 0, time.UTC),  // T4 (maximum)
		time.Date(2025, 1, 15, 9, 0, 0, 0, time.UTC),   // T5 (earlier than initial)
	}

	// Launch concurrent updates
	var wg sync.WaitGroup
	for _, ts := range timestamps {
		wg.Add(1)
		go func(timestamp time.Time) {
			defer wg.Done()
			_ = store.UpdateLastUsed(ctx, apiKey.ID, timestamp)
		}(ts)
	}

	// Wait for all updates to complete
	wg.Wait()

	// Verify final value is the maximum timestamp
	apiKey, err = store.FindByShortToken(ctx, keyParts.ShortToken)
	require.NoError(t, err)
	require.NotNil(t, apiKey.LastUsedAt)

	// Final value should be T4 (13:00), the maximum timestamp
	expectedMax := time.Date(2025, 1, 15, 13, 0, 0, 0, time.UTC)
	assert.Equal(t, expectedMax, *apiKey.LastUsedAt,
		"concurrent updates should result in maximum timestamp being stored")
}
