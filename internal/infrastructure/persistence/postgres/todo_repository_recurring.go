package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres/sqlcgen"
)

// === Recurring Instance Operations ===

// BatchInsertItemsIgnoreConflict inserts items in batch with conflict handling.
// Duplicates based on (recurring_template_id, occurs_at) are silently ignored.
// Returns count of successfully inserted items.
func (s *Store) BatchInsertItemsIgnoreConflict(ctx context.Context, items []*domain.TodoItem) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}

	// Convert domain items to DB parameters
	successCount := 0
	for _, item := range items {
		params, err := domainTodoItemToInsertParams(item)
		if err != nil {
			return successCount, fmt.Errorf("failed to convert item %s: %w", item.ID, err)
		}

		// Insert with conflict handling - ON CONFLICT DO NOTHING
		err = s.queries.InsertItemIgnoreConflict(ctx, params)
		if err != nil {
			return successCount, fmt.Errorf("failed to insert item %s: %w", item.ID, err)
		}

		// Note: Since ON CONFLICT DO NOTHING doesn't tell us if row was inserted,
		// we count all successful executions. This is acceptable because:
		// 1. Idempotent operations shouldn't fail
		// 2. Conflicts are expected and harmless
		// 3. Accurate count would require SELECT after INSERT (2x queries)
		successCount++
	}

	return successCount, nil
}

// domainTodoItemToInsertParams converts a domain TodoItem to InsertItemIgnoreConflictParams.
// Includes version field which is required for batch inserts with ON CONFLICT.
func domainTodoItemToInsertParams(item *domain.TodoItem) (sqlcgen.InsertItemIgnoreConflictParams, error) {
	if _, err := uuid.Parse(item.ID); err != nil {
		return sqlcgen.InsertItemIgnoreConflictParams{}, fmt.Errorf("%w: item %w", domain.ErrInvalidID, err)
	}

	if _, err := uuid.Parse(item.ListID); err != nil {
		return sqlcgen.InsertItemIgnoreConflictParams{}, fmt.Errorf("%w: list %w", domain.ErrInvalidID, err)
	}

	params := sqlcgen.InsertItemIgnoreConflictParams{
		ID:        item.ID,
		ListID:    item.ListID,
		Title:     item.Title,
		Status:    string(item.Status),
		CreatedAt: timeToTimestamptz(item.CreatedAt),
		UpdatedAt: item.UpdatedAt,
		DueAt:     timePtrToTimestamptz(item.DueAt),
		Timezone:  ptrToNullString(item.Timezone),
		Version:   int32(item.Version),
	}

	// Priority
	if item.Priority != nil {
		params.Priority.V = string(*item.Priority)
		params.Priority.Valid = true
	}

	// Durations
	if item.EstimatedDuration != nil {
		params.EstimatedDuration = durationToInterval(*item.EstimatedDuration)
	}
	if item.ActualDuration != nil {
		params.ActualDuration = durationToInterval(*item.ActualDuration)
	}

	// Tags: Direct assignment since sqlc generates []string for TEXT[]
	if len(item.Tags) > 0 {
		params.Tags = item.Tags
	}

	// Recurring Template ID
	recurringTemplateID, err := stringPtrToNullUUID(item.RecurringTemplateID)
	if err != nil {
		return params, fmt.Errorf("invalid recurring template ID: %w", err)
	}
	params.RecurringTemplateID = recurringTemplateID

	// Scheduling fields (new in hybrid refactoring)
	if item.StartsAt != nil {
		params.StartsAt = timeToDate(*item.StartsAt)
	}
	if item.OccursAt != nil {
		params.OccursAt = timeToTimestamptz(*item.OccursAt)
	}
	if item.DueOffset != nil {
		params.DueOffset = durationToInterval(*item.DueOffset)
	}

	return params, nil
}

// DeleteFuturePendingItems deletes future pending items for a template.
// Used before regeneration when pattern changes.
// Returns count of deleted items.
func (s *Store) DeleteFuturePendingItems(ctx context.Context, templateID string, fromDate time.Time) (int64, error) {
	templateUUID, err := uuid.Parse(templateID)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	params := sqlcgen.DeleteFuturePendingItemsParams{
		RecurringTemplateID: uuid.NullUUID{UUID: templateUUID, Valid: true},
		OccursAt:            timeToTimestamptz(fromDate),
	}

	rowsAffected, err := s.queries.DeleteFuturePendingItems(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("failed to delete future items: %w", err)
	}

	return rowsAffected, nil
}

// === Template Generation Tracking ===

// SetGeneratedThrough updates the generated_through marker after generation.
func (s *Store) SetGeneratedThrough(ctx context.Context, templateID string, generatedThrough time.Time) error {
	templateUUID, err := uuid.Parse(templateID)
	if err != nil {
		return fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	params := sqlcgen.SetGeneratedThroughParams{
		GeneratedThrough: timeToDate(generatedThrough),
		UpdatedAt:        time.Now().UTC(),
		ID:               templateUUID.String(),
	}

	rowsAffected, err := s.queries.SetGeneratedThrough(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to set generated_through: %w", err)
	}

	if err := checkRowsAffected(rowsAffected, "template", templateID); err != nil {
		return err
	}

	return nil
}

// === Generation Job Operations ===

// CreateGenerationJob creates a background generation job.
func (s *Store) CreateGenerationJob(ctx context.Context, job *domain.GenerationJob) error {
	params := sqlcgen.InsertGenerationJobParams{
		ID:            job.ID,
		TemplateID:    job.TemplateID,
		GenerateFrom:  job.GenerateFrom,
		GenerateUntil: job.GenerateUntil,
		ScheduledFor:  job.ScheduledFor,
		Status:        "pending",
		RetryCount:    int32(job.RetryCount),
		CreatedAt:     job.CreatedAt,
	}

	return s.queries.InsertGenerationJob(ctx, params)
}
