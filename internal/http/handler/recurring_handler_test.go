package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oapi-codegen/runtime/types"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	openapi "github.com/rezkam/mono/internal/http/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRepository implements todo.Repository and panics on calls we don't expect.
type stubRepository struct{}

func (s *stubRepository) CreateList(ctx context.Context, list *domain.TodoList) error {
	panic("not implemented")
}
func (s *stubRepository) FindListByID(ctx context.Context, id string) (*domain.TodoList, error) {
	panic("not implemented")
}
func (s *stubRepository) FindAllLists(ctx context.Context) ([]*domain.TodoList, error) {
	panic("not implemented")
}
func (s *stubRepository) FindLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error) {
	panic("not implemented")
}
func (s *stubRepository) UpdateList(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error) {
	panic("not implemented")
}
func (s *stubRepository) CreateItem(ctx context.Context, listID string, item *domain.TodoItem) error {
	panic("not implemented")
}
func (s *stubRepository) FindItemByID(ctx context.Context, id string) (*domain.TodoItem, error) {
	panic("not implemented")
}
func (s *stubRepository) UpdateItem(ctx context.Context, params domain.UpdateItemParams) (*domain.TodoItem, error) {
	panic("not implemented")
}
func (s *stubRepository) FindItems(ctx context.Context, params domain.ListTasksParams, excludedStatuses []domain.TaskStatus) (*domain.PagedResult, error) {
	panic("not implemented")
}
func (s *stubRepository) CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) error {
	panic("not implemented")
}
func (s *stubRepository) FindRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	panic("should not be called")
}
func (s *stubRepository) UpdateRecurringTemplate(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error) {
	panic("not implemented")
}
func (s *stubRepository) DeleteRecurringTemplate(ctx context.Context, id string) error {
	panic("not implemented")
}
func (s *stubRepository) FindRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*domain.RecurringTemplate, error) {
	panic("not implemented")
}

func TestUpdateRecurringTemplate_MissingTemplateReturnsBadRequest(t *testing.T) {
	repo := &stubRepository{}
	service := todo.NewService(repo, todo.Config{})
	srv := NewServer(service)

	listID := types.UUID(uuid.New())
	templateID := types.UUID(uuid.New())
	req := httptest.NewRequest(http.MethodPatch, "/v1/lists/"+listID.String()+"/recurring-templates/"+templateID.String(), bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	// Expect handler to validate payload before calling service; otherwise stub panics
	srv.UpdateRecurringTemplate(w, req, listID, templateID)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// spyRepository captures what was passed to UpdateRecurringTemplate
type spyRepository struct {
	stubRepository
	capturedParams   *domain.UpdateRecurringTemplateParams
	existingTemplate *domain.RecurringTemplate
}

func (s *spyRepository) FindRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	if s.existingTemplate != nil {
		return s.existingTemplate, nil
	}
	return nil, domain.ErrTemplateNotFound
}

func (s *spyRepository) UpdateRecurringTemplate(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error) {
	s.capturedParams = &params
	// Return the existing template with updated fields for the test
	result := *s.existingTemplate
	if params.GenerationWindowDays != nil {
		result.GenerationWindowDays = *params.GenerationWindowDays
	}
	return &result, nil
}

// TestUpdateRecurringTemplate_UpdatesGenerationWindowDays tests that
// generation_window_days is actually updated when included in update_mask.
func TestUpdateRecurringTemplate_UpdatesGenerationWindowDays(t *testing.T) {
	now := time.Now().UTC()
	templateID := uuid.New().String()
	listID := uuid.New().String()

	existingTemplate := &domain.RecurringTemplate{
		ID:                   templateID,
		ListID:               listID,
		Title:                "Weekly Meeting",
		RecurrencePattern:    domain.RecurrenceWeekly,
		GenerationWindowDays: 30,
		IsActive:             true,
		CreatedAt:            now,
		UpdatedAt:            now,
		LastGeneratedUntil:   now,
	}

	repo := &spyRepository{
		existingTemplate: existingTemplate,
	}
	service := todo.NewService(repo, todo.Config{})
	srv := NewServer(service)

	listUUID := types.UUID(uuid.MustParse(listID))
	templateUUID := types.UUID(uuid.MustParse(templateID))
	newWindowDays := 60
	updateMask := []string{"generation_window_days"}

	reqBody := openapi.UpdateRecurringTemplateRequest{
		Template: &openapi.RecurringItemTemplate{
			GenerationWindowDays: &newWindowDays,
		},
		UpdateMask: &updateMask,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPatch, "/v1/lists/"+listUUID.String()+"/recurring-templates/"+templateUUID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.UpdateRecurringTemplate(w, req, listUUID, templateUUID)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, repo.capturedParams, "params should have been passed to repository")
	require.NotNil(t, repo.capturedParams.GenerationWindowDays, "generation_window_days should be in params")
	assert.Equal(t, 60, *repo.capturedParams.GenerationWindowDays, "generation_window_days should be updated to 60")
}

// TestCreateRecurringTemplate_InvalidDurationReturnsBadRequest tests that
// invalid duration strings (estimated_duration, due_offset) return 400 Bad Request
// instead of silently accepting 0.
func TestCreateRecurringTemplate_InvalidDurationReturnsBadRequest(t *testing.T) {
	repo := &stubRepository{}
	service := todo.NewService(repo, todo.Config{})
	srv := NewServer(service)

	listID := types.UUID(uuid.New())

	tests := []struct {
		name              string
		estimatedDuration *string
		dueOffset         *string
		wantStatus        int
	}{
		{
			name:              "invalid estimated_duration returns 400",
			estimatedDuration: ptrString("invalid"),
			dueOffset:         nil,
			wantStatus:        http.StatusBadRequest,
		},
		{
			name:              "invalid due_offset returns 400",
			estimatedDuration: nil,
			dueOffset:         ptrString("garbage"),
			wantStatus:        http.StatusBadRequest,
		},
		{
			name:              "empty string estimated_duration returns 400",
			estimatedDuration: func() *string { s := ""; return &s }(),
			dueOffset:         nil,
			wantStatus:        http.StatusBadRequest,
		},
		{
			name:              "number without unit returns 400",
			estimatedDuration: ptrString("30"),
			dueOffset:         nil,
			wantStatus:        http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := openapi.RecurrencePattern("daily")
			reqBody := openapi.CreateRecurringTemplateRequest{
				Title:             "Test Template",
				RecurrencePattern: pattern,
				EstimatedDuration: tt.estimatedDuration,
				DueOffset:         tt.dueOffset,
			}
			body, _ := json.Marshal(reqBody)

			req := httptest.NewRequest(http.MethodPost, "/v1/lists/"+listID.String()+"/recurring-templates", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			srv.CreateRecurringTemplate(w, req, listID)

			assert.Equal(t, tt.wantStatus, w.Code, "expected status %d but got %d", tt.wantStatus, w.Code)
		})
	}
}

// TestMapTemplateToDTO_IncludesRecurrenceConfig tests that
// MapTemplateToDTO includes recurrence_config in the response.
func TestMapTemplateToDTO_IncludesRecurrenceConfig(t *testing.T) {
	template := &domain.RecurringTemplate{
		ID:                "template-123",
		ListID:            "list-123",
		Title:             "Daily Standup",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig: map[string]interface{}{
			"hour":   9,
			"minute": 30,
		},
		GenerationWindowDays: 30,
		IsActive:             true,
		CreatedAt:            time.Now().UTC(),
		UpdatedAt:            time.Now().UTC(),
		LastGeneratedUntil:   time.Now().UTC(),
	}

	dto := MapTemplateToDTO(template)

	require.NotNil(t, dto.RecurrenceConfig, "recurrence_config should not be nil")

	var config map[string]interface{}
	err := json.Unmarshal([]byte(*dto.RecurrenceConfig), &config)
	require.NoError(t, err, "recurrence_config should be valid JSON")

	assert.Equal(t, float64(9), config["hour"], "hour should be 9")
	assert.Equal(t, float64(30), config["minute"], "minute should be 30")
}
