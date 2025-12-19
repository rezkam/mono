package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListTasks_LargePageSize ensures ListTasks enforces server-side limits and pagination tokens.
func TestListTasks_LargePageSize(t *testing.T) {
	env := newListTasksTestEnv(t)

	createListResp, err := env.Service().CreateList(env.Context(), &monov1.CreateListRequest{
		Title: "Page Size Test List",
	})
	require.NoError(t, err)
	listID := createListResp.List.Id

	// Seed 150 items so multiple pages are required.
	for i := 0; i < 150; i++ {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)

		now := time.Now().UTC()
		item := &domain.TodoItem{
			ID:         itemUUID.String(),
			Title:      "Test Item",
			Status:     domain.TaskStatusTodo,
			CreateTime: now,
			UpdatedAt:  now,
		}
		require.NoError(t, env.Store().CreateItem(env.Context(), listID, item))
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resp, err := env.Service().ListTasks(env.Context(), &monov1.ListTasksRequest{
				Parent:   listID,
				PageSize: tc.requestPageSize,
			})
			require.NoError(t, err)

			assert.LessOrEqual(t, len(resp.Items), tc.expectedMaxItems,
				"Response should not exceed expected max items")

			totalReturned := len(resp.Items)
			nextToken := resp.NextPageToken

			// Any scenario with fewer than 150 items per page should yield more pages.
			if tc.expectedMaxItems < 150 {
				assert.NotEmpty(t, nextToken, "Should expose a next_page_token when more results exist")
			}

			for nextToken != "" {
				resp, err = env.Service().ListTasks(env.Context(), &monov1.ListTasksRequest{
					Parent:    listID,
					PageSize:  tc.requestPageSize,
					PageToken: nextToken,
				})
				require.NoError(t, err)

				totalReturned += len(resp.Items)
				nextToken = resp.NextPageToken
			}

			assert.Equal(t, 150, totalReturned, "Should page through all items")
		})
	}
}
