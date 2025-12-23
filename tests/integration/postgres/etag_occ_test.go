package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/rezkam/mono/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEtag_ReturnedInItemResponse verifies that items include an etag field
// in the format specified by AIP-154 (quoted string, e.g., "1").
func TestEtag_ReturnedInItemResponse(t *testing.T) {
	dsn := GetTestStorageDSN(t)

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	todoService := todo.NewService(store, todo.Config{})

	// Create a list and item
	listID := createTestList(t, store, "Etag Test List")
	item := createTestItem(t, todoService, listID, "Test Item")

	// VERIFY: Etag is correct immediately after creation (without refetch)
	createdEtag := item.Etag()
	assert.Equal(t, "1", createdEtag, "Newly created item should have etag \"1\" immediately")

	// Fetch the item
	fetchedItem, err := todoService.GetItem(ctx, item.ID)
	require.NoError(t, err)

	// VERIFY: Etag is returned as numeric string
	etag := fetchedItem.Etag()
	assert.NotEmpty(t, etag, "Etag should not be empty")
	assert.Equal(t, "1", etag, "Fetched item should have etag \"1\"")
}

// TestEtag_UpdateWithMatchingEtag verifies that an update with a matching
// etag succeeds and returns the updated item with new etag.
func TestEtag_UpdateWithMatchingEtag(t *testing.T) {
	dsn := GetTestStorageDSN(t)

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	todoService := todo.NewService(store, todo.Config{})

	// Create a list and item
	listID := createTestList(t, store, "Etag Update Test")
	item := createTestItem(t, todoService, listID, "Original Title")

	// Fetch to get etag
	fetchedItem, err := todoService.GetItem(ctx, item.ID)
	require.NoError(t, err)
	originalEtag := fetchedItem.Etag()
	require.Equal(t, "1", originalEtag)

	// Update with matching etag
	updateParams := domain.UpdateItemParams{
		ItemID:     item.ID,
		ListID:     listID,
		Etag:       ptr.To(originalEtag),
		UpdateMask: []string{"title"},
		Title:      ptr.To("Updated Title"),
	}

	updatedItem, err := todoService.UpdateItem(ctx, updateParams)
	require.NoError(t, err, "Update with matching etag should succeed")

	// VERIFY: New etag is returned and title updated
	assert.Equal(t, "2", updatedItem.Etag(), "Etag should be incremented after update")
	assert.Equal(t, "Updated Title", updatedItem.Title)
}

// TestEtag_UpdateWithMismatchingEtag verifies that an update with a
// mismatching etag returns ErrVersionConflict.
func TestEtag_UpdateWithMismatchingEtag(t *testing.T) {
	dsn := GetTestStorageDSN(t)

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	todoService := todo.NewService(store, todo.Config{})

	// Create a list and item
	listID := createTestList(t, store, "Etag Conflict Test")
	item := createTestItem(t, todoService, listID, "Original Title")

	// Try to update with wrong etag
	updateParams := domain.UpdateItemParams{
		ItemID:     item.ID,
		ListID:     listID,
		Etag:       ptr.To("999"), // Wrong etag
		UpdateMask: []string{"title"},
		Title:      ptr.To("Updated Title"),
	}

	_, err = todoService.UpdateItem(ctx, updateParams)

	// VERIFY: Should fail with ErrVersionConflict
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrVersionConflict)
}

// TestEtag_UpdateWithoutEtag verifies that an update without etag
// succeeds (no version check performed).
func TestEtag_UpdateWithoutEtag(t *testing.T) {
	dsn := GetTestStorageDSN(t)

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	todoService := todo.NewService(store, todo.Config{})

	// Create a list and item
	listID := createTestList(t, store, "Etag Optional Test")
	item := createTestItem(t, todoService, listID, "Original Title")

	// Update WITHOUT etag - should succeed without version check
	updateParams := domain.UpdateItemParams{
		ItemID:     item.ID,
		ListID:     listID,
		Etag:       nil, // No etag - skip version check
		UpdateMask: []string{"title"},
		Title:      ptr.To("Updated Title"),
	}

	updatedItem, err := todoService.UpdateItem(ctx, updateParams)

	// VERIFY: Should succeed
	require.NoError(t, err, "Update without etag should succeed")
	assert.Equal(t, "Updated Title", updatedItem.Title)
	assert.Equal(t, "2", updatedItem.Etag(), "Version should still increment")
}

// TestEtag_ConcurrentUpdates verifies that concurrent updates with etags
// properly detect conflicts (client-side OCC).
func TestEtag_ConcurrentUpdates(t *testing.T) {
	dsn := GetTestStorageDSN(t)

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	todoService := todo.NewService(store, todo.Config{})

	// Create a list and item
	listID := createTestList(t, store, "Etag Concurrent Test")
	item := createTestItem(t, todoService, listID, "Original Title")

	// Client A fetches item
	clientA_item, err := todoService.GetItem(ctx, item.ID)
	require.NoError(t, err)
	clientA_etag := clientA_item.Etag()

	// Client B fetches same item (same etag)
	clientB_item, err := todoService.GetItem(ctx, item.ID)
	require.NoError(t, err)
	clientB_etag := clientB_item.Etag()

	require.Equal(t, clientA_etag, clientB_etag, "Both clients have same etag initially")

	// Client A updates first (succeeds)
	updateA := domain.UpdateItemParams{
		ItemID:     item.ID,
		ListID:     listID,
		Etag:       ptr.To(clientA_etag),
		UpdateMask: []string{"title"},
		Title:      ptr.To("Client A Update"),
	}
	_, errA := todoService.UpdateItem(ctx, updateA)
	require.NoError(t, errA, "Client A update should succeed")

	// Client B tries to update with stale etag (fails)
	updateB := domain.UpdateItemParams{
		ItemID:     item.ID,
		ListID:     listID,
		Etag:       ptr.To(clientB_etag), // Stale etag!
		UpdateMask: []string{"title"},
		Title:      ptr.To("Client B Update"),
	}
	_, errB := todoService.UpdateItem(ctx, updateB)

	// VERIFY: Client B gets conflict
	require.Error(t, errB)
	assert.ErrorIs(t, errB, domain.ErrVersionConflict)

	// Final state should be Client A's update
	finalItem, err := todoService.GetItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "Client A Update", finalItem.Title)
	assert.Equal(t, "2", finalItem.Etag())
}

// TestEtag_FieldMaskUpdate verifies that field mask updates only modify
// specified fields while preserving others.
func TestEtag_FieldMaskUpdate(t *testing.T) {
	dsn := GetTestStorageDSN(t)

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	todoService := todo.NewService(store, todo.Config{})

	// Create a list
	listID := createTestList(t, store, "Field Mask Test")

	// Create item with multiple fields set
	itemID := newUUID(t)
	item := &domain.TodoItem{
		ID:         itemID,
		Title:      "Original Title",
		Status:     domain.TaskStatusTodo,
		Priority:   ptr.To(domain.TaskPriorityHigh),
		Tags:       []string{"original", "test"},
		CreateTime: time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	_, err = todoService.CreateItem(ctx, listID, item)
	require.NoError(t, err)

	// Fetch to get etag
	fetchedItem, err := todoService.GetItem(ctx, itemID)
	require.NoError(t, err)

	// Update only title using field mask
	updateParams := domain.UpdateItemParams{
		ItemID:     itemID,
		ListID:     listID,
		Etag:       ptr.To(fetchedItem.Etag()),
		UpdateMask: []string{"title"}, // Only update title
		Title:      ptr.To("Updated Title"),
	}

	updatedItem, err := todoService.UpdateItem(ctx, updateParams)
	require.NoError(t, err)

	// VERIFY: Title changed, other fields preserved
	assert.Equal(t, "Updated Title", updatedItem.Title)
	assert.Equal(t, domain.TaskStatusTodo, updatedItem.Status, "Status should be preserved")
	assert.Equal(t, domain.TaskPriorityHigh, *updatedItem.Priority, "Priority should be preserved")
	assert.Equal(t, []string{"original", "test"}, updatedItem.Tags, "Tags should be preserved")
}

// TestEtag_CannotBeSetByClient verifies that etag value in payload is ignored.
// Clients should not be able to set etag to arbitrary values - it's always auto-generated.
func TestEtag_CannotBeSetByClient(t *testing.T) {
	dsn := GetTestStorageDSN(t)

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dsn)
	require.NoError(t, err)
	defer store.Close()

	db, cleanup := SetupTestDB(t)
	defer cleanup()
	_ = db

	todoService := todo.NewService(store, todo.Config{})

	// Create a list and item
	listID := createTestList(t, store, "Etag Protection Test")
	item := createTestItem(t, todoService, listID, "Test Item")
	require.Equal(t, "1", item.Etag(), "Initial etag should be \"1\"")

	// Fetch the item to get current etag for OCC
	fetchedItem, err := todoService.GetItem(ctx, item.ID)
	require.NoError(t, err)
	currentEtag := fetchedItem.Etag()

	// Try to update with etag in update_mask AND provide a bogus etag value
	// Note: The etag in params.Etag is for OCC check, not for setting
	// But let's verify that even if we include "etag" in UpdateMask, it's ignored
	updateParams := domain.UpdateItemParams{
		ItemID:     item.ID,
		ListID:     listID,
		Etag:       ptr.To(currentEtag),       // For OCC check - should match "1"
		UpdateMask: []string{"title", "etag"}, // Try to include "etag" in mask
		Title:      ptr.To("Updated Title"),
	}

	updated, err := todoService.UpdateItem(ctx, updateParams)
	require.NoError(t, err, "Update should succeed")

	// VERIFY: Etag was auto-incremented to "2", not manipulated by client
	assert.Equal(t, "2", updated.Etag(), "Etag should auto-increment to \"2\", ignoring 'etag' in update_mask")
	assert.Equal(t, "Updated Title", updated.Title, "Title should be updated")
}

// Helper functions

func createTestList(t *testing.T, store *postgres.Store, title string) string {
	t.Helper()
	listID := newUUID(t)
	list := &domain.TodoList{
		ID:         listID,
		Title:      title,
		CreateTime: time.Now().UTC(),
		Items:      []domain.TodoItem{},
	}
	err := store.CreateList(context.Background(), list)
	require.NoError(t, err)
	return listID
}

func createTestItem(t *testing.T, svc *todo.Service, listID, title string) *domain.TodoItem {
	t.Helper()
	item := &domain.TodoItem{
		ID:         newUUID(t),
		Title:      title,
		Status:     domain.TaskStatusTodo,
		CreateTime: time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	created, err := svc.CreateItem(context.Background(), listID, item)
	require.NoError(t, err)
	return created
}

func newUUID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	require.NoError(t, err)
	return id.String()
}
