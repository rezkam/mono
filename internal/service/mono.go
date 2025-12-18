package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/core"
	"github.com/rezkam/mono/internal/storage/sql/repository"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type MonoService struct {
	monov1.UnimplementedMonoServiceServer
	storage         core.Storage
	defaultPageSize int
	maxPageSize     int
}

func NewMonoService(storage core.Storage, defaultPageSize, maxPageSize int) *MonoService {
	return &MonoService{
		storage:         storage,
		defaultPageSize: defaultPageSize,
		maxPageSize:     maxPageSize,
	}
}

func (s *MonoService) CreateList(ctx context.Context, req *monov1.CreateListRequest) (*monov1.CreateListResponse, error) {
	if req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "title is required")
	}

	idObj, err := uuid.NewV7()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate id: %v", err)
	}
	id := idObj.String()
	list := &core.TodoList{
		ID:         id,
		Title:      req.Title,
		Items:      []core.TodoItem{},
		CreateTime: time.Now().UTC(),
	}

	if err := s.storage.CreateList(ctx, list); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create list: %v", err)
	}

	return &monov1.CreateListResponse{
		List: toProtoList(list),
	}, nil
}

func (s *MonoService) GetList(ctx context.Context, req *monov1.GetListRequest) (*monov1.GetListResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	list, err := s.storage.GetList(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get list: %v", err)
	}

	return &monov1.GetListResponse{
		List: toProtoList(list),
	}, nil
}

func (s *MonoService) CreateItem(ctx context.Context, req *monov1.CreateItemRequest) (*monov1.CreateItemResponse, error) {
	if req.ListId == "" {
		return nil, status.Error(codes.InvalidArgument, "list_id is required")
	}
	if req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "title is required")
	}

	itemIDObj, err := uuid.NewV7()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate id: %v", err)
	}
	itemID := itemIDObj.String()
	item := core.TodoItem{
		ID:         itemID,
		Title:      req.Title,
		Status:     core.TaskStatusTodo,
		CreateTime: time.Now().UTC(),
		Tags:       req.Tags,
	}
	item.UpdatedAt = item.CreateTime

	if req.Priority != monov1.TaskPriority_TASK_PRIORITY_UNSPECIFIED {
		p := toCorePriority(req.Priority)
		item.Priority = &p
	}

	if req.EstimatedDuration != nil {
		d := req.EstimatedDuration.AsDuration()
		item.EstimatedDuration = &d
	}

	if req.DueTime != nil {
		t := req.DueTime.AsTime()
		item.DueTime = &t
	}

	// Handle timezone for due_time
	if req.Timezone != "" {
		// Fixed timezone - validate IANA timezone
		_, err := time.LoadLocation(req.Timezone)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid timezone: %s", req.Timezone)
		}
		item.Timezone = &req.Timezone
	}
	// If req.Timezone is empty, item.Timezone stays nil (floating time)

	if req.RecurringTemplateId != "" {
		item.RecurringTemplateID = &req.RecurringTemplateId
	}

	if req.InstanceDate != nil {
		t := req.InstanceDate.AsTime()
		item.InstanceDate = &t
	}

	// Use the new CreateTodoItem method to preserve status history
	if err := s.storage.CreateTodoItem(ctx, req.ListId, item); err != nil {
		if errors.Is(err, repository.ErrListNotFound) {
			return nil, status.Error(codes.NotFound, "list not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to create item: %v", err)
	}

	return &monov1.CreateItemResponse{
		Item: toProtoItem(item),
	}, nil
}

func (s *MonoService) UpdateItem(ctx context.Context, req *monov1.UpdateItemRequest) (*monov1.UpdateItemResponse, error) {
	// The protovalidate logic is ideally handled by an interceptor, but we assume
	// valid requests here or rely on the fact that we can check basic things.

	list, err := s.storage.GetList(ctx, req.ListId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "list not found: %v", err)
	}

	// Find the item
	var itemIndex = -1
	for i, item := range list.Items {
		if item.ID == req.Item.Id {
			itemIndex = i
			break
		}
	}
	if itemIndex == -1 {
		return nil, status.Errorf(codes.NotFound, "item not found")
	}

	// Apply FieldMask
	// If mask is empty, we might default to full update or no-op.
	// AIP-134 says empty mask = replace all fields (for HTTP PUT) or specific behavior.
	// For PATCH, it usually means update all provided fields.
	// But let's be strict: use the mask.

	if req.UpdateMask == nil || len(req.UpdateMask.Paths) == 0 {
		// Update all available mutable fields
		list.Items[itemIndex].Title = req.Item.Title

		// Only update status if explicitly provided (not UNSPECIFIED)
		// to avoid silently resetting status when clients update other fields
		if req.Item.Status != monov1.TaskStatus_TASK_STATUS_UNSPECIFIED {
			list.Items[itemIndex].Status = toCoreStatus(req.Item.Status)
		}

		if req.Item.Priority != monov1.TaskPriority_TASK_PRIORITY_UNSPECIFIED {
			p := toCorePriority(req.Item.Priority)
			list.Items[itemIndex].Priority = &p
		} else {
			list.Items[itemIndex].Priority = nil
		}

		list.Items[itemIndex].Tags = req.Item.Tags

		if req.Item.EstimatedDuration != nil {
			d := req.Item.EstimatedDuration.AsDuration()
			list.Items[itemIndex].EstimatedDuration = &d
		} else {
			list.Items[itemIndex].EstimatedDuration = nil
		}

		if req.Item.ActualDuration != nil {
			d := req.Item.ActualDuration.AsDuration()
			list.Items[itemIndex].ActualDuration = &d
		} else {
			list.Items[itemIndex].ActualDuration = nil
		}

		if req.Item.DueTime != nil {
			t := req.Item.DueTime.AsTime()
			list.Items[itemIndex].DueTime = &t
		} else {
			list.Items[itemIndex].DueTime = nil
		}

		// Handle timezone
		if req.Item.Timezone != "" {
			// Validate IANA timezone
			_, err := time.LoadLocation(req.Item.Timezone)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid timezone: %s", req.Item.Timezone)
			}
			tz := req.Item.Timezone
			list.Items[itemIndex].Timezone = &tz
		} else {
			list.Items[itemIndex].Timezone = nil
		}

		list.Items[itemIndex].UpdatedAt = time.Now()
	} else {
		for _, path := range req.UpdateMask.Paths {
			switch path {
			case "title":
				list.Items[itemIndex].Title = req.Item.Title
			case "status":
				list.Items[itemIndex].Status = toCoreStatus(req.Item.Status)
			case "priority":
				if req.Item.Priority != monov1.TaskPriority_TASK_PRIORITY_UNSPECIFIED {
					p := toCorePriority(req.Item.Priority)
					list.Items[itemIndex].Priority = &p
				} else {
					list.Items[itemIndex].Priority = nil
				}
			case "tags":
				list.Items[itemIndex].Tags = req.Item.Tags
			case "due_time":
				if req.Item.DueTime != nil {
					t := req.Item.DueTime.AsTime()
					list.Items[itemIndex].DueTime = &t
				} else {
					list.Items[itemIndex].DueTime = nil
				}
			case "estimated_duration":
				if req.Item.EstimatedDuration != nil {
					d := req.Item.EstimatedDuration.AsDuration()
					list.Items[itemIndex].EstimatedDuration = &d
				} else {
					list.Items[itemIndex].EstimatedDuration = nil
				}
			case "actual_duration":
				if req.Item.ActualDuration != nil {
					d := req.Item.ActualDuration.AsDuration()
					list.Items[itemIndex].ActualDuration = &d
				} else {
					list.Items[itemIndex].ActualDuration = nil
				}
			case "timezone":
				if req.Item.Timezone != "" {
					// Validate IANA timezone
					_, err := time.LoadLocation(req.Item.Timezone)
					if err != nil {
						return nil, status.Errorf(codes.InvalidArgument, "invalid timezone: %s", req.Item.Timezone)
					}
					tz := req.Item.Timezone
					list.Items[itemIndex].Timezone = &tz
				} else {
					list.Items[itemIndex].Timezone = nil
				}
			}
		}
		list.Items[itemIndex].UpdatedAt = time.Now()
	}

	// Use the new UpdateTodoItem method to preserve status history
	if err := s.storage.UpdateTodoItem(ctx, list.Items[itemIndex]); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update item: %v", err)
	}

	return &monov1.UpdateItemResponse{Item: toProtoItem(list.Items[itemIndex])}, nil
}

func (s *MonoService) ListLists(ctx context.Context, req *monov1.ListListsRequest) (*monov1.ListListsResponse, error) {
	lists, err := s.storage.ListLists(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list lists: %v", err)
	}

	protoLists := make([]*monov1.TodoList, len(lists))
	for i, list := range lists {
		protoLists[i] = toProtoList(list)
	}

	return &monov1.ListListsResponse{
		Lists: protoLists,
	}, nil
}

func (s *MonoService) ListTasks(ctx context.Context, req *monov1.ListTasksRequest) (*monov1.ListTasksResponse, error) {
	// Parse pagination parameters
	pageSize := s.defaultPageSize
	if req.PageSize > 0 {
		pageSize = int(req.PageSize)
		if pageSize > s.maxPageSize {
			pageSize = s.maxPageSize
		}
	}

	// Decode page token to get offset
	offset := 0
	if req.PageToken != "" {
		var err error
		offset, err = decodePageToken(req.PageToken)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
		}
	}

	// Build storage params
	params := core.ListTasksParams{
		Limit:  pageSize,
		Offset: offset,
	}

	// Optional filters
	if req.Parent != "" {
		params.ListID = &req.Parent
	}

	// Parse filter string (simple implementation for now)
	// Expected format: "status:TODO" or "priority:HIGH" or "tags:urgent"
	if req.Filter != "" {
		parts := strings.Split(req.Filter, ":")
		if len(parts) == 2 {
			field := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch field {
			case "status":
				status := core.TaskStatus(value)
				params.Status = &status
			case "priority":
				priority := core.TaskPriority(value)
				params.Priority = &priority
			case "tags":
				params.Tag = &value
			}
		}
	}

	// Set ordering (default: created_at)
	if req.OrderBy != "" {
		params.OrderBy = req.OrderBy
	}

	// Execute query
	result, err := s.storage.ListTasks(ctx, params)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list tasks: %v", err)
	}

	// Convert to proto items
	protoItems := make([]*monov1.TodoItem, len(result.Items))
	for i, item := range result.Items {
		protoItems[i] = toProtoItem(item)
	}

	// Generate next page token if there are more results
	var nextPageToken string
	if result.HasMore {
		nextPageToken = encodePageToken(offset + pageSize)
	}

	return &monov1.ListTasksResponse{
		Items:         protoItems,
		NextPageToken: nextPageToken,
	}, nil
}

func toProtoList(l *core.TodoList) *monov1.TodoList {
	items := make([]*monov1.TodoItem, len(l.Items))
	for i, item := range l.Items {
		items[i] = toProtoItem(item)
	}
	return &monov1.TodoList{
		Id:          l.ID,
		Title:       l.Title,
		Items:       items,
		CreateTime:  timestamppb.New(l.CreateTime),
		TotalItems:  int32(l.TotalItems),
		UndoneItems: int32(l.UndoneItems),
	}
}

func toProtoItem(i core.TodoItem) *monov1.TodoItem {
	item := &monov1.TodoItem{
		Id:         i.ID,
		Title:      i.Title,
		Status:     toProtoStatus(i.Status),
		CreateTime: timestamppb.New(i.CreateTime),
		UpdatedAt:  timestamppb.New(i.UpdatedAt),
		Tags:       i.Tags,
	}

	if i.Priority != nil {
		item.Priority = toProtoPriority(*i.Priority)
	}

	if i.DueTime != nil {
		item.DueTime = timestamppb.New(*i.DueTime)
	}

	if i.EstimatedDuration != nil {
		item.EstimatedDuration = durationpb.New(*i.EstimatedDuration)
	}

	if i.ActualDuration != nil {
		item.ActualDuration = durationpb.New(*i.ActualDuration)
	}

	if i.RecurringTemplateID != nil {
		item.RecurringTemplateId = *i.RecurringTemplateID
	}

	if i.InstanceDate != nil {
		item.InstanceDate = timestamppb.New(*i.InstanceDate)
	}

	if i.Timezone != nil {
		item.Timezone = *i.Timezone
	}

	return item
}

func toCoreStatus(s monov1.TaskStatus) core.TaskStatus {
	switch s {
	case monov1.TaskStatus_TASK_STATUS_TODO:
		return core.TaskStatusTodo
	case monov1.TaskStatus_TASK_STATUS_IN_PROGRESS:
		return core.TaskStatusInProgress
	case monov1.TaskStatus_TASK_STATUS_BLOCKED:
		return core.TaskStatusBlocked
	case monov1.TaskStatus_TASK_STATUS_DONE:
		return core.TaskStatusDone
	case monov1.TaskStatus_TASK_STATUS_ARCHIVED:
		return core.TaskStatusArchived
	case monov1.TaskStatus_TASK_STATUS_CANCELLED:
		return core.TaskStatusCancelled
	default:
		return core.TaskStatusTodo
	}
}

func toProtoStatus(s core.TaskStatus) monov1.TaskStatus {
	switch s {
	case core.TaskStatusTodo:
		return monov1.TaskStatus_TASK_STATUS_TODO
	case core.TaskStatusInProgress:
		return monov1.TaskStatus_TASK_STATUS_IN_PROGRESS
	case core.TaskStatusBlocked:
		return monov1.TaskStatus_TASK_STATUS_BLOCKED
	case core.TaskStatusDone:
		return monov1.TaskStatus_TASK_STATUS_DONE
	case core.TaskStatusArchived:
		return monov1.TaskStatus_TASK_STATUS_ARCHIVED
	case core.TaskStatusCancelled:
		return monov1.TaskStatus_TASK_STATUS_CANCELLED
	default:
		return monov1.TaskStatus_TASK_STATUS_UNSPECIFIED
	}
}

func toCorePriority(p monov1.TaskPriority) core.TaskPriority {
	switch p {
	case monov1.TaskPriority_TASK_PRIORITY_LOW:
		return core.TaskPriorityLow
	case monov1.TaskPriority_TASK_PRIORITY_MEDIUM:
		return core.TaskPriorityMedium
	case monov1.TaskPriority_TASK_PRIORITY_HIGH:
		return core.TaskPriorityHigh
	case monov1.TaskPriority_TASK_PRIORITY_URGENT:
		return core.TaskPriorityUrgent
	default:
		return core.TaskPriorityLow
	}
}

func toProtoPriority(p core.TaskPriority) monov1.TaskPriority {
	switch p {
	case core.TaskPriorityLow:
		return monov1.TaskPriority_TASK_PRIORITY_LOW
	case core.TaskPriorityMedium:
		return monov1.TaskPriority_TASK_PRIORITY_MEDIUM
	case core.TaskPriorityHigh:
		return monov1.TaskPriority_TASK_PRIORITY_HIGH
	case core.TaskPriorityUrgent:
		return monov1.TaskPriority_TASK_PRIORITY_URGENT
	default:
		return monov1.TaskPriority_TASK_PRIORITY_UNSPECIFIED
	}
}

// encodePageToken encodes an offset into a base64 page token.
func encodePageToken(offset int) string {
	if offset <= 0 {
		return ""
	}
	// Simple encoding: just convert offset to string and base64 encode
	return strings.TrimRight(strings.Replace(
		strings.Replace(
			base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", offset))),
			"+", "-", -1),
		"/", "_", -1),
		"=")
}

// decodePageToken decodes a base64 page token into an offset.
func decodePageToken(token string) (int, error) {
	if token == "" {
		return 0, nil
	}

	// Reverse the URL-safe encoding
	token = strings.Replace(strings.Replace(token, "-", "+", -1), "_", "/", -1)

	// Add padding if needed
	switch len(token) % 4 {
	case 2:
		token += "=="
	case 3:
		token += "="
	}

	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return 0, fmt.Errorf("invalid token: %w", err)
	}

	var offset int
	_, err = fmt.Sscanf(string(decoded), "%d", &offset)
	if err != nil {
		return 0, fmt.Errorf("invalid token format: %w", err)
	}

	return offset, nil
}

// extractUserTimezone extracts the user's timezone from the X-User-Timezone header.
// Returns empty string if header is missing (caller should treat as UTC).
// Returns error if timezone is invalid IANA name.
func extractUserTimezone(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", nil // No metadata = use UTC default
	}

	tzHeaders := md.Get("x-user-timezone")
	if len(tzHeaders) == 0 {
		return "", nil // No header = use UTC default
	}

	userTZ := tzHeaders[0]
	if userTZ == "" {
		return "", nil // Empty header = use UTC default
	}

	// Validate IANA timezone
	_, err := time.LoadLocation(userTZ)
	if err != nil {
		return "", status.Errorf(codes.InvalidArgument, "invalid timezone: %s", userTZ)
	}

	return userTZ, nil
}
