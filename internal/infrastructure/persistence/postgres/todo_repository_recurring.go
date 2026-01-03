package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rezkam/mono/internal/application/todo"
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

	// Tags
	if len(item.Tags) > 0 {
		tagsJSON, err := jsonMarshalHelper(item.Tags)
		if err != nil {
			return params, fmt.Errorf("failed to marshal tags: %w", err)
		}
		params.Tags = tagsJSON
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

// FindStaleTemplates finds templates needing generation.
// Returns templates where generated_through < target date.
func (s *Store) FindStaleTemplates(ctx context.Context, listID string, untilDate time.Time) ([]*domain.RecurringTemplate, error) {
	// Parse listID - empty string means search all lists
	var listUUIDStr string
	if listID != "" {
		listUUID, err := uuid.Parse(listID)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
		}
		listUUIDStr = listUUID.String()
	}

	params := sqlcgen.FindStaleTemplatesParams{
		ListID:           listUUIDStr,
		GeneratedThrough: timeToDate(untilDate),
	}

	dbTemplates, err := s.queries.FindStaleTemplates(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to find stale templates: %w", err)
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

// === Composite Operations ===

// UpdateItemWithException atomically updates a recurring item and creates an exception.
func (s *Store) UpdateItemWithException(
	ctx context.Context,
	params domain.UpdateItemParams,
	exception *domain.RecurringTemplateException,
) (*domain.TodoItem, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txStore := &Store{pool: s.pool, queries: s.queries.WithTx(tx)}

	// 1. Create exception first
	_, err = txStore.CreateException(ctx, exception)
	if err != nil {
		return nil, err
	}

	// 2. Update item
	updatedItem, err := txStore.UpdateItem(ctx, params)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return updatedItem, nil
}

// DeleteItemWithException atomically archives an item and creates a deletion exception.
func (s *Store) DeleteItemWithException(
	ctx context.Context,
	listID string,
	itemID string,
	exception *domain.RecurringTemplateException,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txStore := &Store{pool: s.pool, queries: s.queries.WithTx(tx)}

	// 1. Create exception first
	_, err = txStore.CreateException(ctx, exception)
	if err != nil {
		return err
	}

	// 2. Archive item (soft delete by setting status to archived)
	archived := domain.TaskStatusArchived
	updateParams := domain.UpdateItemParams{
		ItemID:     itemID,
		ListID:     listID,
		UpdateMask: []string{"status"},
		Status:     &archived,
	}

	_, err = txStore.UpdateItem(ctx, updateParams)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// CreateTemplateWithInitialGeneration atomically creates a template and generates initial items.
func (s *Store) CreateTemplateWithInitialGeneration(
	ctx context.Context,
	template *domain.RecurringTemplate,
	syncItems []*domain.TodoItem,
	syncEnd time.Time,
	asyncJob *domain.GenerationJob,
) (*domain.RecurringTemplate, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txStore := &Store{pool: s.pool, queries: s.queries.WithTx(tx)}

	// 1. Create template
	createdTemplate, err := txStore.CreateRecurringTemplate(ctx, template)
	if err != nil {
		return nil, fmt.Errorf("failed to create template: %w", err)
	}

	// 2. Insert sync horizon items
	if len(syncItems) > 0 {
		_, err = txStore.BatchInsertItemsIgnoreConflict(ctx, syncItems)
		if err != nil {
			return nil, fmt.Errorf("failed to insert sync items: %w", err)
		}
	}

	// 3. Update generation marker
	if err := txStore.SetGeneratedThrough(ctx, template.ID, syncEnd); err != nil {
		return nil, fmt.Errorf("failed to update generation marker: %w", err)
	}

	// 4. Create async generation job (if provided)
	if asyncJob != nil {
		if err := txStore.CreateGenerationJob(ctx, asyncJob); err != nil {
			return nil, fmt.Errorf("failed to create generation job: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return createdTemplate, nil
}

// UpdateTemplateWithRegeneration atomically updates template and regenerates future items.
func (s *Store) UpdateTemplateWithRegeneration(
	ctx context.Context,
	params domain.UpdateRecurringTemplateParams,
	deleteFrom time.Time,
	syncItems []*domain.TodoItem,
	syncEnd time.Time,
) (*domain.RecurringTemplate, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txStore := &Store{pool: s.pool, queries: s.queries.WithTx(tx)}

	// 1. Update template
	_, err = txStore.UpdateRecurringTemplate(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to update template: %w", err)
	}

	// 2. Delete future pending items
	if _, err := txStore.DeleteFuturePendingItems(ctx, params.TemplateID, deleteFrom); err != nil {
		return nil, fmt.Errorf("failed to delete future items: %w", err)
	}

	// 3. Insert regenerated items
	if len(syncItems) > 0 {
		_, err = txStore.BatchInsertItemsIgnoreConflict(ctx, syncItems)
		if err != nil {
			return nil, fmt.Errorf("failed to insert regenerated items: %w", err)
		}
	}

	// 4. Update generation marker
	if err := txStore.SetGeneratedThrough(ctx, params.TemplateID, syncEnd); err != nil {
		return nil, fmt.Errorf("failed to update generation marker: %w", err)
	}

	// 5. Refetch template to get updated generated_through value
	finalTemplate, err := txStore.FindRecurringTemplate(ctx, params.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("failed to refetch template: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return finalTemplate, nil
}

// === Transaction Support (DEPRECATED) ===

// Transaction executes a function within a database transaction.
// DEPRECATED: Use composite operations instead (UpdateItemWithException, etc.)
func (s *Store) Transaction(ctx context.Context, fn func(tx todo.Repository) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Create a new Store instance bound to this transaction
	txStore := &Store{
		pool:    s.pool,
		queries: s.queries.WithTx(tx),
	}

	// Execute the callback with the transactional repository
	if err := fn(txStore); err != nil {
		return err // Rollback happens in defer
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// === Dead Letter Queue Operations ===

// ListDeadLetterJobs retrieves unresolved dead letter jobs for manual review.
func (s *Store) ListDeadLetterJobs(ctx context.Context, limit int) ([]*domain.DeadLetterJob, error) {
	rows, err := s.queries.ListPendingDeadLetterJobs(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("failed to list dead letter jobs: %w", err)
	}

	jobs := make([]*domain.DeadLetterJob, 0, len(rows))
	for _, row := range rows {
		job := sqlcDeadLetterToDomain(row)
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// RetryDeadLetterJob creates a new job from a dead letter entry and marks it as retried.
func (s *Store) RetryDeadLetterJob(ctx context.Context, deadLetterID, reviewedBy string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.queries.WithTx(tx)

	newJobID, err := retryDeadLetterJobTx(ctx, qtx, deadLetterID, reviewedBy)
	if err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return newJobID, nil
}

// DiscardDeadLetterJob marks a dead letter job as permanently discarded.
func (s *Store) DiscardDeadLetterJob(ctx context.Context, deadLetterID, reviewedBy, note string) error {
	dlID, err := uuid.Parse(deadLetterID)
	if err != nil {
		return fmt.Errorf("invalid dead letter ID: %w", err)
	}

	params := sqlcgen.MarkDeadLetterAsDiscardedParams{
		ID:           pgtype.UUID{Bytes: dlID, Valid: true},
		ReviewedBy:   sql.Null[string]{V: reviewedBy, Valid: true},
		ReviewerNote: sql.Null[string]{V: note, Valid: true},
	}

	rows, err := s.queries.MarkDeadLetterAsDiscarded(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to mark dead letter as discarded: %w", err)
	}
	if rows == 0 {
		return domain.ErrDeadLetterNotFound
	}

	return nil
}
