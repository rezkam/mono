package handler

import (
	"encoding/json"
	"net/http"

	"github.com/oapi-codegen/runtime/types"

	"github.com/rezkam/mono/internal/http/openapi"
	"github.com/rezkam/mono/internal/http/response"
)

// CreateList implements ServerInterface.CreateList.
// POST /v1/lists
func (s *Server) CreateList(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req openapi.CreateListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid JSON")
		return
	}

	// Call service layer (validation happens here via value objects in future)
	list, err := s.todoService.CreateList(r.Context(), req.Title)
	if err != nil {
		response.FromDomainError(w, err)
		return
	}

	// Map domain model to DTO
	listDTO := MapListToDTO(list)

	// Return success response
	response.Created(w, openapi.CreateListResponse{
		List: &listDTO,
	})
}

// GetList implements ServerInterface.GetList.
// GET /v1/lists/{id}
func (s *Server) GetList(w http.ResponseWriter, r *http.Request, id types.UUID) {
	// Call service layer
	list, err := s.todoService.GetList(r.Context(), id.String())
	if err != nil {
		response.FromDomainError(w, err)
		return
	}

	// Map domain model to DTO
	listDTO := MapListToDTO(list)

	// Return success response
	response.OK(w, openapi.GetListResponse{
		List: &listDTO,
	})
}

// ListLists implements ServerInterface.ListLists.
// GET /v1/lists
func (s *Server) ListLists(w http.ResponseWriter, r *http.Request, params openapi.ListListsParams) {
	// For now, return all lists without pagination
	// TODO: Add repository method for paginated list retrieval
	lists, err := s.todoService.ListLists(r.Context())
	if err != nil {
		response.FromDomainError(w, err)
		return
	}

	// Apply pagination in-memory (temporary until repository supports it)
	pageSize := getPageSize(params.PageSize)
	offset := parsePageToken(params.PageToken)

	// Calculate slice bounds
	start := offset
	if start >= len(lists) {
		start = len(lists)
	}

	end := start + pageSize
	if end > len(lists) {
		end = len(lists)
	}

	// Slice the results
	pagedLists := lists[start:end]
	hasMore := end < len(lists)

	// Map domain models to DTOs
	listDTOs := make([]openapi.TodoList, len(pagedLists))
	for i, list := range pagedLists {
		listDTOs[i] = MapListToDTO(list)
	}

	// Generate next page token if there are more results
	nextToken := generatePageToken(end, hasMore)

	// Return success response
	response.OK(w, openapi.ListListsResponse{
		Lists:         &listDTOs,
		NextPageToken: nextToken,
	})
}
