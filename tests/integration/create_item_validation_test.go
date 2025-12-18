package integration_test

import (
	"context"
	"testing"

	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/service"
	sqlstorage "github.com/rezkam/mono/internal/storage/sql"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestCreateItem_InvalidListID verifies that CreateItem returns NotFound
// for non-existent list IDs instead of Internal (FK violation).
// This test follows TDD approach and will FAIL until the implementation is fixed.
func TestCreateItem_InvalidListID(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := sqlstorage.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	svc := service.NewMonoService(store, 50, 100)

	// Try to create an item with a non-existent list_id
	_, err = svc.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId: "00000000-0000-0000-0000-000000000000",
		Title:  "Test Item",
	})

	require.Error(t, err, "should return error for non-existent list")

	st, ok := status.FromError(err)
	require.True(t, ok, "error should be a gRPC status error")

	// This assertion will FAIL with current implementation (returns Internal)
	// After fix, it should return NotFound
	require.Equal(t, codes.NotFound, st.Code(),
		"should return NotFound for missing list, not %v: %s", st.Code(), st.Message())
}

// TestCreateItem_ValidList verifies normal operation still works
func TestCreateItem_ValidList(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := sqlstorage.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	svc := service.NewMonoService(store, 50, 100)

	// First create a valid list
	listResp, err := svc.CreateList(ctx, &monov1.CreateListRequest{
		Title: "Test List",
	})
	require.NoError(t, err)

	// Now create item in that list - should succeed
	itemResp, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId: listResp.List.Id,
		Title:  "Test Item",
	})

	require.NoError(t, err)
	require.NotNil(t, itemResp)
	require.Equal(t, "Test Item", itemResp.Item.Title)
	require.NotEmpty(t, itemResp.Item.Id, "item should have an ID")
}
