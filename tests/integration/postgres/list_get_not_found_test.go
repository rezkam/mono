package integration

import (
	"testing"

	"github.com/google/uuid"
	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestGetList_NonExistentID verifies that fetching a non-existent list returns NotFound.
func TestGetList_NonExistentID(t *testing.T) {
	env := newListTasksTestEnv(t)

	nonExistentUUID, err := uuid.NewV7()
	require.NoError(t, err)
	nonExistentID := nonExistentUUID.String()

	_, err = env.Service().GetList(env.Context(), &monov1.GetListRequest{
		Id: nonExistentID,
	})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status error")
	assert.Equal(t, codes.NotFound, st.Code(), "Non-existent list should return NotFound")
	assert.Contains(t, st.Message(), nonExistentID,
		"Error message should mention the list ID")
}
