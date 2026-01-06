package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/oapi-codegen/runtime/types"

	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/http/openapi"
	"github.com/rezkam/mono/internal/infrastructure/http/response"
)

// CreateItem implements ServerInterface.CreateItem.
// POST /v1/lists/{list_id}/items
func (h *TodoHandler) CreateItem(w http.ResponseWriter, r *http.Request, listID types.UUID) {
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
		DueAt:    req.DueAt,
	}

	// Parse estimated_duration if provided
	if req.EstimatedDuration != nil {
		d, err := domain.NewDuration(*req.EstimatedDuration)
		if err != nil {
			response.FromDomainError(w, r, err)
			return
		}
		duration := d.Value()
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

	// Set occurrence time if provided
	if req.InstanceDate != nil {
		item.OccursAt = req.InstanceDate
	}

	// Set starts_at if provided
	if req.StartsAt != nil {
		startsAt := req.StartsAt.Time
		item.StartsAt = &startsAt
	}

	// Parse due_offset if provided
	if req.DueOffset != nil {
		d, err := domain.NewDuration(*req.DueOffset)
		if err != nil {
			response.FromDomainError(w, r, err)
			return
		}
		duration := d.Value()
		item.DueOffset = &duration
	}

	// Call service layer (validation and ID generation happens here)
	createdItem, err := h.todoService.CreateItem(r.Context(), listID.String(), item)
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to create item via HTTP",
			"list_id", listID.String(),
			"title", req.Title,
			"error", err)
		response.FromDomainError(w, r, err)
		return
	}

	slog.InfoContext(r.Context(), "item created via HTTP",
		"item_id", createdItem.ID,
		"list_id", listID.String())

	// Map domain model to DTO
	itemDTO := MapItemToDTO(createdItem)

	// Return success response
	response.Created(w, openapi.CreateItemResponse{
		Item: &itemDTO,
	})
}

// UpdateItem implements ServerInterface.UpdateItem.
// PATCH /v1/lists/{list_id}/items/{item_id}
func (h *TodoHandler) UpdateItem(w http.ResponseWriter, r *http.Request, listID types.UUID, itemID types.UUID) {
	// Parse request body
	var req openapi.UpdateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid JSON")
		return
	}

	// Build UpdateItemParams from request
	// Note: item and update_mask are required by OpenAPI spec
	params := domain.UpdateItemParams{
		ItemID: itemID.String(),
		ListID: listID.String(),
		Etag:   req.Item.Etag,
	}

	// Convert update_mask enum values to strings
	// OpenAPI validates that only allowed field names are present
	params.UpdateMask = make([]string, len(req.UpdateMask))
	for i, m := range req.UpdateMask {
		params.UpdateMask[i] = string(m)
	}

	// Map field values from request to params based on update_mask
	for _, field := range params.UpdateMask {
		switch field {
		case "title":
			params.Title = req.Item.Title
		case "status":
			if req.Item.Status != nil {
				status, err := domain.NewTaskStatus(string(*req.Item.Status))
				if err != nil {
					response.FromDomainError(w, r, err)
					return
				}
				params.Status = &status
			}
		case "priority":
			if req.Item.Priority != nil {
				priority, err := domain.NewTaskPriority(string(*req.Item.Priority))
				if err != nil {
					response.FromDomainError(w, r, err)
					return
				}
				params.Priority = &priority
			}
		case "due_at":
			params.DueAt = req.Item.DueAt
		case "tags":
			if req.Item.Tags != nil {
				params.Tags = req.Item.Tags
			}
		case "timezone":
			params.Timezone = req.Item.Timezone
		case "estimated_duration":
			if req.Item.EstimatedDuration != nil {
				d, err := domain.NewDuration(*req.Item.EstimatedDuration)
				if err != nil {
					response.FromDomainError(w, r, err)
					return
				}
				duration := d.Value()
				params.EstimatedDuration = &duration
			}
		case "actual_duration":
			if req.Item.ActualDuration != nil {
				d, err := domain.NewDuration(*req.Item.ActualDuration)
				if err != nil {
					response.FromDomainError(w, r, err)
					return
				}
				duration := d.Value()
				params.ActualDuration = &duration
			}
		case "starts_at":
			if req.Item.StartsAt != nil {
				startsAt := req.Item.StartsAt.Time
				params.StartsAt = &startsAt
			}
		case "due_offset":
			if req.Item.DueOffset != nil {
				d, err := domain.NewDuration(*req.Item.DueOffset)
				if err != nil {
					response.FromDomainError(w, r, err)
					return
				}
				duration := d.Value()
				params.DueOffset = &duration
			}
		}
	}

	// Call service layer - returns updated item
	updated, err := h.todoService.UpdateItem(r.Context(), params)
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to update item via HTTP",
			"list_id", listID.String(),
			"item_id", itemID.String(),
			"update_mask", params.UpdateMask,
			"error", err)
		response.FromDomainError(w, r, err)
		return
	}

	slog.InfoContext(r.Context(), "item updated via HTTP",
		"item_id", itemID.String(),
		"list_id", listID.String(),
		"update_mask", params.UpdateMask)

	// Map domain model to DTO
	itemDTO := MapItemToDTO(updated)

	// Return success response
	response.OK(w, openapi.UpdateItemResponse{
		Item: &itemDTO,
	})
}

// DeleteItem implements ServerInterface.DeleteItem.
// DELETE /v1/lists/{list_id}/items/{item_id}
func (h *TodoHandler) DeleteItem(w http.ResponseWriter, r *http.Request, listID types.UUID, itemID types.UUID) {
	if err := h.todoService.DeleteItem(r.Context(), listID.String(), itemID.String()); err != nil {
		slog.ErrorContext(r.Context(), "failed to delete item via HTTP",
			"list_id", listID.String(),
			"item_id", itemID.String(),
			"error", err)
		response.FromDomainError(w, r, err)
		return
	}

	slog.InfoContext(r.Context(), "item deleted via HTTP",
		"item_id", itemID.String(),
		"list_id", listID.String())

	response.NoContent(w)
}

// ListItems implements ServerInterface.ListItems.
// GET /v1/lists/{list_id}/items
func (h *TodoHandler) ListItems(w http.ResponseWriter, r *http.Request, listID types.UUID, params openapi.ListItemsParams) {
	offset := parsePageToken(params.PageToken)
	listIDStr := listID.String()

	// Map OpenAPI params to domain filter input - just pass through values
	filterInput := domain.ItemsFilterInput{
		Statuses:   mapStatusesToStrings(params.Status),
		Priorities: mapPrioritiesToStrings(params.Priority),
		Tags:       derefStringSlice(params.Tags),
		OrderBy:    mapSortByToString(params.SortBy),
		OrderDir:   mapSortDirToString(params.SortDir),
	}

	// Create validated filter - domain layer validates all fields
	filter, err := domain.NewItemsFilter(filterInput)
	if err != nil {
		response.FromDomainError(w, r, err)
		return
	}

	// Build domain params with validated filter
	domainParams := domain.ListTasksParams{
		Limit:  getPageSize(params.PageSize),
		Offset: offset,
		ListID: &listIDStr,
		Filter: filter,
	}

	// Call service layer
	result, err := h.todoService.ListItems(r.Context(), domainParams)
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to list items via HTTP",
			"list_id", listID.String(),
			"offset", offset,
			"limit", domainParams.Limit,
			"error", err)
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
	response.OK(w, openapi.ListItemsResponse{
		Items:         &itemDTOs,
		NextPageToken: nextToken,
	})
}

// mapStatusesToStrings converts OpenAPI status slice to string slice
func mapStatusesToStrings(statuses *[]openapi.ListItemsParamsStatus) []string {
	if statuses == nil {
		return nil
	}
	result := make([]string, len(*statuses))
	for i, s := range *statuses {
		result[i] = string(s)
	}
	return result
}

// mapPrioritiesToStrings converts OpenAPI priority slice to string slice
func mapPrioritiesToStrings(priorities *[]openapi.ListItemsParamsPriority) []string {
	if priorities == nil {
		return nil
	}
	result := make([]string, len(*priorities))
	for i, p := range *priorities {
		result[i] = string(p)
	}
	return result
}

// mapSortByToString converts OpenAPI sort_by to string pointer
func mapSortByToString(sortBy *openapi.ListItemsParamsSortBy) *string {
	if sortBy == nil {
		return nil
	}
	s := string(*sortBy)
	return &s
}

// mapSortDirToString converts OpenAPI sort_dir to string pointer
func mapSortDirToString(sortDir *openapi.ListItemsParamsSortDir) *string {
	if sortDir == nil {
		return nil
	}
	s := string(*sortDir)
	return &s
}

// Helper to dereference []string pointer
func derefStringSlice(s *[]string) []string {
	if s == nil {
		return []string{}
	}
	return *s
}
