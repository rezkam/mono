package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres/sqlcgen"
)

// retryDeadLetterJobTx implements the retry logic within a transaction.
// Shared by both Store and PostgresCoordinator to avoid duplication.
func retryDeadLetterJobTx(ctx context.Context, qtx *sqlcgen.Queries, deadLetterID, reviewedBy string) (string, error) {
	// Get the dead letter job
	dlID, err := uuid.Parse(deadLetterID)
	if err != nil {
		return "", fmt.Errorf("invalid dead letter ID: %w", err)
	}

	dlJob, err := qtx.GetDeadLetterJob(ctx, pgtype.UUID{Bytes: dlID, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrDeadLetterNotFound
		}
		return "", fmt.Errorf("failed to get dead letter job: %w", err)
	}

	// Create new job from dead letter
	newJobUUID, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("failed to generate job ID: %w", err)
	}

	newJob := sqlcgen.InsertGenerationJobParams{
		ID:            newJobUUID.String(),
		TemplateID:    uuid.UUID(dlJob.TemplateID.Bytes).String(),
		GenerateFrom:  timestamptzToTime(dlJob.GenerateFrom),
		GenerateUntil: timestamptzToTime(dlJob.GenerateUntil),
		ScheduledFor:  time.Now().UTC(),
		Status:        "pending",
		RetryCount:    0, // Reset retry count
		CreatedAt:     time.Now().UTC(),
	}

	if err := qtx.InsertGenerationJob(ctx, newJob); err != nil {
		return "", fmt.Errorf("failed to insert new job: %w", err)
	}

	// Mark dead letter as retried
	markParams := sqlcgen.MarkDeadLetterAsRetriedParams{
		ID:         pgtype.UUID{Bytes: dlID, Valid: true},
		ReviewedBy: sql.Null[string]{V: reviewedBy, Valid: true},
	}
	rows, err := qtx.MarkDeadLetterAsRetried(ctx, markParams)
	if err != nil {
		return "", fmt.Errorf("failed to mark dead letter as retried: %w", err)
	}
	if rows == 0 {
		return "", domain.ErrDeadLetterNotFound
	}

	return newJobUUID.String(), nil
}
