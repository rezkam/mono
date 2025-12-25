package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	templateUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	dbTemplate, err := s.queries.GetRecurringTemplate(ctx, uuidToPgtype(templateUUID))
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

// UpdateRecurringTemplateGenerationWindow updates the last_generated_until timestamp.
func (s *Store) UpdateRecurringTemplateGenerationWindow(ctx context.Context, id string, until time.Time) error {
	templateUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	params := sqlcgen.UpdateRecurringTemplateGenerationWindowParams{
		ID:                 uuidToPgtype(templateUUID),
		LastGeneratedUntil: dateToPgtype(until),
		UpdatedAt:          timeToPgtype(time.Now().UTC()),
	}

	// Single-query pattern: check rowsAffected to detect if template was deleted
	rowsAffected, err := s.queries.UpdateRecurringTemplateGenerationWindow(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to update generation window: %w", err)
	}

	return checkRowsAffected(rowsAffected, "template", id)
}

// === Job Operations ===

// CreateGenerationJob creates a new background job for generating recurring tasks.
func (s *Store) CreateGenerationJob(ctx context.Context, templateID string, scheduledFor, from, until time.Time) (string, error) {
	templateUUID, err := uuid.Parse(templateID)
	if err != nil {
		return "", fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	jobID, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("failed to generate job ID: %w", err)
	}

	// If scheduledFor is zero value, pass nil to use database's now()
	var scheduledForParam interface{}
	if scheduledFor.IsZero() {
		scheduledForParam = nil // Will use COALESCE($3, now()) in SQL
	} else {
		scheduledForParam = scheduledFor
	}

	params := sqlcgen.CreateGenerationJobParams{
		ID:            uuidToPgtype(jobID),
		TemplateID:    uuidToPgtype(templateUUID),
		Column3:       scheduledForParam,
		Status:        "pending",
		GenerateFrom:  dateToPgtype(from),
		GenerateUntil: dateToPgtype(until),
		CreatedAt:     timeToPgtype(time.Now().UTC()),
	}

	if err := s.queries.CreateGenerationJob(ctx, params); err != nil {
		return "", fmt.Errorf("failed to create generation job: %w", err)
	}

	return jobID.String(), nil
}

// ClaimNextGenerationJob atomically claims the next pending job using SKIP LOCKED.
// This uses a PostgreSQL function claim_next_generation_job() for atomic claiming.
func (s *Store) ClaimNextGenerationJob(ctx context.Context) (string, error) {
	// Call the PostgreSQL function that atomically claims a job
	row := s.pool.QueryRow(ctx, "SELECT claim_next_generation_job()")

	var jobID *string
	if err := row.Scan(&jobID); err != nil {
		return "", fmt.Errorf("failed to claim job: %w", err)
	}

	// If no job was available, the function returns NULL
	if jobID == nil {
		return "", nil // Empty string = no jobs available
	}

	return *jobID, nil
}

// GetGenerationJob retrieves job details by ID.
func (s *Store) GetGenerationJob(ctx context.Context, id string) (*domain.GenerationJob, error) {
	jobUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	dbJob, err := s.queries.GetGenerationJob(ctx, uuidToPgtype(jobUUID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: job %s", domain.ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	return dbGenerationJobToDomain(dbJob), nil
}

// UpdateGenerationJobStatus updates job status and optionally records an error.
func (s *Store) UpdateGenerationJobStatus(ctx context.Context, id, status string, errorMessage *string) error {
	jobUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	params := sqlcgen.UpdateGenerationJobStatusParams{
		ID:           uuidToPgtype(jobUUID),
		Status:       status,
		StartedAt:    timeToPgtype(time.Now().UTC()),
		ErrorMessage: errorMessage,
	}

	// Single-query pattern: eliminates two-query anti-pattern (GET then UPDATE)
	// retry_count is preserved automatically (not in UPDATE SET clause)
	rowsAffected, err := s.queries.UpdateGenerationJobStatus(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	return checkRowsAffected(rowsAffected, "job", id)
}

// HasPendingOrRunningJob checks if a template already has a pending or running job.
func (s *Store) HasPendingOrRunningJob(ctx context.Context, templateID string) (bool, error) {
	templateUUID, err := uuid.Parse(templateID)
	if err != nil {
		return false, fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	hasJob, err := s.queries.HasPendingOrRunningJob(ctx, uuidToPgtype(templateUUID))
	if err != nil {
		return false, fmt.Errorf("failed to check for existing job: %w", err)
	}

	return hasJob, nil
}

// === Item Creation ===

// CreateTodoItem creates a new todo item in a list.
// Used by worker to create task instances generated from recurring templates.
func (s *Store) CreateTodoItem(ctx context.Context, listID string, item *domain.TodoItem) error {
	params, err := domainTodoItemToDB(item, listID)
	if err != nil {
		return fmt.Errorf("failed to convert item: %w", err)
	}

	if err := s.queries.CreateTodoItem(ctx, params); err != nil {
		return fmt.Errorf("failed to create item: %w", err)
	}

	return nil
}
