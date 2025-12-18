package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rezkam/mono/internal/core"
	"github.com/rezkam/mono/internal/storage/sql/sqlcgen"
	"github.com/sqlc-dev/pqtype"
)

// CreateRecurringTemplate creates a new recurring task template.
func (s *Store) CreateRecurringTemplate(ctx context.Context, template *core.RecurringTaskTemplate) error {
	templateID, err := uuid.Parse(template.ID)
	if err != nil {
		return fmt.Errorf("invalid template id: %w", err)
	}

	listID, err := uuid.Parse(template.ListID)
	if err != nil {
		return fmt.Errorf("invalid list id: %w", err)
	}

	// Convert tags to JSONB
	tagsJSON := pqtype.NullRawMessage{Valid: false}
	if len(template.Tags) > 0 {
		tagsBytes, err := json.Marshal(template.Tags)
		if err != nil {
			return fmt.Errorf("failed to marshal tags: %w", err)
		}
		tagsJSON = pqtype.NullRawMessage{RawMessage: tagsBytes, Valid: true}
	}

	// Convert priority
	priority := sql.NullString{Valid: false}
	if template.Priority != nil {
		priority = sql.NullString{String: string(*template.Priority), Valid: true}
	}

	// Convert estimated duration to microseconds
	estDuration := pgtype.Interval{}
	if template.EstimatedDuration != nil {
		estDuration = pgtype.Interval{Microseconds: int64(*template.EstimatedDuration / time.Microsecond), Valid: true}
	}

	// Convert recurrence config to JSON
	configBytes, err := json.Marshal(template.RecurrenceConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal recurrence config: %w", err)
	}

	// Convert due offset to microseconds
	dueOffset := pgtype.Interval{}
	if template.DueOffset != nil {
		dueOffset = pgtype.Interval{Microseconds: int64(*template.DueOffset / time.Microsecond), Valid: true}
	}

	err = s.queries.CreateRecurringTemplate(ctx, sqlcgen.CreateRecurringTemplateParams{
		ID:                   templateID,
		ListID:               listID,
		Title:                template.Title,
		Tags:                 tagsJSON,
		Priority:             priority,
		EstimatedDuration:    estDuration,
		RecurrencePattern:    string(template.RecurrencePattern),
		RecurrenceConfig:     configBytes,
		DueOffset:            dueOffset,
		IsActive:             template.IsActive,
		CreatedAt:            template.CreatedAt,
		UpdatedAt:            template.UpdatedAt,
		LastGeneratedUntil:   template.LastGeneratedUntil,
		GenerationWindowDays: int32(template.GenerationWindowDays),
	})
	if err != nil {
		if isForeignKeyViolation(err, "list_id") {
			return fmt.Errorf("%w: %v", ErrListNotFound, err)
		}
		return fmt.Errorf("failed to create recurring template: %w", err)
	}
	return nil
}

// GetRecurringTemplate retrieves a recurring template by its ID.
func (s *Store) GetRecurringTemplate(ctx context.Context, id string) (*core.RecurringTaskTemplate, error) {
	templateID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidID, err)
	}

	dbTemplate, err := s.queries.GetRecurringTemplate(ctx, templateID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: template %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}

	return dbTemplateToCore(dbTemplate)
}

// UpdateRecurringTemplate updates an existing recurring template.
func (s *Store) UpdateRecurringTemplate(ctx context.Context, template *core.RecurringTaskTemplate) error {
	templateID, err := uuid.Parse(template.ID)
	if err != nil {
		return fmt.Errorf("invalid template id: %w", err)
	}

	// Convert tags to JSONB
	tagsJSON := pqtype.NullRawMessage{Valid: false}
	if len(template.Tags) > 0 {
		tagsBytes, err := json.Marshal(template.Tags)
		if err != nil {
			return fmt.Errorf("failed to marshal tags: %w", err)
		}
		tagsJSON = pqtype.NullRawMessage{RawMessage: tagsBytes, Valid: true}
	}

	// Convert priority
	priority := sql.NullString{Valid: false}
	if template.Priority != nil {
		priority = sql.NullString{String: string(*template.Priority), Valid: true}
	}

	// Convert estimated duration
	estDuration := pgtype.Interval{}
	if template.EstimatedDuration != nil {
		estDuration = pgtype.Interval{Microseconds: int64(*template.EstimatedDuration / time.Microsecond), Valid: true}
	}

	// Convert recurrence config
	configBytes, err := json.Marshal(template.RecurrenceConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal recurrence config: %w", err)
	}

	// Convert due offset
	dueOffset := pgtype.Interval{}
	if template.DueOffset != nil {
		dueOffset = pgtype.Interval{Microseconds: int64(*template.DueOffset / time.Microsecond), Valid: true}
	}

	return s.queries.UpdateRecurringTemplate(ctx, sqlcgen.UpdateRecurringTemplateParams{
		Title:             template.Title,
		Tags:              tagsJSON,
		Priority:          priority,
		EstimatedDuration: estDuration,
		RecurrencePattern: string(template.RecurrencePattern),
		RecurrenceConfig:  configBytes,
		DueOffset:         dueOffset,
		UpdatedAt:         time.Now().UTC(),
		ID:                templateID,
	})
}

// UpdateRecurringTemplateGenerationWindow updates only the last_generated_until field.
func (s *Store) UpdateRecurringTemplateGenerationWindow(ctx context.Context, id string, lastGeneratedUntil time.Time) error {
	templateID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid template id: %w", err)
	}

	return s.queries.UpdateRecurringTemplateGenerationWindow(ctx, sqlcgen.UpdateRecurringTemplateGenerationWindowParams{
		LastGeneratedUntil: lastGeneratedUntil,
		UpdatedAt:          time.Now().UTC(),
		ID:                 templateID,
	})
}

// DeleteRecurringTemplate deletes a recurring template.
func (s *Store) DeleteRecurringTemplate(ctx context.Context, id string) error {
	templateID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid template id: %w", err)
	}

	return s.queries.DeleteRecurringTemplate(ctx, templateID)
}

// ListRecurringTemplates lists all recurring templates for a list.
func (s *Store) ListRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*core.RecurringTaskTemplate, error) {
	listUUID, err := uuid.Parse(listID)
	if err != nil {
		return nil, fmt.Errorf("invalid list id: %w", err)
	}

	var dbTemplates []sqlcgen.RecurringTaskTemplate
	if activeOnly {
		dbTemplates, err = s.queries.ListRecurringTemplates(ctx, listUUID)
	} else {
		dbTemplates, err = s.queries.ListAllRecurringTemplatesByList(ctx, listUUID)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list templates: %w", err)
	}

	templates := make([]*core.RecurringTaskTemplate, 0, len(dbTemplates))
	for _, dbTemplate := range dbTemplates {
		template, err := dbTemplateToCore(dbTemplate)
		if err != nil {
			return nil, fmt.Errorf("failed to convert template: %w", err)
		}
		templates = append(templates, template)
	}

	return templates, nil
}

// dbTemplateToCore converts a database recurring template to a core recurring template.
func dbTemplateToCore(dbTemplate sqlcgen.RecurringTaskTemplate) (*core.RecurringTaskTemplate, error) {
	template := &core.RecurringTaskTemplate{
		ID:                   dbTemplate.ID.String(),
		ListID:               dbTemplate.ListID.String(),
		Title:                dbTemplate.Title,
		RecurrencePattern:    core.RecurrencePattern(dbTemplate.RecurrencePattern),
		IsActive:             dbTemplate.IsActive,
		CreatedAt:            dbTemplate.CreatedAt,
		UpdatedAt:            dbTemplate.UpdatedAt,
		LastGeneratedUntil:   dbTemplate.LastGeneratedUntil,
		GenerationWindowDays: int(dbTemplate.GenerationWindowDays),
	}

	// Parse tags
	if dbTemplate.Tags.Valid && len(dbTemplate.Tags.RawMessage) > 0 {
		if err := json.Unmarshal(dbTemplate.Tags.RawMessage, &template.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}

	// Parse priority
	if dbTemplate.Priority.Valid {
		p := core.TaskPriority(dbTemplate.Priority.String)
		template.Priority = &p
	}

	// Parse estimated duration
	if dbTemplate.EstimatedDuration.Valid {
		d := time.Duration(dbTemplate.EstimatedDuration.Microseconds) * time.Microsecond
		template.EstimatedDuration = &d
	}

	// Parse recurrence config
	var config map[string]interface{}
	if err := json.Unmarshal(dbTemplate.RecurrenceConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recurrence config: %w", err)
	}
	template.RecurrenceConfig = config

	// Parse due offset
	if dbTemplate.DueOffset.Valid {
		d := time.Duration(dbTemplate.DueOffset.Microseconds) * time.Microsecond
		template.DueOffset = &d
	}

	return template, nil
}

// GetActiveTemplatesNeedingGeneration returns all active templates that need task generation.
func (s *Store) GetActiveTemplatesNeedingGeneration(ctx context.Context) ([]*core.RecurringTaskTemplate, error) {
	dbTemplates, err := s.queries.ListAllActiveRecurringTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list active templates: %w", err)
	}

	templates := make([]*core.RecurringTaskTemplate, 0, len(dbTemplates))
	today := time.Now().UTC()

	for _, dbTemplate := range dbTemplates {
		// Check if template needs generation
		targetDate := dbTemplate.LastGeneratedUntil.AddDate(0, 0, int(dbTemplate.GenerationWindowDays))
		if targetDate.Before(today) || targetDate.Equal(today) {
			template, err := dbTemplateToCore(dbTemplate)
			if err != nil {
				return nil, fmt.Errorf("failed to convert template: %w", err)
			}
			templates = append(templates, template)
		}
	}

	return templates, nil
}

// CreateGenerationJob creates a new generation job for a template.
func (s *Store) CreateGenerationJob(ctx context.Context, templateID string, scheduledFor time.Time, generateFrom, generateUntil time.Time) (string, error) {
	templateUUID, err := uuid.Parse(templateID)
	if err != nil {
		return "", fmt.Errorf("invalid template id: %w", err)
	}

	jobID, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("failed to generate job id: %v", err)
	}
	err = s.queries.CreateGenerationJob(ctx, sqlcgen.CreateGenerationJobParams{
		ID:            jobID,
		TemplateID:    templateUUID,
		ScheduledFor:  scheduledFor,
		Status:        "PENDING",
		GenerateFrom:  generateFrom,
		GenerateUntil: generateUntil,
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create generation job: %w", err)
	}

	return jobID.String(), nil
}

// ClaimNextGenerationJob claims the next pending job using SKIP LOCKED.
func (s *Store) ClaimNextGenerationJob(ctx context.Context) (string, error) {
	row := s.db.QueryRowContext(ctx, "SELECT claim_next_generation_job()")

	var jobID sql.NullString
	if err := row.Scan(&jobID); err != nil {
		return "", fmt.Errorf("failed to claim job: %w", err)
	}

	if !jobID.Valid {
		return "", nil // No jobs available
	}

	return jobID.String, nil
}

// GetGenerationJob retrieves a generation job by ID.
func (s *Store) GetGenerationJob(ctx context.Context, jobID string) (*core.GenerationJob, error) {
	jobUUID, err := uuid.Parse(jobID)
	if err != nil {
		return nil, fmt.Errorf("invalid job id: %w", err)
	}

	dbJob, err := s.queries.GetGenerationJob(ctx, jobUUID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	job := &core.GenerationJob{
		ID:            dbJob.ID.String(),
		TemplateID:    dbJob.TemplateID.String(),
		ScheduledFor:  dbJob.ScheduledFor,
		Status:        dbJob.Status,
		GenerateFrom:  dbJob.GenerateFrom,
		GenerateUntil: dbJob.GenerateUntil,
		CreatedAt:     dbJob.CreatedAt,
	}

	if dbJob.StartedAt.Valid {
		job.StartedAt = &dbJob.StartedAt.Time
	}
	if dbJob.CompletedAt.Valid {
		job.CompletedAt = &dbJob.CompletedAt.Time
	}
	if dbJob.FailedAt.Valid {
		job.FailedAt = &dbJob.FailedAt.Time
	}
	if dbJob.ErrorMessage.Valid {
		job.ErrorMessage = &dbJob.ErrorMessage.String
	}
	job.RetryCount = int(dbJob.RetryCount)

	return job, nil
}

// UpdateGenerationJobStatus updates the status of a generation job.
func (s *Store) UpdateGenerationJobStatus(ctx context.Context, jobID string, status string, errorMessage *string) error {
	jobUUID, err := uuid.Parse(jobID)
	if err != nil {
		return fmt.Errorf("invalid job id: %w", err)
	}

	// Get current job to increment retry count
	job, err := s.GetGenerationJob(ctx, jobID)
	if err != nil {
		return err
	}

	retryCount := int32(job.RetryCount)
	if status == "FAILED" {
		retryCount++
	}

	errMsg := sql.NullString{Valid: false}
	if errorMessage != nil {
		errMsg = sql.NullString{String: *errorMessage, Valid: true}
	}

	now := time.Now().UTC()
	return s.queries.UpdateGenerationJobStatus(ctx, sqlcgen.UpdateGenerationJobStatusParams{
		Status:       status,
		StartedAt:    sql.NullTime{Time: now, Valid: true},
		ErrorMessage: errMsg,
		RetryCount:   retryCount,
		ID:           jobUUID,
	})
}
