package handler

import (
	"encoding/json"
	"fmt"
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
	// Parse filter and order_by parameters
	var filterParams domain.ListListsParams
	var err error

	if params.Filter != nil && *params.Filter != "" {
		filterParams, err = parseListFilter(*params.Filter)
		if err != nil {
			response.BadRequest(w, fmt.Sprintf("invalid filter: %v", err))
			return
		}
	}

	// Parse order_by
	if params.OrderBy != nil && *params.OrderBy != "" {
		orderBy, orderDir, err := parseOrderBy(*params.OrderBy)
		if err != nil {
			response.BadRequest(w, fmt.Sprintf("invalid order_by: %v", err))
			return
		}
		filterParams.OrderBy = orderBy
		filterParams.OrderDir = orderDir
	} else {
		filterParams.OrderBy = "create_time"
		filterParams.OrderDir = "desc"
	}

	// Parse pagination
	pageSize := getPageSize(params.PageSize)
	offset := parsePageToken(params.PageToken)

	filterParams.Limit = pageSize
	filterParams.Offset = offset

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

	// Generate next page token if there are more results
	nextOffset := offset + pageSize
	nextToken := generatePageToken(nextOffset, result.HasMore)

	// Return success response
	response.OK(w, openapi.ListListsResponse{
		Lists:         &listDTOs,
		NextPageToken: nextToken,
	})
}
