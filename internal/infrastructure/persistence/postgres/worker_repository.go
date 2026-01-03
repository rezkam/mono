package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rezkam/mono/internal/application/worker"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres/sqlcgen"
)

// === Worker Repository Implementation ===
// Implements application/worker.Repository interface (9 methods)

// === Template Operations ===

// GetActiveTemplatesNeedingGeneration retrieves all active templates that need task generation.
func (s *Store) GetActiveTemplatesNeedingGeneration(ctx context.Context) ([]*domain.RecurringTemplate, error) {
	dbTemplates, err := s.queries.ListAllActiveRecurringTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list active templates: %w", err)
	}

	templates := make([]*domain.RecurringTemplate, 0, len(dbTemplates))
	for _, dbTemplate := range dbTemplates {
		template, err := dbRecurringTemplateToDomain(dbTemplate)
		if err != nil {
			return nil, fmt.Errorf("failed to convert template: %w", err)
		}
		templates = append(templates, template)
	}

	return templates, nil
}

// GetRecurringTemplate retrieves a single recurring template by ID.
func (s *Store) GetRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	dbTemplate, err := s.queries.GetRecurringTemplate(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: template %s", domain.ErrTemplateNotFound, id)
		}
		return nil, fmt.Errorf("failed to get template: %w", err)
	}

	template, err := dbRecurringTemplateToDomain(dbTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to convert template: %w", err)
	}

	return template, nil
}

// UpdateRecurringTemplateGenerationWindow updates the generated_through timestamp.
// NOTE: This will be refactored in Phase 5 to use the new coordin coordination layer.
func (s *Store) UpdateRecurringTemplateGenerationWindow(ctx context.Context, id string, until time.Time) error {
	return s.SetGeneratedThrough(ctx, id, until)
}

// === Job Operations ===

// CreateGenerationJobWorker creates a new background job for generating recurring tasks.
// This is the worker-specific implementation that returns the job ID.
// Note: Will be deprecated in Phase 5 when worker is refactored.
func (s *Store) CreateGenerationJobWorker(ctx context.Context, templateID string, scheduledFor, from, until time.Time) (string, error) {
	if _, err := uuid.Parse(templateID); err != nil {
		return "", fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	jobID, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("failed to generate job ID: %w", err)
	}

	// If scheduledFor is zero value, use current time with 1-second buffer.
	// The buffer prevents clock drift issues between Go and PostgreSQL:
	// Go's time.Now() and PostgreSQL's NOW() are independent clocks that can
	// drift by microseconds. Subtracting 1 second ensures the job is immediately
	// claimable even if PostgreSQL's clock is slightly behind.
	if scheduledFor.IsZero() {
		scheduledFor = time.Now().UTC().Add(-1 * time.Second)
	}

	params := sqlcgen.InsertGenerationJobParams{
		ID:            jobID.String(),
		TemplateID:    templateID,
		ScheduledFor:  scheduledFor,
		Status:        "pending",
		RetryCount:    0,
		GenerateFrom:  from,
		GenerateUntil: until,
		CreatedAt:     time.Now().UTC(),
	}

	if err := s.queries.InsertGenerationJob(ctx, params); err != nil {
		return "", fmt.Errorf("failed to create generation job: %w", err)
	}

	return jobID.String(), nil
}

// ScheduleGenerationJob implements worker.Repository.ScheduleGenerationJob.
func (s *Store) ScheduleGenerationJob(ctx context.Context, templateID string, scheduledFor, from, until time.Time) (string, error) {
	return s.CreateGenerationJobWorker(ctx, templateID, scheduledFor, from, until)
}

// GetGenerationJob retrieves job details by ID.
func (s *Store) GetGenerationJob(ctx context.Context, id string) (*domain.GenerationJob, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	dbJob, err := s.queries.GetGenerationJob(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: job %s", domain.ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	return dbGenerationJobToDomain(dbJob), nil
}

// UpdateGenerationJobStatus updates job status and optionally records an error.
// With coordination pattern, this updates timestamps (completed_at/failed_at) and sets status.
func (s *Store) UpdateGenerationJobStatus(ctx context.Context, id, status string, errorMessage *string) error {
	jobUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	// Map status to appropriate SQL update
	switch status {
	case "completed":
		_, err = s.pool.Exec(ctx, `
			UPDATE recurring_generation_jobs
			SET status = 'completed',
				completed_at = NOW(),
				error_message = NULL
			WHERE id = $1
		`, jobUUID)

	case "failed":
		var errMsg string
		if errorMessage != nil {
			errMsg = *errorMessage
		}
		_, err = s.pool.Exec(ctx, `
			UPDATE recurring_generation_jobs
			SET status = 'failed',
				failed_at = NOW(),
				error_message = $2,
				retry_count = retry_count + 1
			WHERE id = $1
		`, jobUUID, errMsg)

	default:
		return fmt.Errorf("%w: %s (use 'completed' or 'failed')", domain.ErrUnsupportedJobStatus, status)
	}

	if err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	return nil
}

// HasPendingOrRunningJob checks if a template already has a pending or running job.
// Returns true if template has any jobs in 'pending' or 'running' status.
func (s *Store) HasPendingOrRunningJob(ctx context.Context, templateID string) (bool, error) {
	templateUUID, err := uuid.Parse(templateID)
	if err != nil {
		return false, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	var exists bool
	err = s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM recurring_generation_jobs
			WHERE template_id = $1
			  AND status IN ('pending', 'running')
			LIMIT 1
		)
	`, templateUUID).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("failed to check for existing jobs: %w", err)
	}

	return exists, nil
}

// === Item Creation ===

// CreateTodoItem creates a new todo item in a list.
// Used by worker to create task instances generated from recurring templates.
// Worker does not need the returned entity, so we discard it.
func (s *Store) CreateTodoItem(ctx context.Context, listID string, item *domain.TodoItem) error {
	params, err := domainTodoItemToDB(item, listID)
	if err != nil {
		return fmt.Errorf("failed to convert item: %w", err)
	}

	if _, err := s.queries.CreateTodoItem(ctx, params); err != nil {
		return fmt.Errorf("failed to create item: %w", err)
	}

	return nil
}

// BatchCreateTodoItems creates multiple todo items in a single operation.
// Returns the number of items created.
func (s *Store) BatchCreateTodoItems(ctx context.Context, listID string, items []domain.TodoItem) (int64, error) {
	if len(items) == 0 {
		return 0, nil
	}

	params, err := domainTodoItemsToBatchParams(items, listID)
	if err != nil {
		return 0, fmt.Errorf("failed to convert items: %w", err)
	}

	count, err := s.queries.BatchCreateTodoItems(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("failed to batch create items: %w", err)
	}

	return count, nil
}

// === Reconciliation Operations ===

// FindStaleTemplatesForReconciliation retrieves templates needing reconciliation.
func (s *Store) FindStaleTemplatesForReconciliation(ctx context.Context, params worker.FindStaleParams) ([]*domain.RecurringTemplate, error) {
	// Convert time.Time to pgtype.Date
	targetDate := timeToDate(params.TargetDate)

	dbTemplates, err := s.queries.FindStaleTemplatesForReconciliation(ctx, sqlcgen.FindStaleTemplatesForReconciliationParams{
		TargetDate:     targetDate,
		UpdatedBefore:  params.UpdatedBefore,
		ExcludePending: params.ExcludePending,
		Limit:          int32(params.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find stale templates for reconciliation: %w", err)
	}

	templates := make([]*domain.RecurringTemplate, 0, len(dbTemplates))
	for _, dbTemplate := range dbTemplates {
		template, err := dbRecurringTemplateToDomain(dbTemplate)
		if err != nil {
			return nil, fmt.Errorf("failed to convert template: %w", err)
		}
		templates = append(templates, template)
	}

	return templates, nil
}
