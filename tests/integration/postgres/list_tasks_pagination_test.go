package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListTasks_LargePageSize ensures ListTasks enforces server-side limits and pagination tokens.
func TestListTasks_LargePageSize(t *testing.T) {
	env := newListTasksTestEnv(t)

	list, err := env.Service().CreateList(env.Context(), "Page Size Test List")
	require.NoError(t, err)
	listID := list.ID

	// Seed 150 items so multiple pages are required.
	for range 150 {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)

		now := time.Now().UTC()
		item := &domain.TodoItem{
			ID:        itemUUID.String(),
			Title:     "Test Item",
			Status:    domain.TaskStatusTodo,
			CreatedAt: now,
			UpdatedAt: now,
		}
		_, err = env.Store().CreateItem(env.Context(), listID, item)
		require.NoError(t, err)
	}

	testCases := []struct {
		name             string
		requestPageSize  int32
		expectedMaxItems int
	}{
		{"page_size_200_should_cap_to_100", 200, 100},
		{"page_size_1000_should_cap_to_100", 1000, 100},
		{"page_size_negative_uses_default", -1, 50},
		{"page_size_zero_uses_default", 0, 50},
		{"page_size_50_within_limit", 50, 50},
		{"page_size_100_at_limit", 100, 100},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params := domain.ListTasksParams{
				ListID: &listID,
				Limit:  int(tc.requestPageSize),
				Offset: 0,
			}
			result, err := env.Service().ListItems(env.Context(), params)
			require.NoError(t, err)

			assert.LessOrEqual(t, len(result.Items), tc.expectedMaxItems,
				"Response should not exceed expected max items")

			totalReturned := len(result.Items)
			hasMore := result.HasMore

			// Any scenario with fewer than 150 items per page should yield more pages.
			if tc.expectedMaxItems < 150 {
				assert.True(t, hasMore, "Should have more results when page size < total items")
			}

			offset := len(result.Items)
			for hasMore {
				params.Offset = offset
				result, err = env.Service().ListItems(env.Context(), params)
				require.NoError(t, err)

				totalReturned += len(result.Items)
				offset += len(result.Items)
				hasMore = result.HasMore
			}

			assert.Equal(t, 150, totalReturned, "Should page through all items")
		})
	}
}
