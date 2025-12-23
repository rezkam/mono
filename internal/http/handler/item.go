package handler

import (
	"encoding/json"
	"net/http"

	"github.com/oapi-codegen/runtime/types"

	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/http/openapi"
	"github.com/rezkam/mono/internal/http/response"
)

// CreateItem implements ServerInterface.CreateItem.
// POST /v1/lists/{list_id}/items
func (s *Server) CreateItem(w http.ResponseWriter, r *http.Request, listID types.UUID) {
	// Parse request body
	var req openapi.CreateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid JSON")
		return
	}

	// Build domain item from request
	item := &domain.TodoItem{
		Title:    req.Title,
		ListID:   listID.String(),
		Tags:     derefStringSlice(req.Tags),
		Timezone: req.Timezone,
		DueTime:  req.DueTime,
	}

	// Parse estimated_duration if provided
	if req.EstimatedDuration != nil {
		duration, err := parseDuration(*req.EstimatedDuration)
		if err != nil {
			response.BadRequest(w, "invalid estimated_duration: "+err.Error())
			return
		}
		item.EstimatedDuration = &duration
	}

	// Validate and set priority if provided
	if req.Priority != nil {
		priority, err := domain.NewTaskPriority(string(*req.Priority))
		if err != nil {
			response.FromDomainError(w, r, err)
			return
		}
		item.Priority = &priority
	}

	// Set recurring template if provided
	if req.RecurringTemplateId != nil {
		templateID := req.RecurringTemplateId.String()
		item.RecurringTemplateID = &templateID
	}

	// Set instance date if provided
	if req.InstanceDate != nil {
		item.InstanceDate = req.InstanceDate
	}

	// Call service layer (validation and ID generation happens here)
	createdItem, err := s.todoService.CreateItem(r.Context(), listID.String(), item)
	if err != nil {
		response.FromDomainError(w, r, err)
		return
	}

	// Map domain model to DTO
	itemDTO := MapItemToDTO(createdItem)

	// Return success response
	response.Created(w, openapi.CreateItemResponse{
		Item: &itemDTO,
	})
}

// UpdateItem implements ServerInterface.UpdateItem.
// PATCH /v1/lists/{list_id}/items/{item_id}
func (s *Server) UpdateItem(w http.ResponseWriter, r *http.Request, listID types.UUID, itemID types.UUID) {
	// Parse request body
	var req openapi.UpdateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid JSON")
		return
	}

	if req.Item == nil {
		response.BadRequest(w, "item is required")
		return
	}

	// Build UpdateItemParams from request
	params := domain.UpdateItemParams{
		ItemID: itemID.String(),
		ListID: listID.String(),
		Etag:   req.Item.Etag,
	}

	// Determine update mask
	if req.UpdateMask == nil || len(*req.UpdateMask) == 0 {
		// No mask specified - update all provided fields
		params.UpdateMask = []string{}
		if req.Item.Title != nil {
			params.UpdateMask = append(params.UpdateMask, "title")
		}
		if req.Item.Status != nil {
			params.UpdateMask = append(params.UpdateMask, "status")
		}
		if req.Item.Priority != nil {
			params.UpdateMask = append(params.UpdateMask, "priority")
		}
		if req.Item.DueTime != nil {
			params.UpdateMask = append(params.UpdateMask, "due_time")
		}
		if req.Item.Tags != nil {
			params.UpdateMask = append(params.UpdateMask, "tags")
		}
		if req.Item.Timezone != nil {
			params.UpdateMask = append(params.UpdateMask, "timezone")
		}
		if req.Item.EstimatedDuration != nil {
			params.UpdateMask = append(params.UpdateMask, "estimated_duration")
		}
		if req.Item.ActualDuration != nil {
			params.UpdateMask = append(params.UpdateMask, "actual_duration")
		}
	} else {
		params.UpdateMask = *req.UpdateMask
	}

	// Map field values from request to params
	for _, field := range params.UpdateMask {
		switch field {
		case "title":
			params.Title = req.Item.Title
		case "status":
			if req.Item.Status != nil {
				status := domain.TaskStatus(*req.Item.Status)
				params.Status = &status
			}
		case "priority":
			if req.Item.Priority != nil {
				priority := domain.TaskPriority(*req.Item.Priority)
				params.Priority = &priority
			}
		case "due_time":
			params.DueTime = req.Item.DueTime
		case "tags":
			if req.Item.Tags != nil {
				params.Tags = req.Item.Tags
			}
		case "timezone":
			params.Timezone = req.Item.Timezone
		case "estimated_duration":
			if req.Item.EstimatedDuration != nil {
				duration, err := parseDuration(*req.Item.EstimatedDuration)
				if err != nil {
					response.BadRequest(w, "invalid estimated_duration: "+err.Error())
					return
				}
				params.EstimatedDuration = &duration
			}
		case "actual_duration":
			if req.Item.ActualDuration != nil {
				duration, err := parseDuration(*req.Item.ActualDuration)
				if err != nil {
					response.BadRequest(w, "invalid actual_duration: "+err.Error())
					return
				}
				params.ActualDuration = &duration
			}
		}
	}

	// Call service layer - returns updated item
	updated, err := s.todoService.UpdateItem(r.Context(), params)
	if err != nil {
		response.FromDomainError(w, r, err)
		return
	}

	// Map domain model to DTO
	itemDTO := MapItemToDTO(updated)

	// Return success response
	response.OK(w, openapi.UpdateItemResponse{
		Item: &itemDTO,
	})
}

// ListTasks implements ServerInterface.ListTasks.
// GET /v1/tasks
func (s *Server) ListTasks(w http.ResponseWriter, r *http.Request, params openapi.ListTasksParams) {
	// Build domain params from query params
	offset := parsePageToken(params.PageToken)

	domainParams := domain.ListTasksParams{
		Limit:  getPageSize(params.PageSize),
		Offset: offset,
	}

	// Set parent list if specified
	if params.Parent != nil {
		parentStr := params.Parent.String()
		domainParams.ListID = &parentStr
	}

	// Set status filter if specified
	if params.Status != nil {
		status := domain.TaskStatus(*params.Status)
		domainParams.Status = &status
	}

	// Set priority filter if specified
	if params.Priority != nil {
		priority := domain.TaskPriority(*params.Priority)
		domainParams.Priority = &priority
	}

	// Set sorting if specified
	if params.SortBy != nil {
		domainParams.OrderBy = string(*params.SortBy)
	}
	if params.SortDir != nil {
		domainParams.OrderDir = string(*params.SortDir)
	}

	// Call service layer
	result, err := s.todoService.ListTasks(r.Context(), domainParams)
	if err != nil {
		response.FromDomainError(w, r, err)
		return
	}

	// Map domain models to DTOs
	itemDTOs := make([]openapi.TodoItem, len(result.Items))
	for i, item := range result.Items {
		itemDTOs[i] = MapItemToDTO(&item)
	}

	// Generate next page token based on repository result
	nextToken := generatePageToken(offset+len(result.Items), result.HasMore)

	// Return success response
	response.OK(w, openapi.ListTasksResponse{
		Items:         &itemDTOs,
		NextPageToken: nextToken,
	})
}

// Helper to dereference []string pointer
func derefStringSlice(s *[]string) []string {
	if s == nil {
		return []string{}
	}
	return *s
}
