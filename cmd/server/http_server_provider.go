package main

import (
	"context"
	"io"

	"github.com/go-chi/chi/v5"
	"github.com/rezkam/mono/internal/config"
)

// provideHTTPServerWithCleanup constructs the HTTP server alongside the cleanup
// hook so Wire can inject both results without custom wiring in main().
func provideHTTPServerWithCleanup(ctx context.Context, router *chi.Mux, cfg *config.ServerConfig, authenticator shutdowner, store io.Closer) (*HTTPServer, func(), error) {
	server := NewHTTPServer(router, cfg)
	cleanup := newCleanup(ctx, authenticator, store)
	return server, cleanup, nil
}
