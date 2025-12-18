package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/core"
	"github.com/rezkam/mono/internal/service"
	"github.com/rezkam/mono/internal/storage/sql/repository"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// MockStorage is a simple in-memory storage for unit testing.
type MockStorage struct {
	lists map[string]*core.TodoList
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		lists: make(map[string]*core.TodoList),
	}
}

func (m *MockStorage) CreateList(ctx context.Context, list *core.TodoList) error {
	m.lists[list.ID] = list
	return nil
}

func (m *MockStorage) GetList(ctx context.Context, id string) (*core.TodoList, error) {
	if l, ok := m.lists[id]; ok {
		return l, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *MockStorage) UpdateList(ctx context.Context, list *core.TodoList) error {
	m.lists[list.ID] = list
	return nil
}

func (m *MockStorage) CreateTodoItem(ctx context.Context, listID string, item core.TodoItem) error {
	if l, ok := m.lists[listID]; ok {
		l.Items = append(l.Items, item)
		return nil
	}
	return repository.ErrListNotFound
}

func (m *MockStorage) UpdateTodoItem(ctx context.Context, item core.TodoItem) error {
	// Find the list containing this item
	for _, l := range m.lists {
		for i, existingItem := range l.Items {
			if existingItem.ID == item.ID {
				l.Items[i] = item
				return nil
			}
		}
	}
	return fmt.Errorf("item not found")
}

func (m *MockStorage) ListLists(ctx context.Context) ([]*core.TodoList, error) {
	var results []*core.TodoList
	for _, l := range m.lists {
		results = append(results, l)
	}
	return results, nil
}

func (m *MockStorage) ListTasks(ctx context.Context, params core.ListTasksParams) (*core.ListTasksResult, error) {
	// Simple implementation for testing - gather all items
	var allItems []core.TodoItem
	for _, l := range m.lists {
		if params.ListID != nil && l.ID != *params.ListID {
			continue
		}
		for _, item := range l.Items {
			// Apply filters
			if params.Status != nil && item.Status != *params.Status {
				continue
			}
			if params.Priority != nil && item.Priority != nil && *item.Priority != *params.Priority {
				continue
			}
			if params.Tag != nil {
				// Check if item has the tag
				hasTag := false
				for _, tag := range item.Tags {
					if tag == *params.Tag {
						hasTag = true
						break
					}
				}
				if !hasTag {
					continue
				}
			}
			allItems = append(allItems, item)
		}
	}

	// Apply pagination
	start := params.Offset
	end := start + params.Limit
	if start >= len(allItems) {
		return &core.ListTasksResult{
			Items:      []core.TodoItem{},
			TotalCount: 0,
			HasMore:    false,
		}, nil
	}
	if end > len(allItems) {
		end = len(allItems)
	}

	return &core.ListTasksResult{
		Items:      allItems[start:end],
		TotalCount: end - start,
		HasMore:    end < len(allItems),
	}, nil
}

func (m *MockStorage) Close() error {
	return nil
}

func (m *MockStorage) CreateRecurringTemplate(ctx context.Context, template *core.RecurringTaskTemplate) error {
	if _, ok := m.lists[template.ListID]; !ok {
		return repository.ErrListNotFound
	}
	return nil
}

func (m *MockStorage) GetRecurringTemplate(ctx context.Context, id string) (*core.RecurringTaskTemplate, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *MockStorage) UpdateRecurringTemplate(ctx context.Context, template *core.RecurringTaskTemplate) error {
	return nil
}

func (m *MockStorage) DeleteRecurringTemplate(ctx context.Context, id string) error {
	return nil
}

func (m *MockStorage) ListRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*core.RecurringTaskTemplate, error) {
	return nil, nil
}

func (m *MockStorage) GetActiveTemplatesNeedingGeneration(ctx context.Context) ([]*core.RecurringTaskTemplate, error) {
	return nil, nil
}

func (m *MockStorage) UpdateRecurringTemplateGenerationWindow(ctx context.Context, templateID string, newGeneratedUntil time.Time) error {
	return nil
}

func (m *MockStorage) CreateGenerationJob(ctx context.Context, templateID string, scheduledFor time.Time, generateFrom, generateUntil time.Time) (string, error) {
	return "", nil
}

func (m *MockStorage) ClaimNextGenerationJob(ctx context.Context) (string, error) {
	return "", nil
}

func (m *MockStorage) GetGenerationJob(ctx context.Context, jobID string) (*core.GenerationJob, error) {
	return nil, fmt.Errorf("not found")
}

func (m *MockStorage) UpdateGenerationJobStatus(ctx context.Context, jobID string, status string, errorMessage *string) error {
	return nil
}

// TestCreateList tests the CreateList gRPC handler.
func TestCreateList(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		resp, err := svc.CreateList(ctx, &monov1.CreateListRequest{
			Title: "Test List",
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.List.Title != "Test List" {
			t.Errorf("expected title 'Test List', got %s", resp.List.Title)
		}

		if resp.List.Id == "" {
			t.Error("expected non-empty ID")
		}

		if len(resp.List.Items) != 0 {
			t.Errorf("expected 0 items, got %d", len(resp.List.Items))
		}
	})

	t.Run("EmptyTitle", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		_, err := svc.CreateList(ctx, &monov1.CreateListRequest{
			Title: "",
		})

		if err == nil {
			t.Fatal("expected error for empty title")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", st.Code())
		}
	})
}

// TestGetList tests the GetList gRPC handler.
func TestGetList(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		// Create a list first
		createResp, err := svc.CreateList(ctx, &monov1.CreateListRequest{
			Title: "Test List",
		})
		if err != nil {
			t.Fatalf("failed to create list: %v", err)
		}

		// Get the list
		resp, err := svc.GetList(ctx, &monov1.GetListRequest{
			Id: createResp.List.Id,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.List.Id != createResp.List.Id {
			t.Errorf("expected ID %s, got %s", createResp.List.Id, resp.List.Id)
		}

		if resp.List.Title != "Test List" {
			t.Errorf("expected title 'Test List', got %s", resp.List.Title)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		_, err := svc.GetList(ctx, &monov1.GetListRequest{
			Id: uuid.New().String(),
		})

		if err == nil {
			t.Fatal("expected error for non-existent list")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.Internal {
			t.Errorf("expected Internal error, got %v", st.Code())
		}
	})

	t.Run("EmptyID", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		_, err := svc.GetList(ctx, &monov1.GetListRequest{
			Id: "",
		})

		if err == nil {
			t.Fatal("expected error for empty ID")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", st.Code())
		}
	})

	t.Run("ListWithRecurringItems", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		// Create list and template
		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		templateResp, _ := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            listResp.List.Id,
			Title:             "Recurring Task",
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKLY,
		})

		// Create two items: one regular, one from template
		_, _ = svc.CreateItem(ctx, &monov1.CreateItemRequest{
			ListId: listResp.List.Id,
			Title:  "Regular Task",
		})

		instanceDate := time.Date(2025, 12, 17, 0, 0, 0, 0, time.UTC)
		_, _ = svc.CreateItem(ctx, &monov1.CreateItemRequest{
			ListId:              listResp.List.Id,
			Title:               "Generated from Template",
			RecurringTemplateId: templateResp.Template.Id,
			InstanceDate:        timestamppb.New(instanceDate),
		})

		// Get the list and verify recurring metadata is included
		resp, err := svc.GetList(ctx, &monov1.GetListRequest{Id: listResp.List.Id})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(resp.List.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(resp.List.Items))
		}

		// Find the recurring item
		var recurringItem *monov1.TodoItem
		for _, item := range resp.List.Items {
			if item.RecurringTemplateId != "" {
				recurringItem = item
				break
			}
		}

		if recurringItem == nil {
			t.Fatal("expected to find item with recurring metadata")
		}

		if recurringItem.RecurringTemplateId != templateResp.Template.Id {
			t.Errorf("expected template_id %s, got %s", templateResp.Template.Id, recurringItem.RecurringTemplateId)
		}

		if recurringItem.InstanceDate == nil {
			t.Error("expected instance_date to be set")
		} else if !recurringItem.InstanceDate.AsTime().Equal(instanceDate) {
			t.Errorf("expected instance_date %v, got %v", instanceDate, recurringItem.InstanceDate.AsTime())
		}
	})
}

// TestCreateItem tests the CreateItem gRPC handler.
func TestCreateItem(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		// Create a list first
		listResp, err := svc.CreateList(ctx, &monov1.CreateListRequest{
			Title: "Test List",
		})
		if err != nil {
			t.Fatalf("failed to create list: %v", err)
		}

		// Create an item
		resp, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
			ListId: listResp.List.Id,
			Title:  "Test Item",
			Tags:   []string{"tag1", "tag2"},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Item.Title != "Test Item" {
			t.Errorf("expected title 'Test Item', got %s", resp.Item.Title)
		}

		if resp.Item.Id == "" {
			t.Error("expected non-empty ID")
		}

		if resp.Item.Status != monov1.TaskStatus_TASK_STATUS_TODO {
			t.Errorf("expected TODO status, got %v", resp.Item.Status)
		}

		if len(resp.Item.Tags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(resp.Item.Tags))
		}
	})

	t.Run("WithPriorityAndDuration", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		listResp, err := svc.CreateList(ctx, &monov1.CreateListRequest{
			Title: "Test List",
		})
		if err != nil {
			t.Fatalf("failed to create list: %v", err)
		}

		dueTime := time.Now().Add(24 * time.Hour)
		estimatedDuration := 2 * time.Hour

		resp, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
			ListId:            listResp.List.Id,
			Title:             "Test Item",
			Priority:          monov1.TaskPriority_TASK_PRIORITY_HIGH,
			EstimatedDuration: durationpb.New(estimatedDuration),
			DueTime:           timestamppb.New(dueTime),
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Item.Priority != monov1.TaskPriority_TASK_PRIORITY_HIGH {
			t.Errorf("expected HIGH priority, got %v", resp.Item.Priority)
		}

		if resp.Item.EstimatedDuration.AsDuration() != estimatedDuration {
			t.Errorf("expected duration %v, got %v", estimatedDuration, resp.Item.EstimatedDuration.AsDuration())
		}

		if !resp.Item.DueTime.AsTime().Equal(dueTime) {
			t.Errorf("expected due time %v, got %v", dueTime, resp.Item.DueTime.AsTime())
		}
	})

	t.Run("EmptyListID", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		_, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
			ListId: "",
			Title:  "Test Item",
		})

		if err == nil {
			t.Fatal("expected error for empty list_id")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", st.Code())
		}
	})

	t.Run("EmptyTitle", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		_, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
			ListId: uuid.New().String(),
			Title:  "",
		})

		if err == nil {
			t.Fatal("expected error for empty title")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", st.Code())
		}
	})

	t.Run("ListNotFound", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		_, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
			ListId: uuid.New().String(),
			Title:  "Test Item",
		})

		if err == nil {
			t.Fatal("expected error for non-existent list")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		// This test expects NotFound for missing list (TDD - will fail until we fix the implementation)
		if st.Code() != codes.NotFound {
			t.Errorf("expected NotFound error, got %v: %s", st.Code(), st.Message())
		}

		// Verify error message mentions the list
		if !strings.Contains(st.Message(), "list not found") {
			t.Errorf("expected error message to contain 'list not found', got: %s", st.Message())
		}
	})

	t.Run("WithRecurringTemplateMetadata", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		// Create list and recurring template
		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		templateResp, _ := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            listResp.List.Id,
			Title:             "Daily Task",
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
		})

		// Create item linked to recurring template
		instanceDate := time.Date(2025, 12, 17, 0, 0, 0, 0, time.UTC)
		resp, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
			ListId:              listResp.List.Id,
			Title:               "Generated Task",
			RecurringTemplateId: templateResp.Template.Id,
			InstanceDate:        timestamppb.New(instanceDate),
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify recurring metadata is returned
		if resp.Item.RecurringTemplateId == "" {
			t.Error("expected recurring_template_id to be set")
		} else if resp.Item.RecurringTemplateId != templateResp.Template.Id {
			t.Errorf("expected template_id %s, got %s", templateResp.Template.Id, resp.Item.RecurringTemplateId)
		}

		if resp.Item.InstanceDate == nil {
			t.Error("expected instance_date to be set")
		} else if !resp.Item.InstanceDate.AsTime().Equal(instanceDate) {
			t.Errorf("expected instance_date %v, got %v", instanceDate, resp.Item.InstanceDate.AsTime())
		}
	})
}

// TestUpdateItem tests the UpdateItem gRPC handler.
func TestUpdateItem(t *testing.T) {
	t.Run("SuccessFullUpdate", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		// Create list and item
		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		itemResp, _ := svc.CreateItem(ctx, &monov1.CreateItemRequest{
			ListId: listResp.List.Id,
			Title:  "Original Title",
		})

		// Update the item (no field mask = update all)
		resp, err := svc.UpdateItem(ctx, &monov1.UpdateItemRequest{
			ListId: listResp.List.Id,
			Item: &monov1.TodoItem{
				Id:       itemResp.Item.Id,
				Title:    "Updated Title",
				Status:   monov1.TaskStatus_TASK_STATUS_IN_PROGRESS,
				Priority: monov1.TaskPriority_TASK_PRIORITY_URGENT,
				Tags:     []string{"updated"},
			},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Item.Title != "Updated Title" {
			t.Errorf("expected title 'Updated Title', got %s", resp.Item.Title)
		}

		if resp.Item.Status != monov1.TaskStatus_TASK_STATUS_IN_PROGRESS {
			t.Errorf("expected IN_PROGRESS status, got %v", resp.Item.Status)
		}

		if resp.Item.Priority != monov1.TaskPriority_TASK_PRIORITY_URGENT {
			t.Errorf("expected URGENT priority, got %v", resp.Item.Priority)
		}
	})

	t.Run("SuccessFieldMask", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		itemResp, _ := svc.CreateItem(ctx, &monov1.CreateItemRequest{
			ListId:   listResp.List.Id,
			Title:    "Original Title",
			Priority: monov1.TaskPriority_TASK_PRIORITY_LOW,
		})

		// Update only the title using field mask
		resp, err := svc.UpdateItem(ctx, &monov1.UpdateItemRequest{
			ListId: listResp.List.Id,
			Item: &monov1.TodoItem{
				Id:    itemResp.Item.Id,
				Title: "Updated Title Only",
			},
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"title"},
			},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Item.Title != "Updated Title Only" {
			t.Errorf("expected title 'Updated Title Only', got %s", resp.Item.Title)
		}

		// Priority should remain unchanged
		if resp.Item.Priority != monov1.TaskPriority_TASK_PRIORITY_LOW {
			t.Errorf("expected LOW priority (unchanged), got %v", resp.Item.Priority)
		}
	})

	t.Run("ItemNotFound", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})

		_, err := svc.UpdateItem(ctx, &monov1.UpdateItemRequest{
			ListId: listResp.List.Id,
			Item: &monov1.TodoItem{
				Id:    uuid.New().String(),
				Title: "Updated",
			},
		})

		if err == nil {
			t.Fatal("expected error for non-existent item")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.NotFound {
			t.Errorf("expected NotFound, got %v", st.Code())
		}
	})

	t.Run("PreservesStatusWhenUpdatingOtherFieldsWithoutMask", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		// Create list and item (starts as TODO by default)
		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		itemResp, _ := svc.CreateItem(ctx, &monov1.CreateItemRequest{
			ListId: listResp.List.Id,
			Title:  "Original Title",
		})

		// Update status to IN_PROGRESS
		_, _ = svc.UpdateItem(ctx, &monov1.UpdateItemRequest{
			ListId: listResp.List.Id,
			Item: &monov1.TodoItem{
				Id:     itemResp.Item.Id,
				Status: monov1.TaskStatus_TASK_STATUS_IN_PROGRESS,
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"status"}},
		})

		// Update only the title, without specifying update_mask or status
		// This simulates a client that wants to update the title only
		// and sends default/zero values for other fields (status=UNSPECIFIED)
		resp, err := svc.UpdateItem(ctx, &monov1.UpdateItemRequest{
			ListId: listResp.List.Id,
			Item: &monov1.TodoItem{
				Id:    itemResp.Item.Id,
				Title: "Updated Title",
				// Status is not set, defaults to TASK_STATUS_UNSPECIFIED (0)
			},
			// No update_mask provided
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Item.Title != "Updated Title" {
			t.Errorf("expected title 'Updated Title', got %s", resp.Item.Title)
		}

		// BUG: Status should remain IN_PROGRESS but gets reset to TODO
		// because UNSPECIFIED defaults to TODO in toCoreStatus
		if resp.Item.Status != monov1.TaskStatus_TASK_STATUS_IN_PROGRESS {
			t.Errorf("expected status to remain IN_PROGRESS, got %v (bug: status reset to TODO when UNSPECIFIED is passed)", resp.Item.Status)
		}
	})
}

// TestListLists tests the ListLists gRPC handler.
func TestListLists(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		// Create multiple lists
		svc.CreateList(ctx, &monov1.CreateListRequest{Title: "List 1"})
		svc.CreateList(ctx, &monov1.CreateListRequest{Title: "List 2"})
		svc.CreateList(ctx, &monov1.CreateListRequest{Title: "List 3"})

		resp, err := svc.ListLists(ctx, &monov1.ListListsRequest{})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(resp.Lists) != 3 {
			t.Errorf("expected 3 lists, got %d", len(resp.Lists))
		}
	})

	t.Run("EmptyLists", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		resp, err := svc.ListLists(ctx, &monov1.ListListsRequest{})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(resp.Lists) != 0 {
			t.Errorf("expected 0 lists, got %d", len(resp.Lists))
		}
	})
}

// TestListTasks tests the ListTasks gRPC handler.
func TestListTasks(t *testing.T) {
	t.Run("AllTasks", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		// Create list with items
		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		svc.CreateItem(ctx, &monov1.CreateItemRequest{ListId: listResp.List.Id, Title: "Item 1"})
		svc.CreateItem(ctx, &monov1.CreateItemRequest{ListId: listResp.List.Id, Title: "Item 2"})

		resp, err := svc.ListTasks(ctx, &monov1.ListTasksRequest{})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items, got %d", len(resp.Items))
		}
	})

	t.Run("FilterByTag", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		svc.CreateItem(ctx, &monov1.CreateItemRequest{ListId: listResp.List.Id, Title: "Item 1", Tags: []string{"urgent"}})
		svc.CreateItem(ctx, &monov1.CreateItemRequest{ListId: listResp.List.Id, Title: "Item 2", Tags: []string{"normal"}})
		svc.CreateItem(ctx, &monov1.CreateItemRequest{ListId: listResp.List.Id, Title: "Item 3", Tags: []string{"urgent"}})

		resp, err := svc.ListTasks(ctx, &monov1.ListTasksRequest{
			Filter: "tags:urgent",
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items with 'urgent' tag, got %d", len(resp.Items))
		}
	})

	t.Run("FilterByParent", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		list1, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "List 1"})
		list2, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "List 2"})

		svc.CreateItem(ctx, &monov1.CreateItemRequest{ListId: list1.List.Id, Title: "Item 1"})
		svc.CreateItem(ctx, &monov1.CreateItemRequest{ListId: list2.List.Id, Title: "Item 2"})
		svc.CreateItem(ctx, &monov1.CreateItemRequest{ListId: list1.List.Id, Title: "Item 3"})

		resp, err := svc.ListTasks(ctx, &monov1.ListTasksRequest{
			Parent: list1.List.Id,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items in list1, got %d", len(resp.Items))
		}
	})

	t.Run("OrderByValidation_ValidFields", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		svc.CreateItem(ctx, &monov1.CreateItemRequest{ListId: listResp.List.Id, Title: "Item 1"})

		// Test all valid order_by fields
		validFields := []string{"due_time", "priority", "created_at", "updated_at"}
		for _, field := range validFields {
			resp, err := svc.ListTasks(ctx, &monov1.ListTasksRequest{
				OrderBy: field,
			})

			if err != nil {
				t.Errorf("unexpected error for valid field %q: %v", field, err)
			}

			if resp == nil {
				t.Errorf("expected response for valid field %q", field)
			}
		}
	})

	t.Run("OrderByValidation_InvalidField", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		svc.CreateItem(ctx, &monov1.CreateItemRequest{ListId: listResp.List.Id, Title: "Item 1"})

		// Test invalid order_by field
		_, err := svc.ListTasks(ctx, &monov1.ListTasksRequest{
			OrderBy: "invalid_field",
		})

		if err == nil {
			t.Fatal("expected error for invalid order_by field")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", st.Code())
		}

		// Verify error message mentions the invalid field
		if !strings.Contains(st.Message(), "invalid_field") {
			t.Errorf("expected error message to contain 'invalid_field', got: %s", st.Message())
		}

		// Verify error message lists valid fields
		expectedFields := []string{"due_time", "priority", "created_at", "updated_at"}
		for _, field := range expectedFields {
			if !strings.Contains(st.Message(), field) {
				t.Errorf("expected error message to mention valid field %q, got: %s", field, st.Message())
			}
		}
	})

	t.Run("OrderByValidation_SQLInjectionAttempt", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		svc.CreateItem(ctx, &monov1.CreateItemRequest{ListId: listResp.List.Id, Title: "Item 1"})

		// Test SQL injection attempt (should be rejected by validation for UX, not security)
		maliciousInputs := []string{
			"id; DROP TABLE todo_items--",
			"id; DELETE FROM todo_items--",
			"created_at DESC; UPDATE--",
		}

		for _, input := range maliciousInputs {
			_, err := svc.ListTasks(ctx, &monov1.ListTasksRequest{
				OrderBy: input,
			})

			if err == nil {
				t.Errorf("expected error for malicious input %q", input)
				continue
			}

			st, ok := status.FromError(err)
			if !ok {
				t.Errorf("expected gRPC status error for input %q", input)
				continue
			}

			if st.Code() != codes.InvalidArgument {
				t.Errorf("expected InvalidArgument for input %q, got %v", input, st.Code())
			}
		}
	})
}

// TestCreateRecurringTemplate tests the CreateRecurringTemplate gRPC handler.
func TestCreateRecurringTemplate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		// Create a list first
		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})

		resp, err := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:               listResp.List.Id,
			Title:                "Daily Standup",
			RecurrencePattern:    monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
			GenerationWindowDays: 30,
			Tags:                 []string{"meeting"},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Template.Title != "Daily Standup" {
			t.Errorf("expected title 'Daily Standup', got %s", resp.Template.Title)
		}

		if resp.Template.Id == "" {
			t.Error("expected non-empty ID")
		}

		if !resp.Template.IsActive {
			t.Error("expected template to be active")
		}

		if resp.Template.GenerationWindowDays != 30 {
			t.Errorf("expected generation window 30, got %d", resp.Template.GenerationWindowDays)
		}
	})

	t.Run("WithPriorityAndDuration", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})

		estimatedDuration := 1 * time.Hour
		dueOffset := 2 * time.Hour

		resp, err := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            listResp.List.Id,
			Title:             "Weekly Review",
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKLY,
			Priority:          monov1.TaskPriority_TASK_PRIORITY_HIGH,
			EstimatedDuration: durationpb.New(estimatedDuration),
			DueOffset:         durationpb.New(dueOffset),
			RecurrenceConfig:  `{"day_of_week": "monday"}`,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Template.Priority != monov1.TaskPriority_TASK_PRIORITY_HIGH {
			t.Errorf("expected HIGH priority, got %v", resp.Template.Priority)
		}

		if resp.Template.EstimatedDuration.AsDuration() != estimatedDuration {
			t.Errorf("expected duration %v, got %v", estimatedDuration, resp.Template.EstimatedDuration.AsDuration())
		}
	})

	t.Run("EmptyListID", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		_, err := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            "",
			Title:             "Test Template",
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
		})

		if err == nil {
			t.Fatal("expected error for empty list_id")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", st.Code())
		}
	})

	t.Run("EmptyTitle", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		_, err := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            uuid.New().String(),
			Title:             "",
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
		})

		if err == nil {
			t.Fatal("expected error for empty title")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", st.Code())
		}
	})

	t.Run("ListNotFound", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		_, err := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            uuid.New().String(),
			Title:             "Test Template",
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
		})

		if err == nil {
			t.Fatal("expected error for non-existent list")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.NotFound {
			t.Errorf("expected NotFound, got %v", st.Code())
		}
	})

	t.Run("InvalidRecurrenceConfig", func(t *testing.T) {
		storage := NewMockStorage()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})

		_, err := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            listResp.List.Id,
			Title:             "Test Template",
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKLY,
			RecurrenceConfig:  `{invalid json}`,
		})

		if err == nil {
			t.Fatal("expected error for invalid recurrence config JSON")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", st.Code())
		}
	})
}

// TestGetRecurringTemplate tests the GetRecurringTemplate gRPC handler.
func TestGetRecurringTemplate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		// Create a list and template first
		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		createResp, _ := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            listResp.List.Id,
			Title:             "Daily Task",
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
		})

		resp, err := svc.GetRecurringTemplate(ctx, &monov1.GetRecurringTemplateRequest{
			Id: createResp.Template.Id,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Template.Id != createResp.Template.Id {
			t.Errorf("expected ID %s, got %s", createResp.Template.Id, resp.Template.Id)
		}

		if resp.Template.Title != "Daily Task" {
			t.Errorf("expected title 'Daily Task', got %s", resp.Template.Title)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		_, err := svc.GetRecurringTemplate(ctx, &monov1.GetRecurringTemplateRequest{
			Id: uuid.New().String(),
		})

		if err == nil {
			t.Fatal("expected error for non-existent template")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.NotFound {
			t.Errorf("expected NotFound, got %v", st.Code())
		}
	})

	t.Run("EmptyID", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		_, err := svc.GetRecurringTemplate(ctx, &monov1.GetRecurringTemplateRequest{
			Id: "",
		})

		if err == nil {
			t.Fatal("expected error for empty ID")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", st.Code())
		}
	})
}

// TestUpdateRecurringTemplate tests the UpdateRecurringTemplate gRPC handler.
func TestUpdateRecurringTemplate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		// Create list and template
		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		createResp, _ := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            listResp.List.Id,
			Title:             "Original Title",
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
		})

		// Update the template
		resp, err := svc.UpdateRecurringTemplate(ctx, &monov1.UpdateRecurringTemplateRequest{
			Template: &monov1.RecurringTaskTemplate{
				Id:                createResp.Template.Id,
				ListId:            listResp.List.Id,
				Title:             "Updated Title",
				RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKLY,
				Priority:          monov1.TaskPriority_TASK_PRIORITY_URGENT,
			},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Template.Title != "Updated Title" {
			t.Errorf("expected title 'Updated Title', got %s", resp.Template.Title)
		}

		if resp.Template.RecurrencePattern != monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKLY {
			t.Errorf("expected WEEKLY pattern, got %v", resp.Template.RecurrencePattern)
		}

		if resp.Template.Priority != monov1.TaskPriority_TASK_PRIORITY_URGENT {
			t.Errorf("expected URGENT priority, got %v", resp.Template.Priority)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		_, err := svc.UpdateRecurringTemplate(ctx, &monov1.UpdateRecurringTemplateRequest{
			Template: &monov1.RecurringTaskTemplate{
				Id:    uuid.New().String(),
				Title: "Updated",
			},
		})

		if err == nil {
			t.Fatal("expected error for non-existent template")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.NotFound {
			t.Errorf("expected NotFound, got %v", st.Code())
		}
	})

	t.Run("InvalidRecurrenceConfig", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		createResp, _ := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            listResp.List.Id,
			Title:             "Test",
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
		})

		_, err := svc.UpdateRecurringTemplate(ctx, &monov1.UpdateRecurringTemplateRequest{
			Template: &monov1.RecurringTaskTemplate{
				Id:               createResp.Template.Id,
				Title:            "Updated",
				RecurrenceConfig: `{bad json`,
			},
		})

		if err == nil {
			t.Fatal("expected error for invalid recurrence config")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", st.Code())
		}
	})

	t.Run("PartialUpdate_WithFieldMask_OnlyUpdatesSpecifiedFields", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		// Create a template with all fields populated
		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		createResp, _ := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            listResp.List.Id,
			Title:             "Original Title",
			Tags:              []string{"tag1", "tag2"},
			Priority:          monov1.TaskPriority_TASK_PRIORITY_HIGH,
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
			EstimatedDuration: durationpb.New(30 * time.Minute),
		})

		// Partial update: only update tags using field mask
		resp, err := svc.UpdateRecurringTemplate(ctx, &monov1.UpdateRecurringTemplateRequest{
			Template: &monov1.RecurringTaskTemplate{
				Id:   createResp.Template.Id,
				Tags: []string{"new-tag"},
			},
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"tags"},
			},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Tags should be updated
		if len(resp.Template.Tags) != 1 || resp.Template.Tags[0] != "new-tag" {
			t.Errorf("expected tags [new-tag], got %v", resp.Template.Tags)
		}

		// Other fields should remain unchanged
		if resp.Template.Title != "Original Title" {
			t.Errorf("expected title to remain 'Original Title', got %s", resp.Template.Title)
		}

		if resp.Template.Priority != monov1.TaskPriority_TASK_PRIORITY_HIGH {
			t.Errorf("expected priority to remain HIGH, got %v", resp.Template.Priority)
		}

		if resp.Template.RecurrencePattern != monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY {
			t.Errorf("expected pattern to remain DAILY, got %v", resp.Template.RecurrencePattern)
		}

		if resp.Template.EstimatedDuration.AsDuration() != 30*time.Minute {
			t.Errorf("expected duration to remain 30m, got %v", resp.Template.EstimatedDuration.AsDuration())
		}
	})

	t.Run("PartialUpdate_WithFieldMask_MultipleFields", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		createResp, _ := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            listResp.List.Id,
			Title:             "Original Title",
			Tags:              []string{"old"},
			Priority:          monov1.TaskPriority_TASK_PRIORITY_LOW,
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
		})

		// Update title and priority only
		resp, err := svc.UpdateRecurringTemplate(ctx, &monov1.UpdateRecurringTemplateRequest{
			Template: &monov1.RecurringTaskTemplate{
				Id:       createResp.Template.Id,
				Title:    "New Title",
				Priority: monov1.TaskPriority_TASK_PRIORITY_URGENT,
			},
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"title", "priority"},
			},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Updated fields
		if resp.Template.Title != "New Title" {
			t.Errorf("expected title 'New Title', got %s", resp.Template.Title)
		}

		if resp.Template.Priority != monov1.TaskPriority_TASK_PRIORITY_URGENT {
			t.Errorf("expected priority URGENT, got %v", resp.Template.Priority)
		}

		// Unchanged fields
		if len(resp.Template.Tags) != 1 || resp.Template.Tags[0] != "old" {
			t.Errorf("expected tags [old] to remain, got %v", resp.Template.Tags)
		}

		if resp.Template.RecurrencePattern != monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY {
			t.Errorf("expected pattern DAILY to remain, got %v", resp.Template.RecurrencePattern)
		}
	})

	t.Run("PartialUpdate_EmptyFieldMask_UpdatesAllFields", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		createResp, _ := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            listResp.List.Id,
			Title:             "Original",
			Priority:          monov1.TaskPriority_TASK_PRIORITY_LOW,
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
		})

		// No field mask = update all fields
		resp, err := svc.UpdateRecurringTemplate(ctx, &monov1.UpdateRecurringTemplateRequest{
			Template: &monov1.RecurringTaskTemplate{
				Id:                createResp.Template.Id,
				ListId:            listResp.List.Id,
				Title:             "Updated",
				Priority:          monov1.TaskPriority_TASK_PRIORITY_URGENT,
				RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKLY,
			},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Template.Title != "Updated" {
			t.Errorf("expected title 'Updated', got %s", resp.Template.Title)
		}

		if resp.Template.Priority != monov1.TaskPriority_TASK_PRIORITY_URGENT {
			t.Errorf("expected priority URGENT, got %v", resp.Template.Priority)
		}

		if resp.Template.RecurrencePattern != monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKLY {
			t.Errorf("expected pattern WEEKLY, got %v", resp.Template.RecurrencePattern)
		}
	})
}

// TestDeleteRecurringTemplate tests the DeleteRecurringTemplate gRPC handler.
func TestDeleteRecurringTemplate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		// Create list and template
		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		createResp, _ := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            listResp.List.Id,
			Title:             "To Delete",
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
		})

		// Delete the template
		_, err := svc.DeleteRecurringTemplate(ctx, &monov1.DeleteRecurringTemplateRequest{
			Id: createResp.Template.Id,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify it's deleted
		_, err = svc.GetRecurringTemplate(ctx, &monov1.GetRecurringTemplateRequest{
			Id: createResp.Template.Id,
		})

		if err == nil {
			t.Fatal("expected error when getting deleted template")
		}
	})

	t.Run("EmptyID", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		_, err := svc.DeleteRecurringTemplate(ctx, &monov1.DeleteRecurringTemplateRequest{
			Id: "",
		})

		if err == nil {
			t.Fatal("expected error for empty ID")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", st.Code())
		}
	})
}

// TestListRecurringTemplates tests the ListRecurringTemplates gRPC handler.
func TestListRecurringTemplates(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		// Create list and templates
		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            listResp.List.Id,
			Title:             "Template 1",
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
		})
		svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            listResp.List.Id,
			Title:             "Template 2",
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKLY,
		})

		resp, err := svc.ListRecurringTemplates(ctx, &monov1.ListRecurringTemplatesRequest{
			ListId: listResp.List.Id,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(resp.Templates) != 2 {
			t.Errorf("expected 2 templates, got %d", len(resp.Templates))
		}
	})

	t.Run("EmptyListID", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		_, err := svc.ListRecurringTemplates(ctx, &monov1.ListRecurringTemplatesRequest{
			ListId: "",
		})

		if err == nil {
			t.Fatal("expected error for empty list_id")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Fatal("expected gRPC status error")
		}

		if st.Code() != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", st.Code())
		}
	})

	t.Run("ActiveOnlyFilter", func(t *testing.T) {
		storage := NewMockStorageWithTemplates()
		svc := service.NewMonoService(storage, 50, 100)
		ctx := context.Background()

		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{Title: "Test List"})
		svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
			ListId:            listResp.List.Id,
			Title:             "Active",
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
		})

		// Mock storage should handle active_only filtering
		resp, err := svc.ListRecurringTemplates(ctx, &monov1.ListRecurringTemplatesRequest{
			ListId:     listResp.List.Id,
			ActiveOnly: true,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify all returned templates are active
		for _, tmpl := range resp.Templates {
			if !tmpl.IsActive {
				t.Error("expected only active templates")
			}
		}
	})
}

// NewMockStorageWithTemplates extends MockStorage with proper template support.
func NewMockStorageWithTemplates() *MockStorageWithTemplates {
	return &MockStorageWithTemplates{
		lists:     make(map[string]*core.TodoList),
		templates: make(map[string]*core.RecurringTaskTemplate),
	}
}

// MockStorageWithTemplates extends MockStorage with full recurring template support.
type MockStorageWithTemplates struct {
	lists     map[string]*core.TodoList
	templates map[string]*core.RecurringTaskTemplate
}

func (m *MockStorageWithTemplates) CreateList(ctx context.Context, list *core.TodoList) error {
	m.lists[list.ID] = list
	return nil
}

func (m *MockStorageWithTemplates) GetList(ctx context.Context, id string) (*core.TodoList, error) {
	if l, ok := m.lists[id]; ok {
		return l, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *MockStorageWithTemplates) UpdateList(ctx context.Context, list *core.TodoList) error {
	m.lists[list.ID] = list
	return nil
}

func (m *MockStorageWithTemplates) CreateTodoItem(ctx context.Context, listID string, item core.TodoItem) error {
	if l, ok := m.lists[listID]; ok {
		l.Items = append(l.Items, item)
		return nil
	}
	return repository.ErrListNotFound
}

func (m *MockStorageWithTemplates) UpdateTodoItem(ctx context.Context, item core.TodoItem) error {
	// Find the list containing this item
	for _, l := range m.lists {
		for i, existingItem := range l.Items {
			if existingItem.ID == item.ID {
				l.Items[i] = item
				return nil
			}
		}
	}
	return fmt.Errorf("item not found")
}

func (m *MockStorageWithTemplates) ListLists(ctx context.Context) ([]*core.TodoList, error) {
	var results []*core.TodoList
	for _, l := range m.lists {
		results = append(results, l)
	}
	return results, nil
}

func (m *MockStorageWithTemplates) ListTasks(ctx context.Context, params core.ListTasksParams) (*core.ListTasksResult, error) {
	// Simple implementation for testing - gather all items
	var allItems []core.TodoItem
	for _, l := range m.lists {
		if params.ListID != nil && l.ID != *params.ListID {
			continue
		}
		for _, item := range l.Items {
			// Apply filters
			if params.Status != nil && item.Status != *params.Status {
				continue
			}
			if params.Priority != nil && item.Priority != nil && *item.Priority != *params.Priority {
				continue
			}
			if params.Tag != nil {
				// Check if item has the tag
				hasTag := false
				for _, tag := range item.Tags {
					if tag == *params.Tag {
						hasTag = true
						break
					}
				}
				if !hasTag {
					continue
				}
			}
			allItems = append(allItems, item)
		}
	}

	// Apply pagination
	start := params.Offset
	end := start + params.Limit
	if start >= len(allItems) {
		return &core.ListTasksResult{
			Items:      []core.TodoItem{},
			TotalCount: 0,
			HasMore:    false,
		}, nil
	}
	if end > len(allItems) {
		end = len(allItems)
	}

	return &core.ListTasksResult{
		Items:      allItems[start:end],
		TotalCount: end - start,
		HasMore:    end < len(allItems),
	}, nil
}

func (m *MockStorageWithTemplates) CreateRecurringTemplate(ctx context.Context, template *core.RecurringTaskTemplate) error {
	if _, ok := m.lists[template.ListID]; !ok {
		return repository.ErrListNotFound
	}
	m.templates[template.ID] = template
	return nil
}

func (m *MockStorageWithTemplates) GetRecurringTemplate(ctx context.Context, id string) (*core.RecurringTaskTemplate, error) {
	if tmpl, ok := m.templates[id]; ok {
		return tmpl, nil
	}
	return nil, fmt.Errorf("%w: template %s", repository.ErrNotFound, id)
}

func (m *MockStorageWithTemplates) UpdateRecurringTemplate(ctx context.Context, template *core.RecurringTaskTemplate) error {
	if _, ok := m.templates[template.ID]; !ok {
		return fmt.Errorf("%w: template %s", repository.ErrNotFound, template.ID)
	}
	m.templates[template.ID] = template
	return nil
}

func (m *MockStorageWithTemplates) DeleteRecurringTemplate(ctx context.Context, id string) error {
	delete(m.templates, id)
	return nil
}

func (m *MockStorageWithTemplates) ListRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*core.RecurringTaskTemplate, error) {
	var results []*core.RecurringTaskTemplate
	for _, tmpl := range m.templates {
		if tmpl.ListID == listID {
			if !activeOnly || tmpl.IsActive {
				results = append(results, tmpl)
			}
		}
	}
	return results, nil
}

func (m *MockStorageWithTemplates) GetActiveTemplatesNeedingGeneration(ctx context.Context) ([]*core.RecurringTaskTemplate, error) {
	return nil, nil
}

func (m *MockStorageWithTemplates) UpdateRecurringTemplateGenerationWindow(ctx context.Context, templateID string, newGeneratedUntil time.Time) error {
	return nil
}

func (m *MockStorageWithTemplates) CreateGenerationJob(ctx context.Context, templateID string, scheduledFor time.Time, generateFrom, generateUntil time.Time) (string, error) {
	return "", nil
}

func (m *MockStorageWithTemplates) ClaimNextGenerationJob(ctx context.Context) (string, error) {
	return "", nil
}

func (m *MockStorageWithTemplates) GetGenerationJob(ctx context.Context, jobID string) (*core.GenerationJob, error) {
	return nil, fmt.Errorf("not found")
}

func (m *MockStorageWithTemplates) UpdateGenerationJobStatus(ctx context.Context, jobID string, status string, errorMessage *string) error {
	return nil
}

func (m *MockStorageWithTemplates) Close() error {
	return nil
}
