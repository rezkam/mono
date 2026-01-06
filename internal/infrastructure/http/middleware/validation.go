package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

// ValidationConfig holds configuration for the OpenAPI validation middleware.
type ValidationConfig struct {
	// MultiError when true collects all validation errors instead of stopping at first.
	MultiError bool
}

// NewValidator creates OpenAPI request validation middleware.
// The middleware validates incoming requests against the OpenAPI spec,
// returning 400 Bad Request for invalid requests.
//
// Note: Authentication is handled separately by Auth middleware,
// so we skip OpenAPI security validation here.
func NewValidator(spec *openapi3.T, config ValidationConfig) func(http.Handler) http.Handler {
	// Set base path to /api without host validation
	// This matches our router mounting at /api
	spec.Servers = openapi3.Servers{
		{URL: "/api"},
	}

	opts := &nethttpmiddleware.Options{
		Options: openapi3filter.Options{
			MultiError: config.MultiError,
			// Skip authentication validation - handled by Auth middleware
			AuthenticationFunc: func(_ context.Context, _ *openapi3filter.AuthenticationInput) error {
				return nil
			},
		},
		ErrorHandlerWithOpts:  validationErrorHandler,
		SilenceServersWarning: true, // We use relative path /api, not full host
	}

	return nethttpmiddleware.OapiRequestValidatorWithOptions(spec, opts)
}

// ErrorResponse matches the OpenAPI ErrorResponse schema.
// All error responses must use this standard structure.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Details []ErrorField `json:"details"` // Always array, never null (OpenAPI spec: nullable: false)
}

type ErrorField struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

// validationErrorHandler formats validation errors as JSON responses using the standard ErrorResponse format.
// Parses OpenAPI validation errors to extract field-specific details.
func validationErrorHandler(ctx context.Context, err error, w http.ResponseWriter, r *http.Request, opts nethttpmiddleware.ErrorHandlerOpts) {
	// Parse validation error to extract field details
	details := parseValidationError(err)

	// Log validation failure with field details
	slog.WarnContext(ctx, "request validation failed",
		"path", r.URL.Path,
		"method", r.Method,
		"invalid_field_count", len(details),
		"error", err.Error())

	// Use standard ErrorResponse structure matching OpenAPI spec
	resp := ErrorResponse{
		Error: ErrorDetail{
			Code:    "VALIDATION_ERROR",
			Message: "validation failed",
			Details: details, // Populated with field-specific errors
		},
	}

	jsonBytes, encErr := json.Marshal(resp)
	if encErr != nil {
		slog.ErrorContext(ctx, "failed to marshal validation error response",
			"path", r.URL.Path,
			"method", r.Method,
			"error", encErr)
		// Fallback to pre-marshaled error
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"INTERNAL_ERROR","message":"failed to encode response","details":[]}}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(opts.StatusCode)
	_, _ = w.Write(jsonBytes)
}

// parseValidationError extracts field-specific validation details from OpenAPI errors.
// Returns an empty array if no specific fields can be extracted.
func parseValidationError(err error) []ErrorField {
	if err == nil {
		return []ErrorField{}
	}

	// Try to extract field information from error message
	// OpenAPI validation errors typically have format like:
	// - "request body has an error: doesn't match schema: Error at \"/title\": minimum string length is 1"
	// - "parameter \"page_size\" in query has an error: number must be at most 100"
	// - "request body has an error: value is required but missing"

	errMsg := err.Error()
	details := []ErrorField{}

	// Common patterns in OpenAPI validation errors
	patterns := []struct {
		marker  string
		extract func(string) *ErrorField
	}{
		{
			marker: "Error at \"/",
			extract: func(msg string) *ErrorField {
				// Extract field from: Error at "/field": issue
				start := len("Error at \"/")
				idx := strings.Index(msg, "Error at \"/")
				if idx == -1 {
					return nil
				}
				rest := msg[idx+start:]
				endQuote := strings.Index(rest, "\"")
				if endQuote == -1 {
					return nil
				}
				field := rest[:endQuote]

				// Extract issue after the colon
				colonIdx := strings.Index(rest, ":")
				if colonIdx == -1 || colonIdx+2 >= len(rest) {
					return &ErrorField{Field: field, Issue: "validation failed"}
				}
				issue := strings.TrimSpace(rest[colonIdx+1:])

				return &ErrorField{Field: field, Issue: issue}
			},
		},
		{
			marker: "parameter \"",
			extract: func(msg string) *ErrorField {
				// Extract from: parameter "field" in query has an error: issue
				start := len("parameter \"")
				idx := strings.Index(msg, "parameter \"")
				if idx == -1 {
					return nil
				}
				rest := msg[idx+start:]
				endQuote := strings.Index(rest, "\"")
				if endQuote == -1 {
					return nil
				}
				field := rest[:endQuote]

				// Extract issue after "has an error:"
				errorMarker := "has an error:"
				errorIdx := strings.Index(rest, errorMarker)
				if errorIdx == -1 {
					return &ErrorField{Field: field, Issue: "invalid parameter"}
				}
				issue := strings.TrimSpace(rest[errorIdx+len(errorMarker):])

				return &ErrorField{Field: field, Issue: issue}
			},
		},
		{
			marker: "request body",
			extract: func(msg string) *ErrorField {
				// Generic request body error without specific field
				if strings.Contains(msg, "doesn't match the schema") ||
					strings.Contains(msg, "doesn't match schema") {
					return &ErrorField{Field: "body", Issue: "request body doesn't match schema"}
				}
				if strings.Contains(msg, "required") {
					return &ErrorField{Field: "body", Issue: "required field missing"}
				}
				return &ErrorField{Field: "body", Issue: "invalid request body"}
			},
		},
	}

	// Try each pattern
	for _, p := range patterns {
		if strings.Contains(errMsg, p.marker) {
			if detail := p.extract(errMsg); detail != nil {
				details = append(details, *detail)
				break // Use first match
			}
		}
	}

	// If no pattern matched, return empty array (not nil)
	if len(details) == 0 {
		return []ErrorField{}
	}

	return details
}
