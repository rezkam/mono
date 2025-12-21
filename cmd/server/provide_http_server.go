package main

import (
	"github.com/go-chi/chi/v5"
	"github.com/rezkam/mono/internal/config"
)

func provideHTTPServer(router *chi.Mux, cfg *config.ServerConfig) *HTTPServer {
	return NewHTTPServer(router, cfg)
}
