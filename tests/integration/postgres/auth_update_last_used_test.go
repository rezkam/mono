package integration

import (
	"testing"
	"time"

	"github.com/rezkam/mono/internal/application/auth"
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
