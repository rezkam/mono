package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/config"
	httpServer "github.com/rezkam/mono/internal/infrastructure/http"
	"github.com/rezkam/mono/internal/infrastructure/http/handler"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/rezkam/mono/internal/recurring"
)

// TestServer holds the test HTTP server and its dependencies.
type TestServer struct {
	Router        http.Handler
	Store         *postgres.Store
	TodoService   *todo.Service
	Coordinator   *postgres.PostgresCoordinator
	Authenticator *auth.Authenticator
	APIKey        string
	Cleanup       func()
}

// SetupTestServer creates a test HTTP server with a real database.
// It returns a TestServer with all dependencies wired up.
func SetupTestServer(t *testing.T) *TestServer {
	t.Helper()

	// Load test configuration
	cfg, err := config.LoadTestConfig()
	if err != nil {
		t.Skipf("Skipping HTTP integration test: %v (set MONO_DB_DSN to run)", err)
	}
	dsn := cfg.Database.DSN

	// Create database connection with cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	store, err := postgres.NewPostgresStore(ctx, dsn)
	if err != nil {
		cancel()
		t.Fatalf("failed to create store: %v", err)
	}

	// Create services
	generator := recurring.NewDomainGenerator()
	todoService := todo.NewService(store, generator, todo.Config{})
	coordinator := postgres.NewPostgresCoordinator(store.Pool())
	authenticator := auth.NewAuthenticator(store, auth.Config{OperationTimeout: 5 * time.Second})

	// Create API handler with OpenAPI validation (reuses production logic)
	apiHandler, err := handler.NewOpenAPIRouter(todoService, coordinator)
	if err != nil {
		cancel()
		_ = store.Close()
		t.Fatalf("failed to create API handler: %v", err)
	}

	// Create server with default 1MB body limit for tests
	serverConfig := httpServer.ServerConfig{
		MaxBodyBytes: 1 << 20, // 1MB
	}
	server, err := httpServer.NewAPIServer(apiHandler, authenticator, serverConfig)
	if err != nil {
		cancel()
		_ = store.Close()
		t.Fatalf("failed to create HTTP server: %v", err)
	}
	router := server.Handler()

	// Generate test API key
	apiKey, err := auth.CreateAPIKey(ctx, store, "sk", "test", "v1", "test-key", nil)
	if err != nil {
		cancel()
		_ = store.Close()
		t.Fatalf("failed to create API key: %v", err)
	}

	// Cleanup function - cancel context first to signal shutdown, then wait for completion
	cleanup := func() {
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		authenticator.Shutdown(shutdownCtx)
		// Truncate tables to ensure test isolation
		_, _ = store.Pool().Exec(context.Background(), "TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
		_ = store.Close()
	}

	return &TestServer{
		Router:        router,
		Store:         store,
		TodoService:   todoService,
		Coordinator:   coordinator,
		Authenticator: authenticator,
		APIKey:        apiKey,
		Cleanup:       cleanup,
	}
}

// NewRequest creates an httptest request with authentication header.
func (ts *TestServer) NewRequest(method, path string, body any) *httptest.ResponseRecorder {
	// This is a helper for creating authenticated requests
	// Actual request creation happens in individual tests
	return httptest.NewRecorder()
}
