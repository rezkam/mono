package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

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

// validationErrorHandler formats validation errors as JSON responses.
func validationErrorHandler(_ context.Context, err error, w http.ResponseWriter, _ *http.Request, opts nethttpmiddleware.ErrorHandlerOpts) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(opts.StatusCode)

	resp := map[string]any{
		"error": map[string]any{
			"code":    "VALIDATION_ERROR",
			"message": err.Error(),
		},
	}

	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		slog.Error("failed to encode validation error response", "error", encErr)
	}
}
