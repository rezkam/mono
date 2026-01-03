package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/worker"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeadLetterAPI(t *testing.T) {
	// Setup test server using shared helper
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create a dead letter job directly via coordinator/store for testing
	ctx := context.Background()

	// Create required template first
	list, err := ts.TodoService.CreateList(ctx, "DLQ Test List")
	require.NoError(t, err)

	// Use domain.RecurrencePatternDaily
	dailyPattern, _ := domain.NewRecurrencePattern("daily")

	template := &domain.RecurringTemplate{
		ListID:            list.ID,
		Title:             "Failing Template",
		RecurrencePattern: dailyPattern,
		IsActive:          true,
		RecurrenceConfig:  map[string]any{}, // Add empty config to satisfy NOT NULL constraint
	}
	createdTemplate, err := ts.TodoService.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Create a dummy job that failed
	job := &domain.GenerationJob{
		ID:            uuid.New().String(),
		TemplateID:    createdTemplate.ID,
		GenerateFrom:  time.Now().UTC(),
		GenerateUntil: time.Now().UTC().Add(24 * time.Hour),
		ScheduledFor:  time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		RetryCount:    worker.DefaultRetryConfig().MaxRetries,
	}

	// Must insert the job first to satisfy FK constraint
	err = ts.Store.CreateGenerationJob(ctx, job)
	require.NoError(t, err)

	// Manually claim the job in DB so MoveToDeadLetter accepts it
	workerID := "worker-1"
	_, err = ts.Store.Pool().Exec(ctx, `
		UPDATE recurring_generation_jobs 
		SET status = 'running', 
			claimed_by = $1, 
			claimed_at = NOW(), 
			available_at = NOW() + INTERVAL '5 minutes'
		WHERE id = $2
	`, workerID, job.ID)
	require.NoError(t, err)

	// Update local object to match DB state (for MoveToDeadLetter validation if any)
	job.ClaimedBy = &workerID

	// Use coordinator to move to DLQ
	// Since we don't have direct access to coordinator in TestServer, create one locally using the same pool
	coordinator := postgres.NewPostgresCoordinator(ts.Store.Pool())
	err = coordinator.MoveToDeadLetter(ctx, job, workerID, "exhausted", "Test error message", nil)
	require.NoError(t, err)

	t.Run("ListDeadLetterJobs", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/dead-letter-jobs", nil)
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)
		w := httptest.NewRecorder()

		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var result struct {
			Jobs []struct {
				ID           string `json:"id"`
				TemplateID   string `json:"template_id"`
				ErrorMessage string `json:"error_message"`
				ErrorType    string `json:"error_type"`
			} `json:"jobs"`
		}

		err := json.NewDecoder(w.Body).Decode(&result)
		require.NoError(t, err)

		require.Len(t, result.Jobs, 1)
		assert.Equal(t, createdTemplate.ID, result.Jobs[0].TemplateID)
		assert.Equal(t, "Test error message", result.Jobs[0].ErrorMessage)
		assert.Equal(t, "exhausted", result.Jobs[0].ErrorType)
	})

	t.Run("RetryDeadLetterJob", func(t *testing.T) {
		// First get the job ID
		jobs, err := ts.TodoService.ListDeadLetterJobs(ctx, 10)
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		dlID := jobs[0].ID

		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/dead-letter-jobs/%s/retry", dlID), nil)
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)
		w := httptest.NewRecorder()

		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var result struct {
			NewJobID string `json:"new_job_id"`
		}
		err = json.NewDecoder(w.Body).Decode(&result)
		require.NoError(t, err)
		assert.NotEmpty(t, result.NewJobID)

		// Verify DLQ entry is resolved (should be empty now)
		updatedJobs, err := ts.TodoService.ListDeadLetterJobs(ctx, 10)
		require.NoError(t, err)
		assert.Empty(t, updatedJobs, "Should have no pending DLQ jobs")
	})

	t.Run("DiscardDeadLetterJob", func(t *testing.T) {
		// Create another DL job
		job2 := &domain.GenerationJob{
			ID:            uuid.New().String(),
			TemplateID:    createdTemplate.ID,
			GenerateFrom:  time.Now().UTC(),
			GenerateUntil: time.Now().UTC().Add(24 * time.Hour),
			ScheduledFor:  time.Now().UTC(),
			CreatedAt:     time.Now().UTC(),
			RetryCount:    worker.DefaultRetryConfig().MaxRetries,
		}

		// Setup job2 in DB so FK constraints pass
		err = ts.Store.CreateGenerationJob(ctx, job2)
		require.NoError(t, err)

		// Manually claim the job in DB so MoveToDeadLetter accepts it
		workerID := "worker-1"
		_, err = ts.Store.Pool().Exec(ctx, `
			UPDATE recurring_generation_jobs 
			SET status = 'running', 
				claimed_by = $1, 
				claimed_at = NOW(), 
				available_at = NOW() + INTERVAL '5 minutes'
			WHERE id = $2
		`, workerID, job2.ID)
		require.NoError(t, err)

		job2.ClaimedBy = &workerID

		err = coordinator.MoveToDeadLetter(ctx, job2, "worker-1", "exhausted", "Another error", nil)
		require.NoError(t, err)

		// Get its ID
		jobs, err := ts.TodoService.ListDeadLetterJobs(ctx, 10)
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		dlID := jobs[0].ID

		// Discard it
		body := bytes.NewBufferString(`{"note": "Not worth fixing"}`)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/dead-letter-jobs/%s/discard", dlID), body)
		req.Header.Set("Authorization", "Bearer "+ts.APIKey)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		ts.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)

		// Verify DLQ entry is resolved
		updatedJobs, err := ts.TodoService.ListDeadLetterJobs(ctx, 10)
		require.NoError(t, err)
		assert.Empty(t, updatedJobs, "Should have no pending DLQ jobs")
	})
}

// Helpers for request/response reading
func readBody(r *http.Response) []byte {
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	return body
}
