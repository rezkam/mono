// Package service provides thin gRPC handlers that translate between protocol buffers and domain models.
//
// ARCHITECTURE DECISION: Thin Handler Layer
//
// This layer is intentionally kept thin (~15-20 lines per handler) with zero business logic.
// Each handler follows a strict 4-step pattern:
//
//  1. Validate - Protocol-level validation only (empty checks, format validation)
//  2. Convert - Transform protobuf messages to domain models
//  3. Delegate - Call application service (where ALL business logic lives)
//  4. Map & Convert - Map domain errors to gRPC codes, convert domain models back to protobuf
//
// Example:
//
//	func (s *MonoService) CreateItem(ctx, req) {
//	    if req.Title == "" { return InvalidArgument }        // 1. Validate
//	    item := protoToTodoItem(req)                         // 2. Convert
//	    created, err := s.service.CreateItem(ctx, req.ListId, item) // 3. Delegate
//	    if err != nil { return mapError(err) }               // 4. Map errors
//	    return &Response{Item: toProtoItem(*created)}, nil   // 4. Convert response
//	}
//
// WHY THIS PATTERN:
//   - Business logic in application layer is testable without gRPC overhead
//   - Same application service reused by gRPC, REST gateway, CLI, background workers
//   - Clear separation: handlers do protocol translation, services do business logic
//   - Field mask handling stays here as it's a protocol-level optimization
package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type MonoService struct {
	monov1.UnimplementedMonoServiceServer
	service         *todo.Service
	defaultPageSize int
	maxPageSize     int
}

func NewMonoService(service *todo.Service, defaultPageSize, maxPageSize int) *MonoService {
	return &MonoService{
		service:         service,
		defaultPageSize: defaultPageSize,
		maxPageSize:     maxPageSize,
	}
}

func (s *MonoService) CreateList(ctx context.Context, req *monov1.CreateListRequest) (*monov1.CreateListResponse, error) {
	// Validate protocol-level requirements
	if req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "title is required")
	}

	list, err := s.service.CreateList(ctx, req.Title)
	if err != nil {
		return nil, mapError(err)
	}

	return &monov1.CreateListResponse{
		List: toProtoList(list),
	}, nil
}

func (s *MonoService) GetList(ctx context.Context, req *monov1.GetListRequest) (*monov1.GetListResponse, error) {
	// Validate protocol-level requirements
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	list, err := s.service.GetList(ctx, req.Id)
	if err != nil {
		return nil, mapError(err)
	}

	return &monov1.GetListResponse{
		List: toProtoList(list),
	}, nil
}

func (s *MonoService) CreateItem(ctx context.Context, req *monov1.CreateItemRequest) (*monov1.CreateItemResponse, error) {
	// Validate protocol-level requirements
	if req.ListId == "" {
		return nil, status.Error(codes.InvalidArgument, "list_id is required")
	}
	if req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "title is required")
	}

	// Convert proto to domain
	item, err := protoToTodoItem(req)
	if err != nil {
		return nil, err // Already a status error
	}

	// Delegate to application service
	created, err := s.service.CreateItem(ctx, req.ListId, item)
	if err != nil {
		return nil, mapError(err)
	}

	return &monov1.CreateItemResponse{
		Item: toProtoItem(*created),
	}, nil
}

func (s *MonoService) UpdateItem(ctx context.Context, req *monov1.UpdateItemRequest) (*monov1.UpdateItemResponse, error) {
	// Validate required fields to prevent nil pointer dereference
	if req.Item == nil {
		return nil, status.Error(codes.InvalidArgument, "item is required")
	}
	if req.Item.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "item.id is required")
	}
	if req.ListId == "" {
		return nil, status.Error(codes.InvalidArgument, "list_id is required")
	}

	// O(1) lookup: Fetch only the item being updated, not the entire list
	// This replaces the previous O(N) approach that loaded all items in the list
	item, err := s.service.GetItem(ctx, req.Item.Id)
	if err != nil {
		return nil, mapError(err)
	}

	// Apply field mask to update item
	if err := applyItemFieldMask(item, req.Item, req.UpdateMask); err != nil {
		return nil, err // Already a status error
	}

	// Delegate to application service with list_id validation
	// This prevents users from updating items in lists they don't own
	if err := s.service.UpdateItem(ctx, req.ListId, item); err != nil {
		return nil, mapError(err)
	}

	return &monov1.UpdateItemResponse{Item: toProtoItem(*item)}, nil
}

func (s *MonoService) ListLists(ctx context.Context, req *monov1.ListListsRequest) (*monov1.ListListsResponse, error) {
	lists, err := s.service.ListLists(ctx)
	if err != nil {
		return nil, mapError(err)
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
	params := domain.ListTasksParams{
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
				status := domain.TaskStatus(value)
				params.Status = &status
			case "priority":
				priority := domain.TaskPriority(value)
				params.Priority = &priority
			case "tags":
				params.Tag = &value
			}
		}
	}

	// Set ordering (default: created_at desc)
	// Supports AIP-132 style: "field" or "field desc" / "field asc"
	if req.OrderBy != "" {
		field, direction := parseOrderByField(req.OrderBy)
		if field == "" {
			return nil, status.Errorf(codes.InvalidArgument,
				"invalid order_by: %q (supported: due_time, priority, created_at, updated_at, create_time; optional: asc/desc)",
				req.OrderBy)
		}
		params.OrderBy = field
		params.OrderDir = direction
	}

	// Execute query
	result, err := s.service.ListTasks(ctx, params)
	if err != nil {
		return nil, mapError(err)
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

func toProtoList(l *domain.TodoList) *monov1.TodoList {
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

func toProtoItem(i domain.TodoItem) *monov1.TodoItem {
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

func toCoreStatus(s monov1.TaskStatus) domain.TaskStatus {
	switch s {
	case monov1.TaskStatus_TASK_STATUS_TODO:
		return domain.TaskStatusTodo
	case monov1.TaskStatus_TASK_STATUS_IN_PROGRESS:
		return domain.TaskStatusInProgress
	case monov1.TaskStatus_TASK_STATUS_BLOCKED:
		return domain.TaskStatusBlocked
	case monov1.TaskStatus_TASK_STATUS_DONE:
		return domain.TaskStatusDone
	case monov1.TaskStatus_TASK_STATUS_ARCHIVED:
		return domain.TaskStatusArchived
	case monov1.TaskStatus_TASK_STATUS_CANCELLED:
		return domain.TaskStatusCancelled
	default:
		return domain.TaskStatusTodo
	}
}

func toProtoStatus(s domain.TaskStatus) monov1.TaskStatus {
	switch s {
	case domain.TaskStatusTodo:
		return monov1.TaskStatus_TASK_STATUS_TODO
	case domain.TaskStatusInProgress:
		return monov1.TaskStatus_TASK_STATUS_IN_PROGRESS
	case domain.TaskStatusBlocked:
		return monov1.TaskStatus_TASK_STATUS_BLOCKED
	case domain.TaskStatusDone:
		return monov1.TaskStatus_TASK_STATUS_DONE
	case domain.TaskStatusArchived:
		return monov1.TaskStatus_TASK_STATUS_ARCHIVED
	case domain.TaskStatusCancelled:
		return monov1.TaskStatus_TASK_STATUS_CANCELLED
	default:
		return monov1.TaskStatus_TASK_STATUS_UNSPECIFIED
	}
}

func toCorePriority(p monov1.TaskPriority) domain.TaskPriority {
	switch p {
	case monov1.TaskPriority_TASK_PRIORITY_LOW:
		return domain.TaskPriorityLow
	case monov1.TaskPriority_TASK_PRIORITY_MEDIUM:
		return domain.TaskPriorityMedium
	case monov1.TaskPriority_TASK_PRIORITY_HIGH:
		return domain.TaskPriorityHigh
	case monov1.TaskPriority_TASK_PRIORITY_URGENT:
		return domain.TaskPriorityUrgent
	default:
		return domain.TaskPriorityLow
	}
}

func toProtoPriority(p domain.TaskPriority) monov1.TaskPriority {
	switch p {
	case domain.TaskPriorityLow:
		return monov1.TaskPriority_TASK_PRIORITY_LOW
	case domain.TaskPriorityMedium:
		return monov1.TaskPriority_TASK_PRIORITY_MEDIUM
	case domain.TaskPriorityHigh:
		return monov1.TaskPriority_TASK_PRIORITY_HIGH
	case domain.TaskPriorityUrgent:
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
// Returns an error for negative offsets or values that exceed int32 range.
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

	// Validate offset is non-negative
	if offset < 0 {
		return 0, fmt.Errorf("invalid token: offset must be non-negative")
	}

	// Validate offset fits in int32 (database OFFSET parameter limit)
	const maxInt32 = 2147483647
	if offset > maxInt32 {
		return 0, fmt.Errorf("invalid token: offset exceeds maximum allowed value")
	}

	return offset, nil
}

// parseOrderByField parses an AIP-132 style order_by string and extracts field and direction.
// Supports both bare field names ("created_at") and AIP-132 style ("created_at desc", "created_at asc").
// Returns the normalized field name and direction ("asc" or "desc").
// Returns empty strings if invalid.
//
// This validation provides clear error messages to API users (UX), not security.
// Security against SQL injection is guaranteed by parameterized queries in the storage layer.
// See tests/integration/sql_injection_resistance_test.go for proof that even with direct
// assignment (params.OrderBy = req.OrderBy) and no validation, SQL injection is impossible.
func parseOrderByField(orderBy string) (field string, direction string) {
	validFields := map[string]bool{
		"due_time":    true,
		"priority":    true,
		"created_at":  true,
		"updated_at":  true,
		"create_time": true, // Alias for proto docs compatibility
	}

	// Normalize to lowercase and trim spaces
	orderBy = strings.TrimSpace(strings.ToLower(orderBy))

	// Check for AIP-132 style with direction: "field desc" or "field asc"
	parts := strings.Fields(orderBy)
	if len(parts) == 0 {
		return "", ""
	}

	field = parts[0]

	// If there's a direction, validate it
	if len(parts) == 2 {
		direction = parts[1]
		if direction != "asc" && direction != "desc" {
			return "", "" // Invalid direction
		}
	} else if len(parts) > 2 {
		return "", "" // Too many parts
	}

	// Normalize create_time to created_at (proto uses create_time, SQL uses created_at)
	if field == "create_time" {
		field = "created_at"
	}

	if validFields[field] {
		return field, direction
	}
	return "", ""
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

// protoToTodoItem converts a CreateItemRequest to a domain TodoItem.
func protoToTodoItem(req *monov1.CreateItemRequest) (*domain.TodoItem, error) {
	item := &domain.TodoItem{
		Title: req.Title,
		Tags:  req.Tags,
	}

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
		if _, err := time.LoadLocation(req.Timezone); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid timezone: %s", req.Timezone)
		}
		item.Timezone = &req.Timezone
	}

	if req.RecurringTemplateId != "" {
		item.RecurringTemplateID = &req.RecurringTemplateId
	}

	if req.InstanceDate != nil {
		t := req.InstanceDate.AsTime()
		item.InstanceDate = &t
	}

	return item, nil
}

// applyItemFieldMask applies a field mask to enable partial updates.
//
// WHY FIELD MASKS:
// Field masks allow clients to update only specific fields without sending the entire object.
// This is a protocol-level optimization that solves several problems:
//
//  1. BANDWIDTH: Send only changed fields, not entire object
//     Example: Update just status → send {id: "123", status: "DONE"}, mask: ["status"]
//     Instead of: Send entire item with all 15+ fields
//
//  2. RACE CONDITIONS: Prevent accidental overwrites
//     Example: User A updates title, User B updates status simultaneously
//     Without mask: Last write wins, one update is lost
//     With mask: Both updates succeed, updating different fields
//
//  3. BACKWARD COMPATIBILITY: Old clients don't break when new fields are added
//     Example: Add "reminder_time" field to TodoItem
//     Old clients sending full updates won't accidentally null out the new field
//
// USAGE EXAMPLES:
//
//	Update only status:
//	  UpdateItem({id: "123", status: "DONE"}, mask: ["status"])
//	  → Only status changes, title/tags/priority unchanged
//
//	Update multiple fields:
//	  UpdateItem({id: "123", title: "New", priority: "HIGH"}, mask: ["title", "priority"])
//	  → Only title and priority change
//
//	Update all fields (no mask):
//	  UpdateItem({...full item...}, mask: nil)
//	  → All mutable fields updated (legacy behavior)
//
// PROTOCOL-LEVEL CONCERN:
// Field masks are a protobuf/gRPC optimization, not business logic.
// They stay in the handler layer (not application layer) because:
//   - Application layer works with complete domain models
//   - Field mask logic is protocol-specific (gRPC/protobuf concept)
//   - Different protocols may have different partial update mechanisms
func applyItemFieldMask(item *domain.TodoItem, protoItem *monov1.TodoItem, mask *fieldmaskpb.FieldMask) error {
	if mask == nil || len(mask.Paths) == 0 {
		// No mask provided - update all mutable fields (legacy full-update behavior)
		item.Title = protoItem.Title

		if protoItem.Status != monov1.TaskStatus_TASK_STATUS_UNSPECIFIED {
			item.Status = toCoreStatus(protoItem.Status)
		}

		if protoItem.Priority != monov1.TaskPriority_TASK_PRIORITY_UNSPECIFIED {
			p := toCorePriority(protoItem.Priority)
			item.Priority = &p
		} else {
			item.Priority = nil
		}

		item.Tags = protoItem.Tags

		if protoItem.EstimatedDuration != nil {
			d := protoItem.EstimatedDuration.AsDuration()
			item.EstimatedDuration = &d
		} else {
			item.EstimatedDuration = nil
		}

		if protoItem.ActualDuration != nil {
			d := protoItem.ActualDuration.AsDuration()
			item.ActualDuration = &d
		} else {
			item.ActualDuration = nil
		}

		if protoItem.DueTime != nil {
			t := protoItem.DueTime.AsTime()
			item.DueTime = &t
		} else {
			item.DueTime = nil
		}

		if protoItem.Timezone != "" {
			if _, err := time.LoadLocation(protoItem.Timezone); err != nil {
				return status.Errorf(codes.InvalidArgument, "invalid timezone: %s", protoItem.Timezone)
			}
			item.Timezone = &protoItem.Timezone
		} else {
			item.Timezone = nil
		}
	} else {
		// Apply only specified fields
		for _, path := range mask.Paths {
			switch path {
			case "title":
				item.Title = protoItem.Title
			case "status":
				item.Status = toCoreStatus(protoItem.Status)
			case "priority":
				if protoItem.Priority != monov1.TaskPriority_TASK_PRIORITY_UNSPECIFIED {
					p := toCorePriority(protoItem.Priority)
					item.Priority = &p
				} else {
					item.Priority = nil
				}
			case "tags":
				item.Tags = protoItem.Tags
			case "due_time":
				if protoItem.DueTime != nil {
					t := protoItem.DueTime.AsTime()
					item.DueTime = &t
				} else {
					item.DueTime = nil
				}
			case "estimated_duration":
				if protoItem.EstimatedDuration != nil {
					d := protoItem.EstimatedDuration.AsDuration()
					item.EstimatedDuration = &d
				} else {
					item.EstimatedDuration = nil
				}
			case "actual_duration":
				if protoItem.ActualDuration != nil {
					d := protoItem.ActualDuration.AsDuration()
					item.ActualDuration = &d
				} else {
					item.ActualDuration = nil
				}
			case "timezone":
				if protoItem.Timezone != "" {
					if _, err := time.LoadLocation(protoItem.Timezone); err != nil {
						return status.Errorf(codes.InvalidArgument, "invalid timezone: %s", protoItem.Timezone)
					}
					item.Timezone = &protoItem.Timezone
				} else {
					item.Timezone = nil
				}
			}
		}
	}

	return nil
}

// mapError maps domain errors to gRPC status codes.
func mapError(err error) error {
	if err == nil {
		return nil
	}

	// Map domain errors to gRPC codes
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrListNotFound) {
		return status.Error(codes.NotFound, err.Error())
	}
	if errors.Is(err, domain.ErrInvalidID) {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	// Default to Internal for unmapped errors
	return status.Errorf(codes.Internal, "%v", err)
}
