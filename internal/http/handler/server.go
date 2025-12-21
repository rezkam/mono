package handler

import (
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/http/openapi"
)

// Server implements the generated ServerInterface from OpenAPI.
type Server struct {
	todoService *todo.Service
}

// NewServer creates a new HTTP handler server.
func NewServer(todoService *todo.Service) *Server {
	return &Server{
		todoService: todoService,
	}
}

// Ensure Server implements ServerInterface at compile time.
var _ openapi.ServerInterface = (*Server)(nil)
