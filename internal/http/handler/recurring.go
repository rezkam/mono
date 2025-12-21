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
	template := &domain.RecurringTemplate{
		ListID:            listID.String(),
		Title:             req.Title,
		Tags:              []string{},
		RecurrencePattern: domain.RecurrencePattern(req.RecurrencePattern),
	}

	if req.Tags != nil {
		template.Tags = *req.Tags
	}

	if req.Priority != nil {
		priority := domain.TaskPriority(*req.Priority)
		template.Priority = &priority
	}

	if req.EstimatedDuration != nil {
		duration := parseDuration(*req.EstimatedDuration)
		template.EstimatedDuration = &duration
	}

	if req.DueOffset != nil {
		duration := parseDuration(*req.DueOffset)
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
		response.FromDomainError(w, err)
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
// GET /v1/recurring-templates/{id}
func (s *Server) GetRecurringTemplate(w http.ResponseWriter, r *http.Request, id types.UUID) {
	// Call service layer
	template, err := s.todoService.GetRecurringTemplate(r.Context(), id.String())
	if err != nil {
		response.FromDomainError(w, err)
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
// PATCH /v1/recurring-templates/{id}
func (s *Server) UpdateRecurringTemplate(w http.ResponseWriter, r *http.Request, id types.UUID) {
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

	// Get existing template
	existing, err := s.todoService.GetRecurringTemplate(r.Context(), id.String())
	if err != nil {
		response.FromDomainError(w, err)
		return
	}

	// Apply updates based on update_mask (if provided)
	if req.UpdateMask == nil || len(*req.UpdateMask) == 0 {
		// Update all fields if no mask provided
		if req.Template.Title != nil {
			existing.Title = *req.Template.Title
		}
		if req.Template.Tags != nil {
			existing.Tags = *req.Template.Tags
		}
		if req.Template.Priority != nil {
			priority := domain.TaskPriority(*req.Template.Priority)
			existing.Priority = &priority
		}
		if req.Template.EstimatedDuration != nil {
			duration := parseDuration(*req.Template.EstimatedDuration)
			existing.EstimatedDuration = &duration
		}
		if req.Template.DueOffset != nil {
			duration := parseDuration(*req.Template.DueOffset)
			existing.DueOffset = &duration
		}
		if req.Template.RecurrencePattern != nil {
			existing.RecurrencePattern = domain.RecurrencePattern(*req.Template.RecurrencePattern)
		}
		if req.Template.RecurrenceConfig != nil && *req.Template.RecurrenceConfig != "" {
			var config map[string]interface{}
			if err := json.Unmarshal([]byte(*req.Template.RecurrenceConfig), &config); err != nil {
				response.BadRequest(w, "invalid recurrence_config JSON")
				return
			}
			existing.RecurrenceConfig = config
		}
		if req.Template.IsActive != nil {
			existing.IsActive = *req.Template.IsActive
		}
	} else {
		// Update only specified fields
		for _, field := range *req.UpdateMask {
			switch field {
			case "title":
				if req.Template.Title != nil {
					existing.Title = *req.Template.Title
				}
			case "tags":
				if req.Template.Tags != nil {
					existing.Tags = *req.Template.Tags
				}
			case "priority":
				if req.Template.Priority != nil {
					priority := domain.TaskPriority(*req.Template.Priority)
					existing.Priority = &priority
				}
			case "estimated_duration":
				if req.Template.EstimatedDuration != nil {
					duration := parseDuration(*req.Template.EstimatedDuration)
					existing.EstimatedDuration = &duration
				}
			case "due_offset":
				if req.Template.DueOffset != nil {
					duration := parseDuration(*req.Template.DueOffset)
					existing.DueOffset = &duration
				}
			case "recurrence_pattern":
				if req.Template.RecurrencePattern != nil {
					existing.RecurrencePattern = domain.RecurrencePattern(*req.Template.RecurrencePattern)
				}
			case "recurrence_config":
				if req.Template.RecurrenceConfig != nil && *req.Template.RecurrenceConfig != "" {
					var config map[string]interface{}
					if err := json.Unmarshal([]byte(*req.Template.RecurrenceConfig), &config); err != nil {
						response.BadRequest(w, "invalid recurrence_config JSON")
						return
					}
					existing.RecurrenceConfig = config
				}
			case "is_active":
				if req.Template.IsActive != nil {
					existing.IsActive = *req.Template.IsActive
				}
			}
		}
	}

	// Call service layer (validation happens here)
	if err := s.todoService.UpdateRecurringTemplate(r.Context(), existing); err != nil {
		response.FromDomainError(w, err)
		return
	}

	// Map domain model to DTO
	templateDTO := MapTemplateToDTO(existing)

	// Return success response
	response.OK(w, openapi.UpdateRecurringTemplateResponse{
		Template: &templateDTO,
	})
}

// DeleteRecurringTemplate implements ServerInterface.DeleteRecurringTemplate.
// DELETE /v1/recurring-templates/{id}
func (s *Server) DeleteRecurringTemplate(w http.ResponseWriter, r *http.Request, id types.UUID) {
	// Call service layer
	if err := s.todoService.DeleteRecurringTemplate(r.Context(), id.String()); err != nil {
		response.FromDomainError(w, err)
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
		response.FromDomainError(w, err)
		return
	}

	// Map domain models to DTOs
	templateDTOs := make([]openapi.RecurringTaskTemplate, len(templates))
	for i, template := range templates {
		templateDTOs[i] = MapTemplateToDTO(template)
	}

	// Return success response
	response.OK(w, openapi.ListRecurringTemplatesResponse{
		Templates: &templateDTOs,
	})
}
