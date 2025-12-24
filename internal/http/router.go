package http

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/rezkam/mono/internal/http/handler"
	mw "github.com/rezkam/mono/internal/http/middleware"
	"github.com/rezkam/mono/internal/http/openapi"
)

// NewRouter creates and configures the Chi router with all middleware and routes.
func NewRouter(server *handler.Server, authMiddleware *mw.Auth) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health check endpoint (no auth required)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			slog.ErrorContext(r.Context(), "Failed to write health check response", "error", err)
		}
	})

	// API routes (OpenAPI spec already includes /v1 prefix in paths)
	r.Route("/api", func(r chi.Router) {
		// Auth middleware for all API routes
		r.Use(authMiddleware.Validate)

		// Mount OpenAPI-generated routes
		// This connects the ServerInterface implementation (server) to Chi routes
		openapi.HandlerFromMux(server, r)
	})

	return r
}
