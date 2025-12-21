package main

import (
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rezkam/mono/internal/config"
)

// provideHTTPServerConfig reads HTTP server config from environment.
func provideHTTPServerConfig() HTTPServerConfig {
	port, _ := config.GetEnv[string]("MONO_HTTP_PORT")
	readTimeoutSec, _ := config.GetEnv[int]("MONO_HTTP_READ_TIMEOUT_SEC")
	writeTimeoutSec, _ := config.GetEnv[int]("MONO_HTTP_WRITE_TIMEOUT_SEC")
	idleTimeoutSec, _ := config.GetEnv[int]("MONO_HTTP_IDLE_TIMEOUT_SEC")
	readHeaderTimeoutSec, _ := config.GetEnv[int]("MONO_HTTP_READ_HEADER_TIMEOUT_SEC")
	shutdownTimeoutSec, _ := config.GetEnv[int]("MONO_HTTP_SHUTDOWN_TIMEOUT_SEC")
	maxHeaderBytes, _ := config.GetEnv[int]("MONO_HTTP_MAX_HEADER_BYTES")
	maxBodyBytes, _ := config.GetEnv[int]("MONO_HTTP_MAX_BODY_BYTES")

	return HTTPServerConfig{
		Port:              port,
		ReadTimeout:       time.Duration(readTimeoutSec) * time.Second,
		WriteTimeout:      time.Duration(writeTimeoutSec) * time.Second,
		IdleTimeout:       time.Duration(idleTimeoutSec) * time.Second,
		ReadHeaderTimeout: time.Duration(readHeaderTimeoutSec) * time.Second,
		ShutdownTimeout:   time.Duration(shutdownTimeoutSec) * time.Second,
		MaxHeaderBytes:    maxHeaderBytes,
		MaxBodyBytes:      int64(maxBodyBytes),
	}
}

func provideHTTPServer(router *chi.Mux, logger *slog.Logger, cfg HTTPServerConfig) *HTTPServer {
	return NewHTTPServer(router, logger, cfg)
}
