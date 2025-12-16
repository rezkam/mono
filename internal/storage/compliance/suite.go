package compliance

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RunStorageComplianceTest runs a standard set of tests against a Storage implementation.
// setup is a function that returns a fresh (clean) Storage instance for the test.
// cleanup is called after the test to clean up resources (if any).
func RunStorageComplianceTest(t *testing.T, setup func() (core.Storage, func())) {
	t.Run("CreateAndGetList", func(t *testing.T) {
		store, teardown := setup()
		defer teardown()
		ctx := context.Background()

		list := &core.TodoList{
			ID:    uuid.New().String(),
			Title: "Test List",
			Items: []core.TodoItem{},
		}

		err := store.CreateList(ctx, list)
		require.NoError(t, err)

		fetched, err := store.GetList(ctx, list.ID)
		require.NoError(t, err)
		assert.Equal(t, list.ID, fetched.ID)
		assert.Equal(t, list.Title, fetched.Title)
		assert.Empty(t, fetched.Items)
	})

	t.Run("UpdateList", func(t *testing.T) {
		store, teardown := setup()
		defer teardown()
		ctx := context.Background()

		list := &core.TodoList{
			ID:    uuid.New().String(),
			Title: "Test List",
			Items: []core.TodoItem{},
		}
		require.NoError(t, store.CreateList(ctx, list))

		// Add item
		newItem := core.TodoItem{ID: "item-1", Title: "Task 1", Completed: false}
		list.AddItem(newItem)

		err := store.UpdateList(ctx, list)
		require.NoError(t, err)

		fetched, err := store.GetList(ctx, list.ID)
		require.NoError(t, err)
		require.Len(t, fetched.Items, 1)
		assert.Equal(t, "Task 1", fetched.Items[0].Title)
	})

	t.Run("ListLists", func(t *testing.T) {
		store, teardown := setup()
		defer teardown()
		ctx := context.Background()

		list1 := &core.TodoList{ID: uuid.New().String(), Title: "List 1", Items: []core.TodoItem{}}
		list2 := &core.TodoList{ID: uuid.New().String(), Title: "List 2", Items: []core.TodoItem{}}

		require.NoError(t, store.CreateList(ctx, list1))
		require.NoError(t, store.CreateList(ctx, list2))

		lists, err := store.ListLists(ctx)
		require.NoError(t, err)

		// Map IDs for easy lookup
		ids := make(map[string]bool)
		for _, l := range lists {
			ids[l.ID] = true
		}

		assert.True(t, ids[list1.ID])
		assert.True(t, ids[list2.ID])
	})

	t.Run("GetNonExistentList", func(t *testing.T) {
		store, teardown := setup()
		defer teardown()
		ctx := context.Background()

		_, err := store.GetList(ctx, "non-existent-id")
		assert.Error(t, err)
	})
}
