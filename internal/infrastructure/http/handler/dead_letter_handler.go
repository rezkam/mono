package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/oapi-codegen/runtime/types"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/http/openapi"
	"github.com/rezkam/mono/internal/infrastructure/http/response"
)

// ListDeadLetterJobs lists pending dead letter jobs.
func (h *TodoHandler) ListDeadLetterJobs(w http.ResponseWriter, r *http.Request, params openapi.ListDeadLetterJobsParams) {
	// Default limit to 50
	limit := 50
	if params.Limit != nil {
		limit = *params.Limit
	}

	jobs, err := h.coordinator.ListDeadLetterJobs(r.Context(), limit)
	if err != nil {
		response.FromDomainError(w, r, err)
		return
	}

	response.OK(w, openapi.ListDeadLetterJobsResponse{
		Jobs: toOpenAPIDeadLetterJobs(jobs),
	})
}

// RetryDeadLetterJob retries a dead letter job.
func (h *TodoHandler) RetryDeadLetterJob(w http.ResponseWriter, r *http.Request, id types.UUID) {
	// TODO: Get reviewer ID from auth context
	reviewedBy := "admin"

	newJobID, err := h.coordinator.RetryDeadLetterJob(r.Context(), id.String(), reviewedBy)
	if err != nil {
		response.FromDomainError(w, r, err)
		return
	}

	newJobUUID, err := uuid.Parse(newJobID)
	if err != nil {
		response.FromDomainError(w, r, err)
		return
	}

	response.OK(w, openapi.RetryDeadLetterJobResponse{
		NewJobId: &newJobUUID,
	})
}

// DiscardDeadLetterJob discards a dead letter job.
func (h *TodoHandler) DiscardDeadLetterJob(w http.ResponseWriter, r *http.Request, id types.UUID) {
	var req openapi.DiscardDeadLetterJobJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.FromDomainError(w, r, domain.ErrInvalidRequest)
		return
	}

	// TODO: Get reviewer ID from auth context
	reviewedBy := "admin"
	note := ""
	if req.Note != nil {
		note = *req.Note
	}

	err := h.coordinator.DiscardDeadLetterJob(r.Context(), id.String(), reviewedBy, note)
	if err != nil {
		response.FromDomainError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func toOpenAPIDeadLetterJobs(jobs []*domain.DeadLetterJob) *[]openapi.DeadLetterJob {
	if len(jobs) == 0 {
		empty := make([]openapi.DeadLetterJob, 0)
		return &empty
	}

	result := make([]openapi.DeadLetterJob, len(jobs))
	for i, job := range jobs {
		jobID, _ := uuid.Parse(job.ID)
		templateID, _ := uuid.Parse(job.TemplateID)

		var originalJobID *types.UUID
		if job.OriginalJobID != "" {
			if parsed, err := uuid.Parse(job.OriginalJobID); err == nil {
				originalJobID = &parsed
			}
		}

		result[i] = openapi.DeadLetterJob{
			Id:            &jobID,
			TemplateId:    &templateID,
			GenerateFrom:  &job.GenerateFrom,
			GenerateUntil: &job.GenerateUntil,
			ErrorType:     &job.ErrorType,
			ErrorMessage:  &job.ErrorMessage,
			FailedAt:      &job.FailedAt,
			RetryCount:    &job.RetryCount,
			LastWorkerId:  &job.LastWorkerID,
			OriginalJobId: originalJobID,
		}
	}
	return &result
}
