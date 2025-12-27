package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/rezkam/mono/internal/application/auth"
	mw "github.com/rezkam/mono/internal/infrastructure/http/middleware"
)

// Default configuration values for the HTTP server.
const (
	DefaultHost              = ""     // Empty means all interfaces (0.0.0.0)
	DefaultPort              = "8081"
	DefaultReadTimeout       = 15 * time.Second
	DefaultWriteTimeout      = 15 * time.Second
	DefaultIdleTimeout       = 60 * time.Second
	DefaultReadHeaderTimeout = 5 * time.Second
	DefaultMaxHeaderBytes    = 1 << 20 // 1MB
	DefaultMaxBodyBytes      = 1 << 20 // 1MB
)

// ServerConfig holds configuration for the HTTP server and router.
type ServerConfig struct {
	Host              string
	Port              string
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	MaxHeaderBytes    int
	MaxBodyBytes      int64
}

// applyDefaults sets default values for any unset (zero) fields.
func (cfg *ServerConfig) applyDefaults() {
	if cfg.Port == "" {
		cfg.Port = DefaultPort
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = DefaultReadTimeout
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = DefaultWriteTimeout
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}
	if cfg.ReadHeaderTimeout <= 0 {
		cfg.ReadHeaderTimeout = DefaultReadHeaderTimeout
	}
	if cfg.MaxHeaderBytes <= 0 {
		cfg.MaxHeaderBytes = DefaultMaxHeaderBytes
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = DefaultMaxBodyBytes
	}
}

// APIServer wraps the HTTP server with router and all HTTP concerns.
type APIServer struct {
	server *http.Server
}

// NewAPIServer creates a new HTTP server with router, middleware, and all HTTP concerns configured.
// The apiHandler is mounted under /api with authentication.
// Applies defaults for zero or invalid config values.
func NewAPIServer(apiHandler http.Handler, authenticator *auth.Authenticator, cfg ServerConfig) *APIServer {
	cfg.applyDefaults()

	router := setupRouter(apiHandler, authenticator, cfg)
	httpServer := setupHTTPServer(router, cfg)

	return &APIServer{
		server: httpServer,
	}
}

// setupRouter creates and configures the Chi router with all middleware and routes.
func setupRouter(apiHandler http.Handler, authenticator *auth.Authenticator, cfg ServerConfig) *chi.Mux {
	r := chi.NewRouter()

	// Global middlewares (applied to all routes)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(mw.MaxBodyBytes(cfg.MaxBodyBytes))

	// Health check endpoint (no auth required)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			slog.ErrorContext(r.Context(), "Failed to write health check response", "error", err)
		}
	})

	// API routes with authentication
	r.Route("/api", func(r chi.Router) {
		authMiddleware := mw.NewAuth(authenticator)
		r.Use(authMiddleware.Validate)

		// Mount the provided API handler
		r.Mount("/", apiHandler)
	})

	return r
}

// setupHTTPServer creates the net/http.Server with the given router and config.
func setupHTTPServer(router *chi.Mux, cfg ServerConfig) *http.Server {
	return &http.Server{
		Addr:              cfg.Host + ":" + cfg.Port,
		Handler:           router,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
	}
}

// Start starts the HTTP server.
func (s *APIServer) Start() error {
	slog.Info("Starting HTTP server", "addr", s.server.Addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server.
// The provided context controls the timeout for outstanding requests.
func (s *APIServer) Shutdown(ctx context.Context) error {
	slog.Info("Shutting down HTTP server")
	return s.server.Shutdown(ctx)
}

// Handler returns the underlying HTTP handler (router) for testing purposes.
func (s *APIServer) Handler() http.Handler {
	return s.server.Handler
}
