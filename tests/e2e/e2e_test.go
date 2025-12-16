package e2e

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	monov1 "github.com/rezkam/mono/api/proto/monov1"
	"github.com/rezkam/mono/internal/service"
	"github.com/rezkam/mono/internal/storage/fs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	serverAddr string
)

func TestMain(m *testing.M) {
	// Setup Server
	tmpDir, err := os.MkdirTemp("", "mono-e2e-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := fs.NewStore(tmpDir)
	if err != nil {
		panic(err)
	}

	svc := service.NewMonoService(store)
	lis, err := net.Listen("tcp", "localhost:0") // Random port
	if err != nil {
		panic(err)
	}
	serverAddr = lis.Addr().String()

	s := grpc.NewServer()
	monov1.RegisterMonoServiceServer(s, svc)

	go func() {
		if err := s.Serve(lis); err != nil {
			panic(err)
		}
	}()

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	code := m.Run()

	s.Stop()
	os.Exit(code)
}

func TestE2E_CreateAndGetList_gRPC(t *testing.T) {
	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	client := monov1.NewMonoServiceClient(conn)
	ctx := context.Background()

	// 1. Create List
	createResp, err := client.CreateList(ctx, &monov1.CreateListRequest{Title: "E2E List"})
	require.NoError(t, err)
	assert.NotEmpty(t, createResp.List.Id)
	assert.Equal(t, "E2E List", createResp.List.Title)

	// 2. Add Item
	itemResp, err := client.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId: createResp.List.Id,
		Title:  "Buy Milk",
	})
	require.NoError(t, err)
	assert.Equal(t, "Buy Milk", itemResp.Item.Title)

	// 3. Create Item
	// Use explicit time for due date (e.g. 24h from now)
	dueTime := time.Now().Add(24 * time.Hour)
	itemResp, err = client.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId:  createResp.List.Id,
		Title:   "Buy Milk",
		DueTime: timestamppb.New(dueTime),
		Tags:    []string{"shopping", "urgent"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, itemResp.Item.Id)
	assert.Equal(t, "Buy Milk", itemResp.Item.Title)
	assert.Equal(t, []string{"shopping", "urgent"}, itemResp.Item.Tags)

	// 4. List Tasks (Filter)
	listTasksResp, err := client.ListTasks(ctx, &monov1.ListTasksRequest{
		Filter: "tags:urgent",
	})
	require.NoError(t, err)
	assert.Len(t, listTasksResp.Items, 1)
	assert.Equal(t, itemResp.Item.Id, listTasksResp.Items[0].Id)

	// 5. Update Item (Tags and Completed)
	updateResp, err := client.UpdateItem(ctx, &monov1.UpdateItemRequest{
		ListId: createResp.List.Id,
		Item: &monov1.TodoItem{
			Id:        itemResp.Item.Id,
			Completed: true,
			Tags:      []string{"shopping", "done"},
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"completed", "tags"}},
	})
	require.NoError(t, err)
	assert.True(t, updateResp.Item.Completed)
	assert.Equal(t, []string{"shopping", "done"}, updateResp.Item.Tags)

	// 6. Verify Update
	getResp2, err := client.GetList(ctx, &monov1.GetListRequest{Id: createResp.List.Id})
	require.NoError(t, err)
	assert.True(t, getResp2.List.Items[1].Completed) // Assuming the second item is the one updated
	assert.Equal(t, []string{"shopping", "done"}, getResp2.List.Items[1].Tags)
}

// Helper for HTTP tests if we were running the Gateway too.
// For now, testing gRPC core functionality covers the logic + storage E2E.
