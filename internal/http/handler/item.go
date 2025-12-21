package handler

import (
	"encoding/json"
	"fmt"
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

	// Get existing item
	existing, err := s.todoService.GetItem(r.Context(), itemID.String())
	if err != nil {
		response.FromDomainError(w, r, err)
		return
	}

	// Apply updates based on update_mask
	// If no update_mask or empty, update all fields
	if req.UpdateMask == nil || len(*req.UpdateMask) == 0 {
		// Update all fields from request
		if req.Item.Title != nil {
			// Validate title (even if empty) to prevent bypassing domain validation
			title, err := domain.NewTitle(*req.Item.Title)
			if err != nil {
				response.FromDomainError(w, r, err)
				return
			}
			existing.Title = title.String()
		}
		if req.Item.Status != nil {
			status, err := domain.NewTaskStatus(string(*req.Item.Status))
			if err != nil {
				response.FromDomainError(w, r, err)
				return
			}
			existing.Status = status
		}
		if req.Item.Priority != nil {
			priority, err := domain.NewTaskPriority(string(*req.Item.Priority))
			if err != nil {
				response.FromDomainError(w, r, err)
				return
			}
			existing.Priority = &priority
		}
		if req.Item.DueTime != nil {
			existing.DueTime = req.Item.DueTime
		}
		if req.Item.Tags != nil {
			existing.Tags = *req.Item.Tags
		}
		if req.Item.Timezone != nil {
			existing.Timezone = req.Item.Timezone
		}
		if req.Item.EstimatedDuration != nil {
			duration, err := parseDuration(*req.Item.EstimatedDuration)
			if err != nil {
				response.BadRequest(w, "invalid estimated_duration: "+err.Error())
				return
			}
			existing.EstimatedDuration = &duration
		}
		if req.Item.ActualDuration != nil {
			duration, err := parseDuration(*req.Item.ActualDuration)
			if err != nil {
				response.BadRequest(w, "invalid actual_duration: "+err.Error())
				return
			}
			existing.ActualDuration = &duration
		}
	} else {
		// Update only specified fields
		for _, field := range *req.UpdateMask {
			switch field {
			case "title":
				if req.Item.Title != nil {
					// Validate title (even if empty) to prevent bypassing domain validation
					title, err := domain.NewTitle(*req.Item.Title)
					if err != nil {
						response.FromDomainError(w, r, err)
						return
					}
					existing.Title = title.String()
				}
			case "status":
				if req.Item.Status != nil {
					status, err := domain.NewTaskStatus(string(*req.Item.Status))
					if err != nil {
						response.FromDomainError(w, r, err)
						return
					}
					existing.Status = status
				}
			case "priority":
				if req.Item.Priority != nil {
					priority, err := domain.NewTaskPriority(string(*req.Item.Priority))
					if err != nil {
						response.FromDomainError(w, r, err)
						return
					}
					existing.Priority = &priority
				}
			case "due_time":
				existing.DueTime = req.Item.DueTime
			case "tags":
				if req.Item.Tags != nil {
					existing.Tags = *req.Item.Tags
				}
			case "timezone":
				existing.Timezone = req.Item.Timezone
			case "estimated_duration":
				if req.Item.EstimatedDuration != nil {
					duration, err := parseDuration(*req.Item.EstimatedDuration)
					if err != nil {
						response.BadRequest(w, "invalid estimated_duration: "+err.Error())
						return
					}
					existing.EstimatedDuration = &duration
				}
			case "actual_duration":
				if req.Item.ActualDuration != nil {
					duration, err := parseDuration(*req.Item.ActualDuration)
					if err != nil {
						response.BadRequest(w, "invalid actual_duration: "+err.Error())
						return
					}
					existing.ActualDuration = &duration
				}
			}
		}
	}

	// Call service layer
	if err := s.todoService.UpdateItem(r.Context(), listID.String(), existing); err != nil {
		response.FromDomainError(w, r, err)
		return
	}

	// Fetch updated item to return
	updated, err := s.todoService.GetItem(r.Context(), itemID.String())
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

	// Parse filter (AIP-160)
	if params.Filter != nil && *params.Filter != "" {
		// TODO: Full AIP-160 parser - for now just log warning
		// filterCriteria, err := parseFilter(*params.Filter)
		// if err != nil {
		//     response.BadRequest(w, fmt.Sprintf("invalid filter: %v", err))
		//     return
		// }
		// Apply filter criteria to domainParams
	}

	// Parse order_by (AIP-132)
	if params.OrderBy != nil && *params.OrderBy != "" {
		field, direction, err := parseOrderBy(*params.OrderBy)
		if err != nil {
			response.BadRequest(w, fmt.Sprintf("invalid order_by: %v", err))
			return
		}
		domainParams.OrderBy = field
		domainParams.OrderDir = direction
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
