package worker

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/domain"
)

// mockRepository implements Repository for testing
type mockRepository struct {
	// Templates
	getRecurringTemplateFunc   func(ctx context.Context, id string) (*domain.RecurringTemplate, error)
	getActiveTemplatesFunc     func(ctx context.Context) ([]*domain.RecurringTemplate, error)
	updateGenerationWindowFunc func(ctx context.Context, id string, until time.Time) error

	// Jobs
	createGenerationJobFunc    func(ctx context.Context, templateID string, scheduledFor, from, until time.Time) (string, error)
	claimNextGenerationJobFunc func(ctx context.Context) (string, error)
	getGenerationJobFunc       func(ctx context.Context, id string) (*domain.GenerationJob, error)
	updateJobStatusFunc        func(ctx context.Context, id, status string, errorMessage *string) error
	hasPendingOrRunningJobFunc func(ctx context.Context, templateID string) (bool, error)

	// Items
	createTodoItemFunc func(ctx context.Context, listID string, item *domain.TodoItem) error
}

func (m *mockRepository) GetActiveTemplatesNeedingGeneration(ctx context.Context) ([]*domain.RecurringTemplate, error) {
	if m.getActiveTemplatesFunc != nil {
		return m.getActiveTemplatesFunc(ctx)
	}
	return nil, nil
}

func (m *mockRepository) GetRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	if m.getRecurringTemplateFunc != nil {
		return m.getRecurringTemplateFunc(ctx, id)
	}
	return nil, domain.ErrNotImplemented
}

func (m *mockRepository) UpdateRecurringTemplateGenerationWindow(ctx context.Context, id string, until time.Time) error {
	if m.updateGenerationWindowFunc != nil {
		return m.updateGenerationWindowFunc(ctx, id, until)
	}
	return nil
}

func (m *mockRepository) CreateGenerationJob(ctx context.Context, templateID string, scheduledFor, from, until time.Time) (string, error) {
	if m.createGenerationJobFunc != nil {
		return m.createGenerationJobFunc(ctx, templateID, scheduledFor, from, until)
	}
	return "job-id", nil
}

func (m *mockRepository) ClaimNextGenerationJob(ctx context.Context) (string, error) {
	if m.claimNextGenerationJobFunc != nil {
		return m.claimNextGenerationJobFunc(ctx)
	}
	return "", nil
}

func (m *mockRepository) GetGenerationJob(ctx context.Context, id string) (*domain.GenerationJob, error) {
	if m.getGenerationJobFunc != nil {
		return m.getGenerationJobFunc(ctx, id)
	}
	return nil, domain.ErrNotImplemented
}

func (m *mockRepository) UpdateGenerationJobStatus(ctx context.Context, id, status string, errorMessage *string) error {
	if m.updateJobStatusFunc != nil {
		return m.updateJobStatusFunc(ctx, id, status, errorMessage)
	}
	return nil
}

func (m *mockRepository) CreateTodoItem(ctx context.Context, listID string, item *domain.TodoItem) error {
	if m.createTodoItemFunc != nil {
		return m.createTodoItemFunc(ctx, listID, item)
	}
	return nil
}

func (m *mockRepository) HasPendingOrRunningJob(ctx context.Context, templateID string) (bool, error) {
	if m.hasPendingOrRunningJobFunc != nil {
		return m.hasPendingOrRunningJobFunc(ctx, templateID)
	}
	return false, nil
}

// TestProcessOneJob_UpdateStatusError_TemplateNotFound tests that errors from
// UpdateGenerationJobStatus are not ignored when marking job as failed after template error.
func TestProcessOneJob_UpdateStatusError_TemplateNotFound(t *testing.T) {
	templateErr := domain.ErrTemplateNotFound
	statusUpdateErr := domain.ErrDatabaseUnavailable

	repo := &mockRepository{
		claimNextGenerationJobFunc: func(ctx context.Context) (string, error) {
			return "job-123", nil
		},
		getGenerationJobFunc: func(ctx context.Context, id string) (*domain.GenerationJob, error) {
			return &domain.GenerationJob{
				ID:            "job-123",
				TemplateID:    "template-456",
				GenerateFrom:  time.Now().UTC(),
				GenerateUntil: time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
		getRecurringTemplateFunc: func(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
			return nil, templateErr
		},
		updateJobStatusFunc: func(ctx context.Context, id, status string, errorMessage *string) error {
			if status == "failed" {
				return statusUpdateErr
			}
			return nil
		},
	}

	w := New(repo, WithOperationTimeout(5*time.Second))
	ctx := context.Background()

	processed, err := w.RunProcessOnce(ctx)

	if processed {
		t.Error("expected processed to be false when template not found")
	}
	if err == nil {
		t.Fatal("expected error when both template and status update fail")
	}
	// Error should mention both failures
	if !strings.Contains(err.Error(), "failed to get template") {
		t.Errorf("error should mention template failure, got: %v", err)
	}
	if !strings.Contains(err.Error(), "failed to update job status") {
		t.Errorf("error should mention status update failure, got: %v", err)
	}
}

// TestProcessOneJob_UpdateStatusError_GenerationFailed tests that errors from
// UpdateGenerationJobStatus are not ignored when marking job as failed after generation error.
func TestProcessOneJob_UpdateStatusError_GenerationFailed(t *testing.T) {
	statusUpdateErr := domain.ErrDatabaseUnavailable

	repo := &mockRepository{
		claimNextGenerationJobFunc: func(ctx context.Context) (string, error) {
			return "job-123", nil
		},
		getGenerationJobFunc: func(ctx context.Context, id string) (*domain.GenerationJob, error) {
			return &domain.GenerationJob{
				ID:            "job-123",
				TemplateID:    "template-456",
				GenerateFrom:  time.Now().UTC(),
				GenerateUntil: time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
		getRecurringTemplateFunc: func(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
			return &domain.RecurringTemplate{
				ID:                "template-456",
				ListID:            "list-789",
				Title:             "Test Template",
				RecurrencePattern: "INVALID_PATTERN", // Will cause generator to fail
				IsActive:          true,
			}, nil
		},
		updateJobStatusFunc: func(ctx context.Context, id, status string, errorMessage *string) error {
			if status == "failed" {
				return statusUpdateErr
			}
			return nil
		},
	}

	w := New(repo, WithOperationTimeout(5*time.Second))
	ctx := context.Background()

	processed, err := w.RunProcessOnce(ctx)

	if processed {
		t.Error("expected processed to be false when generation fails")
	}
	if err == nil {
		t.Fatal("expected error when both generation and status update fail")
	}
	// Error should mention both failures
	if !strings.Contains(err.Error(), "failed to generate tasks") {
		t.Errorf("error should mention generation failure, got: %v", err)
	}
	if !strings.Contains(err.Error(), "failed to update job status") {
		t.Errorf("error should mention status update failure, got: %v", err)
	}
}

// TestProcessOneJob_UpdateStatusError_TaskCreationFailed tests that errors from
// UpdateGenerationJobStatus are not ignored when marking job as failed after task creation error.
func TestProcessOneJob_UpdateStatusError_TaskCreationFailed(t *testing.T) {
	taskCreateErr := domain.ErrFailedToCreateTask
	statusUpdateErr := domain.ErrDatabaseUnavailable

	repo := &mockRepository{
		claimNextGenerationJobFunc: func(ctx context.Context) (string, error) {
			return "job-123", nil
		},
		getGenerationJobFunc: func(ctx context.Context, id string) (*domain.GenerationJob, error) {
			return &domain.GenerationJob{
				ID:            "job-123",
				TemplateID:    "template-456",
				GenerateFrom:  time.Now().UTC(),
				GenerateUntil: time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
		getRecurringTemplateFunc: func(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
			return &domain.RecurringTemplate{
				ID:                   "template-456",
				ListID:               "list-789",
				Title:                "Test Template",
				RecurrencePattern:    domain.RecurrenceDaily,
				IsActive:             true,
				GenerationWindowDays: 7,
			}, nil
		},
		createTodoItemFunc: func(ctx context.Context, listID string, item *domain.TodoItem) error {
			return taskCreateErr
		},
		updateJobStatusFunc: func(ctx context.Context, id, status string, errorMessage *string) error {
			if status == "failed" {
				return statusUpdateErr
			}
			return nil
		},
	}

	w := New(repo, WithOperationTimeout(5*time.Second))
	ctx := context.Background()

	processed, err := w.RunProcessOnce(ctx)

	if processed {
		t.Error("expected processed to be false when task creation fails")
	}
	if err == nil {
		t.Fatal("expected error when both task creation and status update fail")
	}
	// Error should mention both failures
	if !strings.Contains(err.Error(), "failed to create task") {
		t.Errorf("error should mention task creation failure, got: %v", err)
	}
	if !strings.Contains(err.Error(), "failed to update job status") {
		t.Errorf("error should mention status update failure, got: %v", err)
	}
}

// TestProcessOneJob_UpdateStatusError_CompletionFailed tests that errors from
// UpdateGenerationJobStatus are not ignored when marking job as completed.
func TestProcessOneJob_UpdateStatusError_CompletionFailed(t *testing.T) {
	statusUpdateErr := domain.ErrCompletionFailed
	tasksCreated := 0

	repo := &mockRepository{
		claimNextGenerationJobFunc: func(ctx context.Context) (string, error) {
			return "job-123", nil
		},
		getGenerationJobFunc: func(ctx context.Context, id string) (*domain.GenerationJob, error) {
			return &domain.GenerationJob{
				ID:            "job-123",
				TemplateID:    "template-456",
				GenerateFrom:  time.Now().UTC(),
				GenerateUntil: time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
		getRecurringTemplateFunc: func(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
			return &domain.RecurringTemplate{
				ID:                   "template-456",
				ListID:               "list-789",
				Title:                "Test Template",
				RecurrencePattern:    domain.RecurrenceDaily,
				IsActive:             true,
				GenerationWindowDays: 7,
			}, nil
		},
		createTodoItemFunc: func(ctx context.Context, listID string, item *domain.TodoItem) error {
			tasksCreated++
			return nil
		},
		updateJobStatusFunc: func(ctx context.Context, id, status string, errorMessage *string) error {
			if status == "completed" {
				return statusUpdateErr
			}
			return nil
		},
	}

	w := New(repo, WithOperationTimeout(5*time.Second))
	ctx := context.Background()

	processed, err := w.RunProcessOnce(ctx)

	// Tasks were created, so some work was done
	if tasksCreated == 0 {
		t.Error("expected at least one task to be created")
	}

	// But processing should fail because we couldn't mark job as completed
	if processed {
		t.Error("expected processed to be false when completion status update fails")
	}
	if err == nil {
		t.Fatal("expected error when marking job as completed fails")
	}
	if !strings.Contains(err.Error(), "failed to mark job as completed") {
		t.Errorf("error should mention completion failure, got: %v", err)
	}
}

// TestProcessOneJob_StatusUpdateSuccess tests that when UpdateGenerationJobStatus succeeds,
// errors are still properly returned for the primary failure.
func TestProcessOneJob_StatusUpdateSuccess_TemplateNotFound(t *testing.T) {
	templateErr := domain.ErrTemplateNotFound
	statusUpdated := false

	repo := &mockRepository{
		claimNextGenerationJobFunc: func(ctx context.Context) (string, error) {
			return "job-123", nil
		},
		getGenerationJobFunc: func(ctx context.Context, id string) (*domain.GenerationJob, error) {
			return &domain.GenerationJob{
				ID:            "job-123",
				TemplateID:    "template-456",
				GenerateFrom:  time.Now().UTC(),
				GenerateUntil: time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
		getRecurringTemplateFunc: func(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
			return nil, templateErr
		},
		updateJobStatusFunc: func(ctx context.Context, id, status string, errorMessage *string) error {
			if status == "failed" {
				statusUpdated = true
			}
			return nil
		},
	}

	w := New(repo, WithOperationTimeout(5*time.Second))
	ctx := context.Background()

	processed, err := w.RunProcessOnce(ctx)

	if processed {
		t.Error("expected processed to be false when template not found")
	}
	if err == nil {
		t.Fatal("expected error when template not found")
	}
	if !statusUpdated {
		t.Error("expected status to be updated to failed")
	}
	// Error should only mention template failure (status update succeeded)
	if !strings.Contains(err.Error(), "failed to get template") {
		t.Errorf("error should mention template failure, got: %v", err)
	}
	if strings.Contains(err.Error(), "failed to update job status") {
		t.Errorf("error should NOT mention status update failure when it succeeded, got: %v", err)
	}
}

// TestWorker_GracefulShutdown tests that in-flight operations complete on shutdown.
// When ctx is cancelled:
// 1. Worker stops accepting new work (tickers stop)
// 2. In-flight operations complete (not aborted)
// 3. Worker exits cleanly
//
// Note: This test uses real time.Sleep because it's testing actual shutdown behavior
// of a long-running worker with tickers. synctest is not appropriate here.
func TestWorker_GracefulShutdown(t *testing.T) {
	var operationStarted atomic.Bool
	var operationCompleted atomic.Bool

	repo := &mockRepository{
		getActiveTemplatesFunc: func(ctx context.Context) ([]*domain.RecurringTemplate, error) {
			return nil, nil
		},
		claimNextGenerationJobFunc: func(ctx context.Context) (string, error) {
			operationStarted.Store(true)
			// Simulate slow operation - should complete even after shutdown signal
			time.Sleep(200 * time.Millisecond)
			operationCompleted.Store(true)
			return "", nil
		},
	}

	w := New(repo,
		WithScheduleInterval(1*time.Hour),        // Don't trigger scheduler
		WithProcessInterval(50*time.Millisecond), // Trigger processor quickly
		WithOperationTimeout(5*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error, 1)
	go func() {
		errChan <- w.Start(ctx)
	}()

	// Wait for operation to START
	for i := 0; i < 50; i++ {
		if operationStarted.Load() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !operationStarted.Load() {
		t.Fatal("operation never started")
	}

	// Cancel context while operation is in-flight
	cancel()

	// Wait for worker to exit
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("expected nil error on graceful shutdown, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not exit after context cancellation")
	}

	// Verify operation completed (was not aborted)
	if !operationCompleted.Load() {
		t.Error("in-flight operation was aborted - should have completed gracefully")
	}
}
