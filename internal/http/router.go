package http

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/http/handler"
	mw "github.com/rezkam/mono/internal/http/middleware"
	"github.com/rezkam/mono/internal/http/openapi"
)

const (
	// DefaultMaxBodyBytes is the default maximum request body size (1MB).
	// Prevents clients from accidentally or maliciously sending large requests.
	DefaultMaxBodyBytes = 1 << 20 // 1MB
)

// Config holds configuration for the HTTP router.
type Config struct {
	MaxBodyBytes int64
}

// NewRouter creates and configures the Chi router with all middleware and routes.
// Applies defaults for zero or invalid config values.
func NewRouter(server *handler.Server, authenticator *auth.Authenticator, config Config) *chi.Mux {
	// Apply defaults
	if config.MaxBodyBytes <= 0 {
		config.MaxBodyBytes = DefaultMaxBodyBytes
	}

	r := chi.NewRouter()

	// Global middlewares (applied to all routes)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(mw.MaxBodyBytes(config.MaxBodyBytes))

	// Health check endpoint (no auth required)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			slog.ErrorContext(r.Context(), "Failed to write health check response", "error", err)
		}
	})

	// Get embedded OpenAPI spec for request validation
	spec, err := openapi.GetSwagger()
	if err != nil {
		// This should never happen with embedded spec, but log and continue without validation
		slog.Error("Failed to load OpenAPI spec for validation", "error", err)
	}

	// Create OpenAPI validation middleware
	var validatorMw func(http.Handler) http.Handler
	if spec != nil {
		validatorMw = mw.NewValidator(spec, mw.ValidationConfig{MultiError: true})
	}

	// API routes (OpenAPI spec already includes /v1 prefix in paths)
	r.Route("/api", func(r chi.Router) {
		// 1. OpenAPI request validation (rejects invalid requests early)
		if validatorMw != nil {
			r.Use(validatorMw)
		}

		// 2. Auth middleware for all API routes
		authMiddleware := mw.NewAuth(authenticator)
		r.Use(authMiddleware.Validate)

		// Mount OpenAPI-generated routes
		// This connects the ServerInterface implementation (server) to Chi routes
		openapi.HandlerFromMux(server, r)
	})

	return r
}
