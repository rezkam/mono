package integration

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetList_NonExistentID verifies that fetching a non-existent list returns NotFound.
func TestGetList_NonExistentID(t *testing.T) {
	env := newListTasksTestEnv(t)

	nonExistentUUID, err := uuid.NewV7()
	require.NoError(t, err)
	nonExistentID := nonExistentUUID.String()

	_, err = env.Service().GetList(env.Context(), nonExistentID)
	require.Error(t, err)

	assert.True(t, errors.Is(err, domain.ErrListNotFound),
		"Non-existent list should return ErrListNotFound")
}
