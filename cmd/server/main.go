package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/config"
	httpRouter "github.com/rezkam/mono/internal/http"
	httpHandler "github.com/rezkam/mono/internal/http/handler"
	httpMiddleware "github.com/rezkam/mono/internal/http/middleware"
	"github.com/rezkam/mono/internal/infrastructure/observability"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to run: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Create main application context that cancels on SIGTERM/SIGINT
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Initialize observability FIRST - before anything that might fail and need logging
	otelEnabled := provideObservabilityEnabled()
	logger, otelCleanup, err := provideObservability(ctx, otelEnabled)
	if err != nil {
		return fmt.Errorf("failed to initialize observability: %w", err)
	}
	defer otelCleanup() // Cleanup LAST (after server cleanup)

	// Set as default so slog.Info(), slog.Error() etc. work globally without passing logger
	slog.SetDefault(logger)

	// Initialize database
	dbConfig := provideDBConfig()
	store, err := postgres.NewStoreWithConfig(ctx, dbConfig)
	if err != nil {
		slog.Error("failed to initialize database", "error", err)
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			slog.Error("failed to close database", "error", err)
		}
	}()

	// Initialize HTTP server
	server, cleanup, err := initializeHTTPServer(logger, store)
	if err != nil {
		slog.Error("failed to initialize server", "error", err)
		return fmt.Errorf("failed to initialize server: %w", err)
	}
	defer cleanup()

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := server.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("server error: %w", err)
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case <-ctx.Done():
		fmt.Println("\nReceived shutdown signal")

		// Graceful shutdown with configured timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), server.ShutdownTimeout())
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown error: %w", err)
		}

		fmt.Println("Server stopped gracefully")
		return nil

	case err := <-serverErr:
		return err
	}
}

// initializeHTTPServer wires application services and HTTP components.
// Returns the server, a cleanup function, and any initialization error.
func initializeHTTPServer(logger *slog.Logger, store *postgres.Store) (*HTTPServer, func(), error) {
	// Initialize todo service
	todoConfig := provideTodoConfig()
	todoService := todo.NewService(store, todoConfig)

	// Initialize authenticator
	authConfig := provideAuthConfig()
	authenticator := auth.NewAuthenticator(store, authConfig)
	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := authenticator.Shutdown(shutdownCtx); err != nil {
			slog.Warn("Authenticator shutdown error", "error", err)
		}
	}

	// Initialize HTTP handler and middleware
	handler := httpHandler.NewServer(todoService)
	authMiddleware := httpMiddleware.NewAuth(authenticator)

	// Create router
	router := httpRouter.NewRouter(handler, authMiddleware)

	// Create HTTP server
	httpServerConfig := provideHTTPServerConfig()
	httpServer := NewHTTPServer(router, logger, httpServerConfig)

	return httpServer, cleanup, nil
}

// provideDBConfig reads database config from environment.
func provideDBConfig() postgres.DBConfig {
	dsn, _ := config.GetEnv[string]("MONO_STORAGE_DSN")
	maxOpenConns, _ := config.GetEnv[int]("MONO_DB_MAX_OPEN_CONNS")
	maxIdleConns, _ := config.GetEnv[int]("MONO_DB_MAX_IDLE_CONNS")
	connMaxLifetimeSec, _ := config.GetEnv[int]("MONO_DB_CONN_MAX_LIFETIME_SEC")
	connMaxIdleTimeSec, _ := config.GetEnv[int]("MONO_DB_CONN_MAX_IDLE_TIME_SEC")

	return postgres.DBConfig{
		DSN:             dsn,
		MaxOpenConns:    maxOpenConns,
		MaxIdleConns:    maxIdleConns,
		ConnMaxLifetime: time.Duration(connMaxLifetimeSec) * time.Second,
		ConnMaxIdleTime: time.Duration(connMaxIdleTimeSec) * time.Second,
	}
}

// provideTodoConfig reads pagination config from environment.
func provideTodoConfig() todo.Config {
	defaultPageSize, _ := config.GetEnv[int]("MONO_DEFAULT_PAGE_SIZE")
	maxPageSize, _ := config.GetEnv[int]("MONO_MAX_PAGE_SIZE")

	return todo.Config{
		DefaultPageSize: defaultPageSize,
		MaxPageSize:     maxPageSize,
	}
}

// provideAuthConfig reads authenticator config from environment.
func provideAuthConfig() auth.Config {
	operationTimeoutSec, _ := config.GetEnv[int]("MONO_AUTH_OPERATION_TIMEOUT_SEC")
	updateQueueSize, _ := config.GetEnv[int]("MONO_AUTH_UPDATE_QUEUE_SIZE")

	return auth.Config{
		OperationTimeout: time.Duration(operationTimeoutSec) * time.Second,
		UpdateQueueSize:  updateQueueSize,
	}
}

// provideObservabilityEnabled reads MONO_OTEL_ENABLED from environment.
func provideObservabilityEnabled() bool {
	enabled, exists := config.GetEnv[bool]("MONO_OTEL_ENABLED")
	if !exists {
		return false
	}
	return enabled
}

// provideObservability initializes OpenTelemetry providers and returns cleanup function.
func provideObservability(ctx context.Context, enabled bool) (*slog.Logger, func(), error) {
	tracerProvider, err := observability.InitTracerProvider(ctx, enabled)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init tracer provider: %w", err)
	}

	meterProvider, err := observability.InitMeterProvider(ctx, enabled)
	if err != nil {
		_ = tracerProvider.Shutdown(ctx)
		return nil, nil, fmt.Errorf("failed to init meter provider: %w", err)
	}

	loggerProvider, logger, err := observability.InitLogger(ctx, enabled)
	if err != nil {
		_ = meterProvider.Shutdown(ctx)
		_ = tracerProvider.Shutdown(ctx)
		return nil, nil, fmt.Errorf("failed to init logger: %w", err)
	}

	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := loggerProvider.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "failed to shutdown logger provider: %v\n", err)
		}
		if err := meterProvider.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "failed to shutdown meter provider: %v\n", err)
		}
		if err := tracerProvider.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "failed to shutdown tracer provider: %v\n", err)
		}
	}

	return logger, cleanup, nil
}

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
