package worker

import (
	"context"
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
	scheduleGenerationJobFunc  func(ctx context.Context, templateID string, scheduledFor, from, until time.Time) (string, error)
	getGenerationJobFunc       func(ctx context.Context, id string) (*domain.GenerationJob, error)
	updateJobStatusFunc        func(ctx context.Context, id, status string, errorMessage *string) error
	hasPendingOrRunningJobFunc func(ctx context.Context, templateID string) (bool, error)

	// Items
	createTodoItemFunc       func(ctx context.Context, listID string, item *domain.TodoItem) error
	batchCreateTodoItemsFunc func(ctx context.Context, listID string, items []domain.TodoItem) (int64, error)
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

func (m *mockRepository) ScheduleGenerationJob(ctx context.Context, templateID string, scheduledFor, from, until time.Time) (string, error) {
	if m.scheduleGenerationJobFunc != nil {
		return m.scheduleGenerationJobFunc(ctx, templateID, scheduledFor, from, until)
	}
	return "job-id", nil
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

func (m *mockRepository) BatchCreateTodoItems(ctx context.Context, listID string, items []domain.TodoItem) (int64, error) {
	if m.batchCreateTodoItemsFunc != nil {
		return m.batchCreateTodoItemsFunc(ctx, listID, items)
	}
	return int64(len(items)), nil
}

func (m *mockRepository) BatchInsertItemsIgnoreConflict(ctx context.Context, items []*domain.TodoItem) (int, error) {
	return len(items), nil
}

func (m *mockRepository) SetGeneratedThrough(ctx context.Context, templateID string, generatedThrough time.Time) error {
	return nil
}

func (m *mockRepository) ListExceptions(ctx context.Context, templateID string, from, until time.Time) ([]*domain.RecurringTemplateException, error) {
	return nil, nil
}

func (m *mockRepository) FindStaleTemplatesForReconciliation(ctx context.Context, params FindStaleParams) ([]*domain.RecurringTemplate, error) {
	return nil, nil
}

func (m *mockRepository) DeleteFuturePendingItems(ctx context.Context, templateID string, after time.Time) (int64, error) {
	return 0, nil
}

// TestWorker_GracefulShutdown tests that in-flight scheduling operations complete on shutdown.
func TestWorker_GracefulShutdown(t *testing.T) {
	var operationStarted atomic.Bool
	var operationCompleted atomic.Bool

	repo := &mockRepository{
		getActiveTemplatesFunc: func(ctx context.Context) ([]*domain.RecurringTemplate, error) {
			operationStarted.Store(true)
			// Simulate slow operation - should complete even after shutdown signal
			time.Sleep(200 * time.Millisecond)
			operationCompleted.Store(true)
			return nil, nil
		},
	}

	w := New(repo,
		WithScheduleInterval(50*time.Millisecond), // Trigger scheduler quickly
		WithOperationTimeout(5*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error, 1)
	go func() {
		errChan <- w.Start(ctx)
	}()

	// Wait for operation to START
	for range 50 {
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

// TestRunScheduleOnce_SchedulesJobsForTemplatesNeedingGeneration tests that
// RunScheduleOnce creates jobs for templates that need generation.
func TestRunScheduleOnce_SchedulesJobsForTemplatesNeedingGeneration(t *testing.T) {
	var scheduledTemplateIDs []string

	repo := &mockRepository{
		getActiveTemplatesFunc: func(ctx context.Context) ([]*domain.RecurringTemplate, error) {
			return []*domain.RecurringTemplate{
				{
					ID:                    "template-1",
					ListID:                "list-1",
					Title:                 "Daily Task",
					IsActive:              true,
					GenerationHorizonDays: 7,
					CreatedAt:             time.Now().UTC().AddDate(0, 0, -30),
				},
				{
					ID:                    "template-2",
					ListID:                "list-2",
					Title:                 "Weekly Task",
					IsActive:              true,
					GenerationHorizonDays: 14,
					CreatedAt:             time.Now().UTC().AddDate(0, 0, -30),
				},
			}, nil
		},
		scheduleGenerationJobFunc: func(ctx context.Context, templateID string, scheduledFor, from, until time.Time) (string, error) {
			scheduledTemplateIDs = append(scheduledTemplateIDs, templateID)
			return "job-" + templateID, nil
		},
	}

	w := New(repo)
	ctx := context.Background()

	err := w.RunScheduleOnce(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(scheduledTemplateIDs) != 2 {
		t.Errorf("expected 2 jobs scheduled, got %d", len(scheduledTemplateIDs))
	}
}

// TestRunScheduleOnce_SkipsTemplatesWithExistingJobs tests that templates
// with pending or running jobs are skipped.
func TestRunScheduleOnce_SkipsTemplatesWithExistingJobs(t *testing.T) {
	var scheduledTemplateIDs []string

	repo := &mockRepository{
		getActiveTemplatesFunc: func(ctx context.Context) ([]*domain.RecurringTemplate, error) {
			return []*domain.RecurringTemplate{
				{
					ID:                    "template-1",
					ListID:                "list-1",
					Title:                 "Has Pending Job",
					IsActive:              true,
					GenerationHorizonDays: 7,
					CreatedAt:             time.Now().UTC().AddDate(0, 0, -30),
				},
				{
					ID:                    "template-2",
					ListID:                "list-2",
					Title:                 "No Pending Job",
					IsActive:              true,
					GenerationHorizonDays: 7,
					CreatedAt:             time.Now().UTC().AddDate(0, 0, -30),
				},
			}, nil
		},
		hasPendingOrRunningJobFunc: func(ctx context.Context, templateID string) (bool, error) {
			return templateID == "template-1", nil
		},
		scheduleGenerationJobFunc: func(ctx context.Context, templateID string, scheduledFor, from, until time.Time) (string, error) {
			scheduledTemplateIDs = append(scheduledTemplateIDs, templateID)
			return "job-" + templateID, nil
		},
	}

	w := New(repo)
	ctx := context.Background()

	err := w.RunScheduleOnce(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(scheduledTemplateIDs) != 1 {
		t.Errorf("expected 1 job scheduled, got %d", len(scheduledTemplateIDs))
	}
	if len(scheduledTemplateIDs) > 0 && scheduledTemplateIDs[0] != "template-2" {
		t.Errorf("expected template-2 to be scheduled, got %s", scheduledTemplateIDs[0])
	}
}
