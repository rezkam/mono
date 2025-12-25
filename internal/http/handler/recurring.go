package handler

import (
	"encoding/json"
	"net/http"

	"github.com/oapi-codegen/runtime/types"

	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/http/openapi"
	"github.com/rezkam/mono/internal/http/response"
)

// CreateRecurringTemplate implements ServerInterface.CreateRecurringTemplate.
// POST /v1/lists/{list_id}/recurring-templates
func (s *Server) CreateRecurringTemplate(w http.ResponseWriter, r *http.Request, listID types.UUID) {
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
		duration, err := domain.ParseDuration(*req.EstimatedDuration)
		if err != nil {
			response.FromDomainError(w, r, err)
			return
		}
		template.EstimatedDuration = &duration
	}

	if req.DueOffset != nil {
		duration, err := domain.ParseDuration(*req.DueOffset)
		if err != nil {
			response.FromDomainError(w, r, err)
			return
		}
		template.DueOffset = &duration
	}

	if req.GenerationWindowDays != nil {
		template.GenerationWindowDays = *req.GenerationWindowDays
	}

	// RecurrenceConfig is JSON string, parse it to map
	// Default to empty object if not provided (database requires NOT NULL)
	if req.RecurrenceConfig != nil && *req.RecurrenceConfig != "" {
		var config map[string]interface{}
		if err := json.Unmarshal([]byte(*req.RecurrenceConfig), &config); err != nil {
			response.BadRequest(w, "invalid recurrence_config JSON")
			return
		}
		template.RecurrenceConfig = config
	} else {
		// Set default empty config
		template.RecurrenceConfig = make(map[string]interface{})
	}

	// Call service layer (validation happens here)
	created, err := s.todoService.CreateRecurringTemplate(r.Context(), template)
	if err != nil {
		response.FromDomainError(w, r, err)
		return
	}

	// Map domain model to DTO
	templateDTO := MapTemplateToDTO(created)

	// Return success response
	response.Created(w, openapi.CreateRecurringTemplateResponse{
		Template: &templateDTO,
	})
}

// GetRecurringTemplate implements ServerInterface.GetRecurringTemplate.
// GET /v1/lists/{list_id}/recurring-templates/{template_id}
func (s *Server) GetRecurringTemplate(w http.ResponseWriter, r *http.Request, listID types.UUID, templateID types.UUID) {
	// Call service layer
	// Note: listID is available for validation if needed in the future
	template, err := s.todoService.GetRecurringTemplate(r.Context(), templateID.String())
	if err != nil {
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
func (s *Server) UpdateRecurringTemplate(w http.ResponseWriter, r *http.Request, listID types.UUID, templateID types.UUID) {
	// Parse request body
	var req openapi.UpdateRecurringTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid JSON")
		return
	}

	if req.Template == nil {
		response.BadRequest(w, "template is required")
		return
	}

	// Build UpdateRecurringTemplateParams
	// Note: listID is available for validation if needed in the future
	params := domain.UpdateRecurringTemplateParams{
		TemplateID: templateID.String(),
	}

	// Determine update mask
	if req.UpdateMask == nil || len(*req.UpdateMask) == 0 {
		// No mask specified - update all provided fields
		params.UpdateMask = []string{}
		if req.Template.Title != nil {
			params.UpdateMask = append(params.UpdateMask, "title")
		}
		if req.Template.Tags != nil {
			params.UpdateMask = append(params.UpdateMask, "tags")
		}
		if req.Template.Priority != nil {
			params.UpdateMask = append(params.UpdateMask, "priority")
		}
		if req.Template.EstimatedDuration != nil {
			params.UpdateMask = append(params.UpdateMask, "estimated_duration")
		}
		if req.Template.DueOffset != nil {
			params.UpdateMask = append(params.UpdateMask, "due_offset")
		}
		if req.Template.RecurrencePattern != nil {
			params.UpdateMask = append(params.UpdateMask, "recurrence_pattern")
		}
		if req.Template.RecurrenceConfig != nil {
			params.UpdateMask = append(params.UpdateMask, "recurrence_config")
		}
		if req.Template.IsActive != nil {
			params.UpdateMask = append(params.UpdateMask, "is_active")
		}
		if req.Template.GenerationWindowDays != nil {
			params.UpdateMask = append(params.UpdateMask, "generation_window_days")
		}
	} else {
		params.UpdateMask = *req.UpdateMask
	}

	// Map field values from request to params
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
				duration, err := domain.ParseDuration(*req.Template.EstimatedDuration)
				if err != nil {
					response.FromDomainError(w, r, err)
					return
				}
				params.EstimatedDuration = &duration
			}
		case "due_offset":
			if req.Template.DueOffset != nil {
				duration, err := domain.ParseDuration(*req.Template.DueOffset)
				if err != nil {
					response.FromDomainError(w, r, err)
					return
				}
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
				var config map[string]interface{}
				if err := json.Unmarshal([]byte(*req.Template.RecurrenceConfig), &config); err != nil {
					response.BadRequest(w, "invalid recurrence_config JSON")
					return
				}
				params.RecurrenceConfig = config
			}
		case "is_active":
			params.IsActive = req.Template.IsActive
		case "generation_window_days":
			params.GenerationWindowDays = req.Template.GenerationWindowDays
		}
	}

	// Call service layer (validation happens there)
	updated, err := s.todoService.UpdateRecurringTemplate(r.Context(), params)
	if err != nil {
		response.FromDomainError(w, r, err)
		return
	}

	// Map domain model to DTO
	templateDTO := MapTemplateToDTO(updated)

	// Return success response
	response.OK(w, openapi.UpdateRecurringTemplateResponse{
		Template: &templateDTO,
	})
}

// DeleteRecurringTemplate implements ServerInterface.DeleteRecurringTemplate.
// DELETE /v1/lists/{list_id}/recurring-templates/{template_id}
func (s *Server) DeleteRecurringTemplate(w http.ResponseWriter, r *http.Request, listID types.UUID, templateID types.UUID) {
	// Call service layer
	// Note: listID is available for validation if needed in the future
	if err := s.todoService.DeleteRecurringTemplate(r.Context(), templateID.String()); err != nil {
		response.FromDomainError(w, r, err)
		return
	}

	// Return success response (204 No Content)
	response.NoContent(w)
}

// ListRecurringTemplates implements ServerInterface.ListRecurringTemplates.
// GET /v1/lists/{list_id}/recurring-templates
func (s *Server) ListRecurringTemplates(w http.ResponseWriter, r *http.Request, listID types.UUID, params openapi.ListRecurringTemplatesParams) {
	// Determine if we should filter by active status
	activeOnly := false
	if params.ActiveOnly != nil {
		activeOnly = *params.ActiveOnly
	}

	// Call service layer
	templates, err := s.todoService.ListRecurringTemplates(r.Context(), listID.String(), activeOnly)
	if err != nil {
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
