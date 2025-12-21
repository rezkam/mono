package http_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/config"
	httpRouter "github.com/rezkam/mono/internal/http"
	"github.com/rezkam/mono/internal/http/handler"
	"github.com/rezkam/mono/internal/http/middleware"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
)

// TestServer holds the test HTTP server and its dependencies.
type TestServer struct {
	Router        *chi.Mux
	Store         *postgres.Store
	TodoService   *todo.Service
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
		t.Skipf("Skipping HTTP integration test: %v (set MONO_STORAGE_DSN to run)", err)
	}

	// Create database connection
	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, cfg.StorageDSN)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	// Create services
	todoService := todo.NewService(store, todo.Config{})
	authenticator := auth.NewAuthenticator(ctx, store, 5*time.Second)

	// Create handlers and middleware
	server := handler.NewServer(todoService)
	authMiddleware := middleware.NewAuth(authenticator)

	// Create router
	router := httpRouter.NewRouter(server, authMiddleware)

	// Generate test API key
	apiKey, err := auth.CreateAPIKey(ctx, store, "sk", "test", "v1", "test-key", nil)
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	// Cleanup function
	cleanup := func() {
		_ = authenticator.Shutdown(context.Background())
		_ = store.Close()
	}

	return &TestServer{
		Router:        router,
		Store:         store,
		TodoService:   todoService,
		Authenticator: authenticator,
		APIKey:        apiKey,
		Cleanup:       cleanup,
	}
}

// NewRequest creates an httptest request with authentication header.
func (ts *TestServer) NewRequest(method, path string, body interface{}) *httptest.ResponseRecorder {
	// This is a helper for creating authenticated requests
	// Actual request creation happens in individual tests
	return httptest.NewRecorder()
}
