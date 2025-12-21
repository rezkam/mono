package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFieldMask_MultipleFields verifies that multiple fields can be updated at once with a field mask.
func TestFieldMask_MultipleFields(t *testing.T) {
	env := newListTasksTestEnv(t)

	list, err := env.Service().CreateList(env.Context(), "Multi-Field Update Test")
	require.NoError(t, err)
	listID := list.ID

	itemUUID, err := uuid.NewV7()
	require.NoError(t, err)
	itemID := itemUUID.String()

	priorityLow := domain.TaskPriorityLow
	timezone := "America/New_York"
	dueTime := time.Now().Add(48 * time.Hour).UTC()

	item := &domain.TodoItem{
		ID:         itemID,
		Title:      "Original Title",
		Status:     domain.TaskStatusTodo,
		Priority:   &priorityLow,
		Tags:       []string{"old-tag"},
		DueTime:    &dueTime,
		Timezone:   &timezone,
		CreateTime: time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	require.NoError(t, env.Store().CreateItem(env.Context(), listID, item))

	// Update multiple fields by fetching, modifying, then updating
	existingItem, err := env.Service().GetItem(env.Context(), itemID)
	require.NoError(t, err)

	newDueTime := time.Now().Add(72 * time.Hour).UTC()
	priorityHigh := domain.TaskPriorityHigh
	existingItem.Title = "Updated Title"
	existingItem.Status = domain.TaskStatusInProgress
	existingItem.Priority = &priorityHigh
	existingItem.Tags = []string{"new-tag-1", "new-tag-2"}
	existingItem.DueTime = &newDueTime

	err = env.Service().UpdateItem(env.Context(), listID, existingItem)
	require.NoError(t, err)

	// Verify updated fields
	updatedItem, err := env.Service().GetItem(env.Context(), itemID)
	require.NoError(t, err)

	assert.Equal(t, "Updated Title", updatedItem.Title)
	assert.Equal(t, domain.TaskStatusInProgress, updatedItem.Status)
	require.NotNil(t, updatedItem.Priority)
	assert.Equal(t, domain.TaskPriorityHigh, *updatedItem.Priority)
	assert.ElementsMatch(t, []string{"new-tag-1", "new-tag-2"}, updatedItem.Tags)
	require.NotNil(t, updatedItem.DueTime)
	assert.Equal(t, newDueTime.Unix(), updatedItem.DueTime.Unix())

	fetchedItem, err := env.Store().FindItemByID(env.Context(), itemID)
	require.NoError(t, err)
	assert.NotNil(t, fetchedItem.Timezone)
	assert.Equal(t, "America/New_York", *fetchedItem.Timezone,
		"Timezone should be preserved (not in field mask)")
}
