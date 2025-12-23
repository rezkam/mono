package handler

import (
	"encoding/json"
	"net/http"

	"github.com/oapi-codegen/runtime/types"

	"github.com/rezkam/mono/internal/domain"
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
		response.FromDomainError(w, r, err)
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
		response.FromDomainError(w, r, err)
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
	// Build domain params from query params
	offset := parsePageToken(params.PageToken)

	filterParams := domain.ListListsParams{
		Limit:  getPageSize(params.PageSize),
		Offset: offset,
	}

	// Set filter parameters if specified
	if params.TitleContains != nil {
		filterParams.TitleContains = params.TitleContains
	}
	if params.CreatedAfter != nil {
		filterParams.CreateTimeAfter = params.CreatedAfter
	}
	if params.CreatedBefore != nil {
		filterParams.CreateTimeBefore = params.CreatedBefore
	}

	// Set sorting if specified
	if params.SortBy != nil {
		filterParams.OrderBy = string(*params.SortBy)
	}
	if params.SortDir != nil {
		filterParams.OrderDir = string(*params.SortDir)
	}

	// Call service layer with filters and sorting
	result, err := s.todoService.FindLists(r.Context(), filterParams)
	if err != nil {
		response.FromDomainError(w, r, err)
		return
	}

	// Map domain models to DTOs
	listDTOs := make([]openapi.TodoList, len(result.Lists))
	for i, list := range result.Lists {
		listDTOs[i] = MapListToDTO(list)
	}

	// Generate next page token based on repository result
	nextToken := generatePageToken(offset+len(result.Lists), result.HasMore)

	// Return success response
	response.OK(w, openapi.ListListsResponse{
		Lists:         &listDTOs,
		NextPageToken: nextToken,
	})
}
