//go:build wireinject
// +build wireinject

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/wire"

	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/config"
	httpRouter "github.com/rezkam/mono/internal/http"
	httpHandler "github.com/rezkam/mono/internal/http/handler"
	httpMiddleware "github.com/rezkam/mono/internal/http/middleware"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
)

// ConfigSet provides configuration.
var ConfigSet = wire.NewSet(
	config.LoadServerConfig,
	// Extract specific config fields for dependencies
	wire.FieldsOf(new(*config.ServerConfig),
		"StorageConfig",
		"AuthConfig",
	),
)

// DatabaseSet provides database connection and store.
// The Store implements both todo.Repository and auth.Repository
var DatabaseSet = wire.NewSet(
	provideStore,
	wire.Bind(new(todo.Repository), new(*postgres.Store)),
	wire.Bind(new(auth.Repository), new(*postgres.Store)),
)

// provideStore creates a postgres.Store from StorageConfig.
// Returns the store and a cleanup function that closes the database connection pool.
func provideStore(ctx context.Context, cfg *config.StorageConfig) (*postgres.Store, func(), error) {
	store, err := postgres.NewPostgresStore(ctx, cfg.StorageDSN)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		if err := store.Close(); err != nil {
			// Log error but don't fail cleanup
			fmt.Fprintf(os.Stderr, "failed to close database: %v\n", err)
		}
	}

	return store, cleanup, nil
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

// ServiceSet provides application services.
var ServiceSet = wire.NewSet(
	todo.NewService,
	provideTodoConfig,
)

// provideAuthenticator creates an Authenticator and returns a cleanup function
// that gracefully shuts down the background worker.
func provideAuthenticator(ctx context.Context, repo auth.Repository, timeout time.Duration) (*auth.Authenticator, func(), error) {
	authenticator := auth.NewAuthenticator(ctx, repo, timeout)

	cleanup := func() {
		// Use a background context with timeout for shutdown
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := authenticator.Shutdown(shutdownCtx); err != nil {
			// Log error but don't fail cleanup
			fmt.Fprintf(os.Stderr, "failed to shutdown authenticator: %v\n", err)
		}
	}

	return authenticator, cleanup, nil
}

// AuthSet provides authentication components.
var AuthSet = wire.NewSet(
	// Provide operation timeout for authenticator
	wire.Value(time.Duration(5*time.Second)),
	provideAuthenticator,
)

// HTTPSet provides HTTP layer components.
var HTTPSet = wire.NewSet(
	httpHandler.NewServer,
	httpMiddleware.NewAuth,
	httpRouter.NewRouter,
)

// InitializeHTTPServer wires everything together for the HTTP server.
func InitializeHTTPServer(ctx context.Context) (*HTTPServer, func(), error) {
	wire.Build(
		ConfigSet,
		DatabaseSet,
		ServiceSet,
		AuthSet,
		HTTPSet,
		provideHTTPServer,
	)
	return nil, nil, nil
}
