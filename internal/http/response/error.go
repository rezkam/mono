package response

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/rezkam/mono/internal/domain"
)

// ErrorResponse is the standard error response format.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error information.
type ErrorDetail struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Details []ErrorField `json:"details,omitempty"`
}

// ErrorField describes a field-specific error.
type ErrorField struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

// BadRequest sends a 400 Bad Request error.
func BadRequest(w http.ResponseWriter, message string) {
	Error(w, "INVALID_REQUEST", message, http.StatusBadRequest)
}

// ValidationError sends a 400 validation error with field details.
func ValidationError(w http.ResponseWriter, field, issue string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{
			Code:    "VALIDATION_ERROR",
			Message: "validation failed",
			Details: []ErrorField{
				{Field: field, Issue: issue},
			},
		},
	})
}

// NotFound sends a 404 Not Found error.
func NotFound(w http.ResponseWriter, resource string) {
	Error(w, "NOT_FOUND", resource+" not found", http.StatusNotFound)
}

// Unauthorized sends a 401 Unauthorized error.
func Unauthorized(w http.ResponseWriter, message string) {
	Error(w, "UNAUTHORIZED", message, http.StatusUnauthorized)
}

// InternalError sends a 500 Internal Server Error.
func InternalError(w http.ResponseWriter, err error) {
	// TODO: Log the actual error for debugging
	// log.Error().Err(err).Msg("internal error")

	// Return generic message to client
	Error(w, "INTERNAL_ERROR", "an internal error occurred", http.StatusInternalServerError)
}

// Error sends a generic error response.
func Error(w http.ResponseWriter, code, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// FromDomainError maps domain errors to HTTP responses.
func FromDomainError(w http.ResponseWriter, err error) {
	switch {
	// Validation errors (400)
	case errors.Is(err, domain.ErrTitleRequired):
		ValidationError(w, "title", "required field missing")
	case errors.Is(err, domain.ErrTitleTooLong):
		ValidationError(w, "title", "must be 255 characters or less")
	case errors.Is(err, domain.ErrInvalidID):
		ValidationError(w, "id", "invalid ID format")
	case errors.Is(err, domain.ErrInvalidTaskStatus):
		ValidationError(w, "status", "invalid task status")
	case errors.Is(err, domain.ErrInvalidTaskPriority):
		ValidationError(w, "priority", "invalid priority level")
	case errors.Is(err, domain.ErrInvalidRecurrencePattern):
		ValidationError(w, "recurrence_pattern", "invalid recurrence pattern")
	case errors.Is(err, domain.ErrRecurringTaskRequiresTemplate):
		ValidationError(w, "recurring_template_id", "required for recurring tasks")
	case errors.Is(err, domain.ErrInvalidGenerationWindow):
		ValidationError(w, "generation_window_days", "must be between 1 and 365")

	// Not found errors (404)
	case errors.Is(err, domain.ErrListNotFound):
		NotFound(w, "list")
	case errors.Is(err, domain.ErrItemNotFound):
		NotFound(w, "item")
	case errors.Is(err, domain.ErrTemplateNotFound):
		NotFound(w, "recurring template")
	case errors.Is(err, domain.ErrNotFound):
		NotFound(w, "resource")

	// Auth errors (401)
	case errors.Is(err, domain.ErrUnauthorized):
		Unauthorized(w, "invalid or missing API key")

	// Unknown errors (500)
	default:
		InternalError(w, err)
	}
}
