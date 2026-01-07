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

// CreateRecurringTemplate implements ServerInterface.CreateRecurringTemplate.
// POST /v1/lists/{list_id}/recurring-templates
func (h *TodoHandler) CreateRecurringTemplate(w http.ResponseWriter, r *http.Request, listID types.UUID) {
	// Parse request body
	var req openapi.CreateRecurringTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid JSON")
		return
	}

	// Build domain model from DTO
	pattern, err := domain.NewRecurrencePattern(string(req.RecurrencePattern))
	if err != nil {
		response.FromDomainError(w, r, err)
		return
	}

	template := &domain.RecurringTemplate{
		ListID:            listID.String(),
		Title:             req.Title,
		Tags:              []string{},
		RecurrencePattern: pattern,
	}

	if req.Tags != nil {
		template.Tags = *req.Tags
	}

	if req.Priority != nil {
		priority, err := domain.NewTaskPriority(string(*req.Priority))
		if err != nil {
			response.FromDomainError(w, r, err)
			return
		}
		template.Priority = &priority
	}

	if req.EstimatedDuration != nil {
		d, err := domain.NewDuration(*req.EstimatedDuration)
		if err != nil {
			response.FromDomainFieldError(w, r, err, "estimated_duration")
			return
		}
		duration := d.Value()
		template.EstimatedDuration = &duration
	}

	if req.DueOffset != nil {
		d, err := domain.NewDuration(*req.DueOffset)
		if err != nil {
			response.FromDomainFieldError(w, r, err, "due_offset")
			return
		}
		duration := d.Value()
		template.DueOffset = &duration
	}

	// Map horizon fields with defaults
	if req.SyncHorizonDays != nil {
		template.SyncHorizonDays = *req.SyncHorizonDays
	} else {
		template.SyncHorizonDays = domain.DefaultSyncHorizonDays
	}

	if req.GenerationHorizonDays != nil {
		template.GenerationHorizonDays = *req.GenerationHorizonDays
	} else {
		template.GenerationHorizonDays = domain.DefaultGenerationHorizonDays
	}

	// RecurrenceConfig is JSON string, parse it to map
	// Default to empty object if not provided (database requires NOT NULL)
	if req.RecurrenceConfig != nil && *req.RecurrenceConfig != "" {
		var config map[string]any
		if err := json.Unmarshal([]byte(*req.RecurrenceConfig), &config); err != nil {
			response.BadRequest(w, "invalid recurrence_config JSON")
			return
		}
		template.RecurrenceConfig = config
	} else {
		// Set default empty config
		template.RecurrenceConfig = make(map[string]any)
	}

	// Call service layer (validation happens here)
	created, err := h.todoService.CreateRecurringTemplate(r.Context(), template)
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to create recurring template via HTTP",
			"list_id", listID.String(),
			"recurrence_pattern", string(req.RecurrencePattern),
			"error", err)
		response.FromDomainError(w, r, err)
		return
	}

	slog.InfoContext(r.Context(), "recurring template created via HTTP",
		"template_id", created.ID,
		"list_id", listID.String())

	// Map domain model to DTO
	templateDTO := MapTemplateToDTO(created)

	// Return success response
	response.Created(w, openapi.CreateRecurringTemplateResponse{
		Template: &templateDTO,
	})
}

// GetRecurringTemplate implements ServerInterface.GetRecurringTemplate.
// GET /v1/lists/{list_id}/recurring-templates/{template_id}
func (h *TodoHandler) GetRecurringTemplate(w http.ResponseWriter, r *http.Request, listID types.UUID, templateID types.UUID) {
	// Call service layer with list ownership validation
	template, err := h.todoService.FindRecurringTemplateByID(r.Context(), listID.String(), templateID.String())
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to get recurring template via HTTP",
			"list_id", listID.String(),
			"template_id", templateID.String(),
			"error", err)
		response.FromDomainError(w, r, err)
		return
	}

	// Map domain model to DTO
	templateDTO := MapTemplateToDTO(template)

	// Return success response
	response.OK(w, openapi.GetRecurringTemplateResponse{
		Template: &templateDTO,
	})
}

// UpdateRecurringTemplate implements ServerInterface.UpdateRecurringTemplate.
// PATCH /v1/lists/{list_id}/recurring-templates/{template_id}
func (h *TodoHandler) UpdateRecurringTemplate(w http.ResponseWriter, r *http.Request, listID types.UUID, templateID types.UUID) {
	// Parse request body
	var req openapi.UpdateRecurringTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid JSON")
		return
	}

	// Build UpdateRecurringTemplateParams
	// Note: template and update_mask are required by OpenAPI spec
	params := domain.UpdateRecurringTemplateParams{
		TemplateID: templateID.String(),
		ListID:     listID.String(),
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
			params.Title = req.Template.Title
		case "tags":
			params.Tags = req.Template.Tags
		case "priority":
			if req.Template.Priority != nil {
				priority, err := domain.NewTaskPriority(string(*req.Template.Priority))
				if err != nil {
					response.FromDomainError(w, r, err)
					return
				}
				params.Priority = &priority
			}
		case "estimated_duration":
			if req.Template.EstimatedDuration != nil {
				d, err := domain.NewDuration(*req.Template.EstimatedDuration)
				if err != nil {
					response.FromDomainFieldError(w, r, err, "estimated_duration")
					return
				}
				duration := d.Value()
				params.EstimatedDuration = &duration
			}
		case "due_offset":
			if req.Template.DueOffset != nil {
				d, err := domain.NewDuration(*req.Template.DueOffset)
				if err != nil {
					response.FromDomainFieldError(w, r, err, "due_offset")
					return
				}
				duration := d.Value()
				params.DueOffset = &duration
			}
		case "recurrence_pattern":
			if req.Template.RecurrencePattern != nil {
				pattern, err := domain.NewRecurrencePattern(string(*req.Template.RecurrencePattern))
				if err != nil {
					response.FromDomainError(w, r, err)
					return
				}
				params.RecurrencePattern = &pattern
			}
		case "recurrence_config":
			if req.Template.RecurrenceConfig != nil && *req.Template.RecurrenceConfig != "" {
				var config map[string]any
				if err := json.Unmarshal([]byte(*req.Template.RecurrenceConfig), &config); err != nil {
					response.BadRequest(w, "invalid recurrence_config JSON")
					return
				}
				params.RecurrenceConfig = config
			}
		case "is_active":
			params.IsActive = req.Template.IsActive
		case "sync_horizon_days":
			params.SyncHorizonDays = req.Template.SyncHorizonDays
		case "generation_horizon_days":
			params.GenerationHorizonDays = req.Template.GenerationHorizonDays
		}
	}

	// Call service layer (validation happens there)
	updated, err := h.todoService.UpdateRecurringTemplate(r.Context(), params)
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to update recurring template via HTTP",
			"list_id", listID.String(),
			"template_id", templateID.String(),
			"update_mask", params.UpdateMask,
			"error", err)
		response.FromDomainError(w, r, err)
		return
	}

	slog.InfoContext(r.Context(), "recurring template updated via HTTP",
		"template_id", templateID.String(),
		"list_id", listID.String(),
		"update_mask", params.UpdateMask)

	// Map domain model to DTO
	templateDTO := MapTemplateToDTO(updated)

	// Return success response
	response.OK(w, openapi.UpdateRecurringTemplateResponse{
		Template: &templateDTO,
	})
}

// DeleteRecurringTemplate implements ServerInterface.DeleteRecurringTemplate.
// DELETE /v1/lists/{list_id}/recurring-templates/{template_id}
func (h *TodoHandler) DeleteRecurringTemplate(w http.ResponseWriter, r *http.Request, listID types.UUID, templateID types.UUID) {
	// Call service layer with list ownership validation
	if err := h.todoService.DeleteRecurringTemplate(r.Context(), listID.String(), templateID.String()); err != nil {
		slog.ErrorContext(r.Context(), "failed to delete recurring template via HTTP",
			"list_id", listID.String(),
			"template_id", templateID.String(),
			"error", err)
		response.FromDomainError(w, r, err)
		return
	}

	slog.InfoContext(r.Context(), "recurring template deleted via HTTP",
		"template_id", templateID.String(),
		"list_id", listID.String())

	// Return success response (204 No Content)
	response.NoContent(w)
}

// ListRecurringTemplates implements ServerInterface.ListRecurringTemplates.
// GET /v1/lists/{list_id}/recurring-templates
func (h *TodoHandler) ListRecurringTemplates(w http.ResponseWriter, r *http.Request, listID types.UUID, params openapi.ListRecurringTemplatesParams) {
	// Determine if we should filter by active status
	activeOnly := false
	if params.ActiveOnly != nil {
		activeOnly = *params.ActiveOnly
	}

	// Call service layer
	templates, err := h.todoService.ListRecurringTemplates(r.Context(), listID.String(), activeOnly)
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to list recurring templates via HTTP",
			"list_id", listID.String(),
			"active_only", activeOnly,
			"error", err)
		response.FromDomainError(w, r, err)
		return
	}

	// Map domain models to DTOs
	templateDTOs := make([]openapi.RecurringItemTemplate, len(templates))
	for i, template := range templates {
		templateDTOs[i] = MapTemplateToDTO(template)
	}

	// Return success response
	response.OK(w, openapi.ListRecurringTemplatesResponse{
		Templates: &templateDTOs,
	})
}
