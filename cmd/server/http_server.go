package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// Default configuration values.
const (
	DefaultHTTPPort          = "8081"
	DefaultReadTimeout       = 15 * time.Second
	DefaultWriteTimeout      = 15 * time.Second
	DefaultIdleTimeout       = 60 * time.Second
	DefaultReadHeaderTimeout = 5 * time.Second
	DefaultShutdownTimeout   = 10 * time.Second
	DefaultMaxHeaderBytes    = 1 << 20 // 1MB
	DefaultMaxBodyBytes      = 1 << 20 // 1MB
)

// HTTPServerConfig holds configuration for the HTTP server.
type HTTPServerConfig struct {
	Port              string
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
	MaxHeaderBytes    int
	MaxBodyBytes      int64
}

// HTTPServer wraps the HTTP server and its configuration.
type HTTPServer struct {
	server          *http.Server
	router          *chi.Mux
	logger          *slog.Logger
	shutdownTimeout time.Duration
}

// NewHTTPServer creates a new HTTP server with the given router, logger, and configuration.
// Applies defaults for zero or invalid config values.
func NewHTTPServer(router *chi.Mux, logger *slog.Logger, cfg HTTPServerConfig) *HTTPServer {
	// Apply defaults for zero or empty values
	if cfg.Port == "" {
		cfg.Port = DefaultHTTPPort
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
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = DefaultShutdownTimeout
	}
	if cfg.MaxHeaderBytes <= 0 {
		cfg.MaxHeaderBytes = DefaultMaxHeaderBytes
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = DefaultMaxBodyBytes
	}

	return &HTTPServer{
		router:          router,
		logger:          logger,
		shutdownTimeout: cfg.ShutdownTimeout,
		server: &http.Server{
			Addr:              ":" + cfg.Port,
			Handler:           router,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			MaxHeaderBytes:    cfg.MaxHeaderBytes,
		},
	}
}

// Start starts the HTTP server.
func (s *HTTPServer) Start() error {
	s.logger.Info("Starting HTTP server", "addr", s.server.Addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down HTTP server")
	return s.server.Shutdown(ctx)
}

// ShutdownTimeout returns the configured shutdown timeout duration.
func (s *HTTPServer) ShutdownTimeout() time.Duration {
	return s.shutdownTimeout
}
