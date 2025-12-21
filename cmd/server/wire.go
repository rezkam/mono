//go:build wireinject
// +build wireinject

package main

import (
	"context"
	"io"
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
	wire.Bind(new(io.Closer), new(*postgres.Store)),
)

// provideStore creates a postgres.Store from StorageConfig.
func provideStore(ctx context.Context, cfg *config.StorageConfig) (*postgres.Store, error) {
	return postgres.NewPostgresStore(ctx, cfg.StorageDSN)
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

// AuthSet provides authentication components.
var AuthSet = wire.NewSet(
	// Provide operation timeout for authenticator
	wire.Value(time.Duration(5*time.Second)),
	auth.NewAuthenticator,
	wire.Bind(new(shutdowner), new(*auth.Authenticator)),
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
