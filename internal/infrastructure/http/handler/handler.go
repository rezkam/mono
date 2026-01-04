package handler

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/application/worker"
	mw "github.com/rezkam/mono/internal/infrastructure/http/middleware"
	"github.com/rezkam/mono/internal/infrastructure/http/openapi"
)

// TodoHandler implements the generated ServerInterface from OpenAPI.
// It adapts HTTP requests to application service calls.
type TodoHandler struct {
	todoService *todo.Service
	coordinator worker.GenerationCoordinator
}

// NewTodoHandler creates a new HTTP API handler.
func NewTodoHandler(todoService *todo.Service, coordinator worker.GenerationCoordinator) *TodoHandler {
	return &TodoHandler{
		todoService: todoService,
		coordinator: coordinator,
	}
}

// NewOpenAPIRouter creates an HTTP handler with OpenAPI validation and route mounting.
// This router includes request validation against the OpenAPI spec and mounts all API routes.
// Both production code and tests should use this function to ensure identical behavior.
func NewOpenAPIRouter(todoService *todo.Service, coordinator worker.GenerationCoordinator) (http.Handler, error) {
	// Create handler that implements OpenAPI ServerInterface
	h := NewTodoHandler(todoService, coordinator)

	// Get embedded OpenAPI spec for request validation
	spec, err := openapi.GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI spec: %w", err)
	}

	// Build API router with validation middleware
	apiRouter := chi.NewRouter()
	if spec != nil {
		validatorMw := mw.NewValidator(spec, mw.ValidationConfig{MultiError: true})
		apiRouter.Use(validatorMw)
	}

	// Mount OpenAPI-generated routes
	openapi.HandlerFromMux(h, apiRouter)

	return apiRouter, nil
}

// Ensure TodoHandler implements ServerInterface at compile time.
var _ openapi.ServerInterface = (*TodoHandler)(nil)
