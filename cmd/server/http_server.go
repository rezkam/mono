package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rezkam/mono/internal/config"
)

// HTTPServer wraps the HTTP server and its configuration.
type HTTPServer struct {
	server *http.Server
	router *chi.Mux
	config *config.ServerConfig
}

// NewHTTPServer creates a new HTTP server with the given router and configuration.
func NewHTTPServer(router *chi.Mux, cfg *config.ServerConfig) *HTTPServer {
	addr := ":" + cfg.GatewayConfig.RESTPort

	return &HTTPServer{
		router: router,
		config: cfg,
		server: &http.Server{
			Addr:         addr,
			Handler:      router,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}
}

// Start starts the HTTP server.
func (s *HTTPServer) Start() error {
	fmt.Printf("Starting HTTP server on %s\n", s.server.Addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	fmt.Println("Shutting down HTTP server...")
	return s.server.Shutdown(ctx)
}
