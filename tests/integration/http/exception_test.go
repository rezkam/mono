package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/http/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListExceptions_ReturnsExceptions(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create list and template
	list := createTestList(t, ts, "Test List")
	template := createTemplateForExceptionTest(t, ts, list.Id.String(), "Daily Task")

	// Create exception
	excID, _ := uuid.NewV7()
	occursAt := time.Now().UTC().Truncate(time.Second)
	exception := &domain.RecurringTemplateException{
		ID:            excID.String(),
		TemplateID:    template.ID,
		OccursAt:      occursAt,
		ExceptionType: domain.ExceptionTypeDeleted,
		CreatedAt:     time.Now().UTC(),
	}
	_, err := ts.Store.CreateException(ctx, exception)
	require.NoError(t, err)

	// GET request
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/lists/"+template.ListID+"/recurring-templates/"+template.ID+"/exceptions",
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp openapi.ListRecurringTemplateExceptionsResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.NotNil(t, resp.Exceptions)
	assert.Len(t, *resp.Exceptions, 1)
	assert.Equal(t, openapi.Deleted, *(*resp.Exceptions)[0].ExceptionType)
}

func TestListExceptions_WithDateRange(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()

	// Create list and template
	list := createTestList(t, ts, "Test List")
	template := createTemplateForExceptionTest(t, ts, list.Id.String(), "Daily Task")

	// Create 3 exceptions at different dates
	now := time.Now().UTC().Truncate(time.Second)
	exceptions := []*domain.RecurringTemplateException{
		{
			ID:            uuid.NewString(),
			TemplateID:    template.ID,
			OccursAt:      now.AddDate(0, 0, -10), // 10 days ago
			ExceptionType: domain.ExceptionTypeDeleted,
			CreatedAt:     time.Now().UTC(),
		},
		{
			ID:            uuid.NewString(),
			TemplateID:    template.ID,
			OccursAt:      now, // today
			ExceptionType: domain.ExceptionTypeEdited,
			CreatedAt:     time.Now().UTC(),
		},
		{
			ID:            uuid.NewString(),
			TemplateID:    template.ID,
			OccursAt:      now.AddDate(0, 0, 10), // 10 days from now
			ExceptionType: domain.ExceptionTypeRescheduled,
			CreatedAt:     time.Now().UTC(),
		},
	}

	for _, exc := range exceptions {
		_, err := ts.Store.CreateException(ctx, exc)
		require.NoError(t, err)
	}

	// Query with date range that excludes the first exception
	fromDate := now.AddDate(0, 0, -5).Format(time.RFC3339)
	toDate := now.AddDate(0, 0, 15).Format(time.RFC3339)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/lists/"+template.ListID+"/recurring-templates/"+template.ID+"/exceptions?from_date="+fromDate+"&to_date="+toDate,
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp openapi.ListRecurringTemplateExceptionsResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.NotNil(t, resp.Exceptions)
	// Should only return 2 exceptions (today and +10 days, not -10 days)
	assert.Len(t, *resp.Exceptions, 2)
}

func TestListExceptions_TemplateNotFound(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create list
	list := createTestList(t, ts, "Test List")

	// Use non-existent template ID
	nonExistentTemplateID := uuid.NewString()

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/lists/"+list.Id.String()+"/recurring-templates/"+nonExistentTemplateID+"/exceptions",
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestListExceptions_WrongList(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Create template
	list := createTestList(t, ts, "Test List")
	template := createTemplateForExceptionTest(t, ts, list.Id.String(), "Daily Task")

	// Create different list
	differentList := createTestList(t, ts, "Different List")

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/lists/"+differentList.Id.String()+"/recurring-templates/"+template.ID+"/exceptions",
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// createTemplateForExceptionTest creates a domain.RecurringTemplate for exception tests
func createTemplateForExceptionTest(t *testing.T, ts *TestServer, listID, title string) *domain.RecurringTemplate {
	t.Helper()

	templateID, _ := uuid.NewV7()
	template := &domain.RecurringTemplate{
		ID:                    templateID.String(),
		ListID:                listID,
		Title:                 title,
		RecurrencePattern:     "daily",
		RecurrenceConfig:      map[string]any{"interval": 1},
		IsActive:              true,
		GeneratedThrough:      time.Now().UTC(),
		SyncHorizonDays:       14,
		GenerationHorizonDays: 365,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
	}

	created, err := ts.Store.CreateRecurringTemplate(context.Background(), template)
	require.NoError(t, err)

	return created
}
