package domain

import (
	"testing"
	"time"
)

// TestUpdateItemStatus_UsesUTC verifies that UpdateItemStatus uses UTC timestamps.
// This ensures consistency with other timestamp fields in the domain (CreateTime, CreatedAt, etc.)
// which all use UTC to avoid timezone-related comparison and sorting issues.
func TestUpdateItemStatus_UsesUTC(t *testing.T) {
	// Create a list with one item
	item := TodoItem{
		ID:         "item-1",
		Title:      "Test item",
		ListID:     "list-1",
		Status:     TaskStatusTodo,
		CreateTime: time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	list := &TodoList{
		ID:         "list-1",
		Title:      "Test list",
		Items:      []TodoItem{item},
		CreateTime: time.Now().UTC(),
	}

	// Update the item status
	updated := list.UpdateItemStatus("item-1", TaskStatusInProgress)
	if !updated {
		t.Fatal("expected item to be updated")
	}

	// Verify the timestamp is in UTC
	updatedItem := list.Items[0]
	if updatedItem.UpdatedAt.Location() != time.UTC {
		t.Errorf("expected UpdatedAt to use UTC, got location: %v", updatedItem.UpdatedAt.Location())
	}
}
