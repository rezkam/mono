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
	httpServer "github.com/rezkam/mono/internal/infrastructure/http"
	"github.com/rezkam/mono/internal/infrastructure/http/handler"
	"github.com/rezkam/mono/internal/infrastructure/observability"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
)

// Default values for optional configuration.
const (
	defaultShutdownTimeout = 5 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to run: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration from environment
	cfg, err := config.LoadServerConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create main application context that cancels on SIGTERM/SIGINT
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Initialize observability FIRST - before anything that might fail and need logging
	otelCfg := observability.Config{
		Enabled:     cfg.Observability.OTelEnabled,
		ServiceName: cfg.Observability.ServiceName,
	}
	if otelCfg.ServiceName == "" {
		otelCfg.ServiceName = observability.DefaultServiceName
	}
	logger, otelCleanup, err := initObservability(ctx, otelCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize observability: %w", err)
	}
	defer otelCleanup(shutdownTimeout(cfg.ShutdownTimeout))

	// Set as default so slog.Info(), slog.Error() etc. work globally without passing logger
	slog.SetDefault(logger)

	// Initialize database
	store, err := postgres.NewStoreWithConfig(ctx, toDBConfig(cfg.Database))
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
	server, cleanup, err := initializeAPIServer(store, cfg)
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
		slog.Info("Shutdown signal received")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout(cfg.ShutdownTimeout))
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown error: %w", err)
		}

		slog.Info("Server stopped gracefully")
		return nil

	case err := <-serverErr:
		return err
	}
}

// initializeAPIServer wires application services and HTTP components.
func initializeAPIServer(store *postgres.Store, cfg *config.ServerConfig) (*httpServer.APIServer, func(), error) {
	// Initialize todo service
	todoService := todo.NewService(store, todo.Config{
		DefaultPageSize: cfg.Todo.DefaultPageSize,
		MaxPageSize:     cfg.Todo.MaxPageSize,
	})

	// Initialize authenticator
	authenticator := auth.NewAuthenticator(store, auth.Config{
		OperationTimeout: cfg.Auth.OperationTimeout,
		UpdateQueueSize:  cfg.Auth.UpdateQueueSize,
	})

	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout(cfg.ShutdownTimeout))
		defer cancel()

		if err := authenticator.Shutdown(shutdownCtx); err != nil {
			slog.Warn("Authenticator shutdown error", "error", err)
		}
	}

	// Build API handler with OpenAPI routing and validation
	apiHandler, err := handler.NewOpenAPIRouter(todoService)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create API handler: %w", err)
	}

	// Create server with HTTP configuration
	server := httpServer.NewAPIServer(apiHandler, authenticator, toHTTPConfig(cfg.HTTP))

	return server, cleanup, nil
}

// initObservability initializes OpenTelemetry providers and returns cleanup function.
func initObservability(ctx context.Context, cfg observability.Config) (*slog.Logger, func(time.Duration), error) {
	tracerProvider, err := observability.InitTracerProvider(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init tracer provider: %w", err)
	}

	meterProvider, err := observability.InitMeterProvider(ctx, cfg)
	if err != nil {
		_ = tracerProvider.Shutdown(ctx)
		return nil, nil, fmt.Errorf("failed to init meter provider: %w", err)
	}

	loggerProvider, logger, err := observability.InitLogger(ctx, cfg)
	if err != nil {
		_ = meterProvider.Shutdown(ctx)
		_ = tracerProvider.Shutdown(ctx)
		return nil, nil, fmt.Errorf("failed to init logger: %w", err)
	}

	cleanup := func(timeout time.Duration) {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
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

// shutdownTimeout returns the configured timeout or default.
func shutdownTimeout(configured time.Duration) time.Duration {
	if configured > 0 {
		return configured
	}
	return defaultShutdownTimeout
}

// toDBConfig converts config.DatabaseConfig to postgres.DBConfig.
func toDBConfig(cfg config.DatabaseConfig) postgres.DBConfig {
	return postgres.DBConfig{
		DSN:             cfg.DSN,
		MaxOpenConns:    cfg.MaxOpenConns,
		MaxIdleConns:    cfg.MaxIdleConns,
		ConnMaxLifetime: time.Duration(cfg.ConnMaxLifetime) * time.Second,
		ConnMaxIdleTime: time.Duration(cfg.ConnMaxIdleTime) * time.Second,
	}
}

// toHTTPConfig converts config.HTTPConfig to httpServer.ServerConfig.
func toHTTPConfig(cfg config.HTTPConfig) httpServer.ServerConfig {
	return httpServer.ServerConfig{
		Host:              cfg.Host,
		Port:              cfg.Port,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
		MaxBodyBytes:      cfg.MaxBodyBytes,
	}
}
