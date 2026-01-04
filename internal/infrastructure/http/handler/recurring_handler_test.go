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
	"github.com/rezkam/mono/internal/application/worker"
	"github.com/rezkam/mono/internal/domain"
	openapi "github.com/rezkam/mono/internal/infrastructure/http/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRepository implements todo.Repository and panics on calls we don't expect.
type stubRepository struct{}

// stubGenerator implements todo.TaskGenerator and panics on calls we don't expect.
type stubGenerator struct{}

func (g *stubGenerator) GenerateTasksForTemplateWithExceptions(ctx context.Context, template *domain.RecurringTemplate, from, until time.Time, exceptions []*domain.RecurringTemplateException) ([]*domain.TodoItem, error) {
	return nil, nil // Return empty slice for tests that need generator support
}

// stubCoordinator implements worker.GenerationCoordinator for tests that don't need coordinator.
type stubCoordinator struct{}

func (c *stubCoordinator) InsertJob(ctx context.Context, job *domain.GenerationJob) error {
	return nil
}
func (c *stubCoordinator) InsertMany(ctx context.Context, jobs []*domain.GenerationJob) error {
	return nil
}
func (c *stubCoordinator) ClaimNextJob(ctx context.Context, workerID string, availabilityTimeout time.Duration) (*domain.GenerationJob, error) {
	return nil, nil
}
func (c *stubCoordinator) ExtendAvailability(ctx context.Context, jobID, workerID string, extension time.Duration) error {
	return nil
}
func (c *stubCoordinator) CompleteJob(ctx context.Context, jobID, workerID string) error {
	return nil
}
func (c *stubCoordinator) FailJob(ctx context.Context, jobID, workerID, errMsg string, cfg worker.RetryConfig) (bool, error) {
	return false, nil
}
func (c *stubCoordinator) CancelJob(ctx context.Context, jobID string) error {
	return nil
}
func (c *stubCoordinator) SubscribeToCancellations(ctx context.Context) (<-chan string, error) {
	return nil, nil
}
func (c *stubCoordinator) MoveToDeadLetter(ctx context.Context, job *domain.GenerationJob, workerID, errType, errMsg string, stackTrace *string) error {
	return nil
}
func (c *stubCoordinator) ListDeadLetterJobs(ctx context.Context, limit int) ([]*domain.DeadLetterJob, error) {
	return nil, nil
}
func (c *stubCoordinator) RetryDeadLetterJob(ctx context.Context, deadLetterID, reviewedBy string) (string, error) {
	return "", nil
}
func (c *stubCoordinator) DiscardDeadLetterJob(ctx context.Context, deadLetterID, reviewedBy, note string) error {
	return nil
}
func (c *stubCoordinator) TryAcquireExclusiveRun(ctx context.Context, runType string, holderID string, leaseDuration time.Duration) (func(), bool, error) {
	return nil, false, nil
}

func (s *stubRepository) CreateList(ctx context.Context, list *domain.TodoList) (*domain.TodoList, error) {
	panic("not implemented")
}
func (s *stubRepository) FindListByID(ctx context.Context, id string) (*domain.TodoList, error) {
	panic("not implemented")
}
func (s *stubRepository) FindLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error) {
	panic("not implemented")
}
func (s *stubRepository) UpdateList(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error) {
	panic("not implemented")
}
func (s *stubRepository) CreateItem(ctx context.Context, listID string, item *domain.TodoItem) (*domain.TodoItem, error) {
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
func (s *stubRepository) CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) (*domain.RecurringTemplate, error) {
	panic("not implemented")
}
func (s *stubRepository) FindRecurringTemplateByID(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
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
func (s *stubRepository) BatchInsertItemsIgnoreConflict(ctx context.Context, items []*domain.TodoItem) (int, error) {
	panic("not implemented")
}
func (s *stubRepository) DeleteFuturePendingItems(ctx context.Context, templateID string, fromDate time.Time) (int64, error) {
	panic("not implemented")
}
func (s *stubRepository) FindStaleTemplates(ctx context.Context, listID string, untilDate time.Time) ([]*domain.RecurringTemplate, error) {
	panic("not implemented")
}
func (s *stubRepository) SetGeneratedThrough(ctx context.Context, templateID string, generatedThrough time.Time) error {
	panic("not implemented")
}
func (s *stubRepository) CreateGenerationJob(ctx context.Context, job *domain.GenerationJob) error {
	panic("not implemented")
}
func (s *stubRepository) Transaction(ctx context.Context, fn func(todo.Repository) error) error {
	panic("not implemented")
}
func (s *stubRepository) CreateException(ctx context.Context, exception *domain.RecurringTemplateException) (*domain.RecurringTemplateException, error) {
	panic("not implemented")
}
func (s *stubRepository) FindExceptions(ctx context.Context, templateID string, from, until time.Time) ([]*domain.RecurringTemplateException, error) {
	panic("not implemented")
}
func (s *stubRepository) FindExceptionByOccurrence(ctx context.Context, templateID string, occursAt time.Time) (*domain.RecurringTemplateException, error) {
	panic("not implemented")
}
func (s *stubRepository) DeleteException(ctx context.Context, templateID string, occursAt time.Time) error {
	panic("not implemented")
}
func (s *stubRepository) ListAllExceptionsByTemplate(ctx context.Context, templateID string) ([]*domain.RecurringTemplateException, error) {
	panic("not implemented")
}

// Atomic executes callback without transaction (tests don't need real transactions)
func (s *stubRepository) Atomic(ctx context.Context, fn func(todo.Repository) error) error {
	return fn(s)
}

// spyRepository captures what was passed to UpdateRecurringTemplate
type spyRepository struct {
	stubRepository
	capturedParams   *domain.UpdateRecurringTemplateParams
	existingTemplate *domain.RecurringTemplate
}

func (s *spyRepository) FindRecurringTemplateByID(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	if s.existingTemplate != nil {
		return s.existingTemplate, nil
	}
	return nil, domain.ErrTemplateNotFound
}

func (s *spyRepository) UpdateRecurringTemplate(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error) {
	s.capturedParams = &params
	// Return the existing template with updated fields for the test
	result := *s.existingTemplate
	if params.GenerationHorizonDays != nil {
		result.GenerationHorizonDays = *params.GenerationHorizonDays
	}
	return &result, nil
}

// Atomic executes the function and delegates calls back to the spyRepository
func (s *spyRepository) Atomic(ctx context.Context, fn func(todo.Repository) error) error {
	return fn(s)
}

// Methods required by updateTemplateWithRegeneration
func (s *spyRepository) DeleteFuturePendingItems(ctx context.Context, templateID string, fromDate time.Time) (int64, error) {
	return 0, nil
}

func (s *spyRepository) FindExceptions(ctx context.Context, templateID string, from, until time.Time) ([]*domain.RecurringTemplateException, error) {
	return nil, nil
}

func (s *spyRepository) BatchInsertItemsIgnoreConflict(ctx context.Context, items []*domain.TodoItem) (int, error) {
	return 0, nil
}

func (s *spyRepository) SetGeneratedThrough(ctx context.Context, templateID string, generatedThrough time.Time) error {
	return nil
}

func (s *spyRepository) CreateGenerationJob(ctx context.Context, job *domain.GenerationJob) error {
	return nil
}

// TestUpdateRecurringTemplate_UpdatesGenerationWindowDays tests that
// generation_window_days is actually updated when included in update_mask.
func TestUpdateRecurringTemplate_UpdatesGenerationWindowDays(t *testing.T) {
	now := time.Now().UTC()
	templateIDObj, err := uuid.NewV7()
	require.NoError(t, err)
	templateID := templateIDObj.String()
	listIDObj, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listIDObj.String()

	existingTemplate := &domain.RecurringTemplate{
		ID:                    templateID,
		ListID:                listID,
		Title:                 "Weekly Meeting",
		RecurrencePattern:     domain.RecurrenceWeekly,
		RecurrenceConfig:      map[string]any{},
		SyncHorizonDays:       14,
		GenerationHorizonDays: 30,
		IsActive:              true,
		CreatedAt:             now,
		UpdatedAt:             now,
		GeneratedThrough:      now,
	}

	repo := &spyRepository{
		existingTemplate: existingTemplate,
	}
	generator := &stubGenerator{}
	service := todo.NewService(repo, generator, todo.Config{})
	coordinator := &stubCoordinator{}
	srv := NewTodoHandler(service, coordinator)

	listUUID := types.UUID(uuid.MustParse(listID))
	templateUUID := types.UUID(uuid.MustParse(templateID))
	newWindowDays := 60

	reqBody := openapi.UpdateRecurringTemplateRequest{
		Template: openapi.RecurringItemTemplate{
			GenerationHorizonDays: &newWindowDays,
		},
		UpdateMask: []openapi.UpdateRecurringTemplateRequestUpdateMask{
			openapi.UpdateRecurringTemplateRequestUpdateMaskGenerationHorizonDays,
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPatch, "/v1/lists/"+listUUID.String()+"/recurring-templates/"+templateUUID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.UpdateRecurringTemplate(w, req, listUUID, templateUUID)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, repo.capturedParams, "params should have been passed to repository")
	require.NotNil(t, repo.capturedParams.GenerationHorizonDays, "generation_horizon_days should be in params")
	assert.Equal(t, 60, *repo.capturedParams.GenerationHorizonDays, "generation_horizon_days should be updated to 60")
}

// TestCreateRecurringTemplate_InvalidDurationReturnsBadRequest tests that
// invalid duration strings (estimated_duration, due_offset) return 400 Bad Request
// instead of silently accepting 0.
func TestCreateRecurringTemplate_InvalidDurationReturnsBadRequest(t *testing.T) {
	repo := &stubRepository{}
	generator := &stubGenerator{}
	service := todo.NewService(repo, generator, todo.Config{})
	coordinator := &stubCoordinator{}
	srv := NewTodoHandler(service, coordinator)

	listIDObj, err := uuid.NewV7()
	require.NoError(t, err)
	listID := types.UUID(listIDObj)

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
		RecurrenceConfig: map[string]any{
			"hour":   9,
			"minute": 30,
		},
		SyncHorizonDays:       14,
		GenerationHorizonDays: 30,
		IsActive:              true,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
		GeneratedThrough:      time.Now().UTC(),
	}

	dto := MapTemplateToDTO(template)

	require.NotNil(t, dto.RecurrenceConfig, "recurrence_config should not be nil")

	var config map[string]any
	err := json.Unmarshal([]byte(*dto.RecurrenceConfig), &config)
	require.NoError(t, err, "recurrence_config should be valid JSON")

	assert.Equal(t, float64(9), config["hour"], "hour should be 9")
	assert.Equal(t, float64(30), config["minute"], "minute should be 30")
}
