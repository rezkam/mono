package core

import (
	"context"
	"time"
)

// TodoItem represents a single task within a list.
type TodoItem struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Completed  bool      `json:"completed"`
	CreateTime time.Time `json:"create_time"`
	DueTime    time.Time `json:"due_time,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
}

// TodoList represents a collection of tasks.
type TodoList struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Items      []TodoItem `json:"items"`
	CreateTime time.Time  `json:"create_time"`
}

// AddItem adds a new item to the list.
func (l *TodoList) AddItem(item TodoItem) {
	l.Items = append(l.Items, item)
}

// UpdateItemStatus updates the status of an item in the list.
// Returns true if the item was found and updated.
func (l *TodoList) UpdateItemStatus(itemID string, completed bool) bool {
	for i, item := range l.Items {
		if item.ID == itemID {
			l.Items[i].Completed = completed
			return true
		}
	}
	return false
}

// Storage defines the interface for persisting TodoLists.
// implementations can be file-based, cloud-storage based, etc.
type Storage interface {
	// CreateList creates a new TodoList.
	CreateList(ctx context.Context, list *TodoList) error

	// GetList retrieves a TodoList by its ID.
	GetList(ctx context.Context, id string) (*TodoList, error)

	// UpdateList updates an existing TodoList (e.g. adding items, changing status).
	UpdateList(ctx context.Context, list *TodoList) error

	// ListLists returns all available TodoLists.
	ListLists(ctx context.Context) ([]*TodoList, error)
}
