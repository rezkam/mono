package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rezkam/mono/internal/application/worker"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres/sqlcgen"
)

// PostgresCoordinator implements worker.GenerationCoordinator using PostgreSQL.
type PostgresCoordinator struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

// NewPostgresCoordinator creates a new PostgreSQL-backed coordinator.
func NewPostgresCoordinator(pool *pgxpool.Pool) *PostgresCoordinator {
	return &PostgresCoordinator{
		pool:    pool,
		queries: sqlcgen.New(pool),
	}
}

// === Job Insertion ===

func (c *PostgresCoordinator) InsertJob(ctx context.Context, job *domain.GenerationJob) error {
	params := domainJobToInsertParams(job)
	_, err := c.queries.InsertGenerationJob(ctx, params)
	if err != nil {
		// pgx.ErrNoRows means ON CONFLICT DO NOTHING triggered - job already exists
		if errors.Is(err, pgx.ErrNoRows) {
			slog.InfoContext(ctx, "job already exists for template, skipping",
				"job_id", job.ID,
				"template_id", job.TemplateID)
			return nil // Not an error - idempotent behavior
		}
		slog.ErrorContext(ctx, "failed to insert generation job",
			"job_id", job.ID,
			"template_id", job.TemplateID,
			"scheduled_for", job.ScheduledFor,
			"error", err)
	}
	return err
}

func (c *PostgresCoordinator) InsertMany(ctx context.Context, jobs []*domain.GenerationJob) error {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin transaction for batch job insert",
			"batch_size", len(jobs),
			"error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := c.queries.WithTx(tx)
	for i, job := range jobs {
		params := domainJobToInsertParams(job)
		_, err := qtx.InsertGenerationJob(ctx, params)
		if err != nil {
			// pgx.ErrNoRows means ON CONFLICT DO NOTHING triggered - skip this job
			if errors.Is(err, pgx.ErrNoRows) {
				slog.InfoContext(ctx, "job already exists for template in batch, skipping",
					"job_id", job.ID,
					"template_id", job.TemplateID,
					"batch_position", i)
				continue // Not an error - idempotent behavior
			}
			slog.ErrorContext(ctx, "failed to insert job in batch",
				"job_id", job.ID,
				"template_id", job.TemplateID,
				"batch_position", i,
				"batch_size", len(jobs),
				"error", err)
			return fmt.Errorf("failed to insert job %s: %w", job.ID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit batch job insert transaction",
			"batch_size", len(jobs),
			"error", err)
		return err
	}

	slog.InfoContext(ctx, "batch job insertion completed",
		"batch_size", len(jobs))

	return nil
}

// === Job Claiming & Processing ===

func (c *PostgresCoordinator) ClaimNextJob(ctx context.Context, workerID string, availabilityTimeout time.Duration) (*domain.GenerationJob, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin transaction for job claim",
			"worker_id", workerID,
			"error", err)
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := c.queries.WithTx(tx)

	// Claim the next available job using SKIP LOCKED
	row, err := qtx.ClaimNextPendingJob(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // No jobs available - not an error
		}
		slog.ErrorContext(ctx, "failed to claim next job",
			"worker_id", workerID,
			"error", err)
		return nil, fmt.Errorf("failed to claim job: %w", err)
	}

	// Mark as running with worker ownership
	availableAt := time.Now().UTC().Add(availabilityTimeout)
	markParams := sqlcgen.MarkJobAsRunningParams{
		ID:          row.ID,
		ClaimedBy:   sql.Null[string]{V: workerID, Valid: true},
		AvailableAt: timeToTimestamptz(availableAt),
	}
	_, err = qtx.MarkJobAsRunning(ctx, markParams)
	if err != nil {
		slog.ErrorContext(ctx, "failed to mark job as running",
			"job_id", row.ID,
			"worker_id", workerID,
			"error", err)
		return nil, fmt.Errorf("failed to mark job as running: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit job claim transaction",
			"job_id", row.ID,
			"worker_id", workerID,
			"error", err)
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Convert to domain model
	job := &domain.GenerationJob{
		ID:            row.ID,
		TemplateID:    row.TemplateID,
		GenerateFrom:  row.GenerateFrom,
		GenerateUntil: row.GenerateUntil,
		ScheduledFor:  row.ScheduledFor,
		RetryCount:    int(row.RetryCount),
		CreatedAt:     row.CreatedAt,
		AvailableAt:   availableAt,
		ClaimedBy:     &workerID,
		ClaimedAt:     func() *time.Time { now := time.Now().UTC(); return &now }(),
	}

	return job, nil
}

func (c *PostgresCoordinator) ExtendAvailability(ctx context.Context, jobID, workerID string, extension time.Duration) error {
	newAvailableAt := time.Now().UTC().Add(extension)
	params := sqlcgen.ExtendJobAvailabilityParams{
		ID:          jobID,
		ClaimedBy:   sql.Null[string]{V: workerID, Valid: true},
		AvailableAt: timeToTimestamptz(newAvailableAt),
	}

	rows, err := c.queries.ExtendJobAvailability(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to extend availability: %w", err)
	}
	if rows == 0 {
		return domain.ErrJobOwnershipLost
	}
	return nil
}

// === Job Completion ===

func (c *PostgresCoordinator) CompleteJob(ctx context.Context, jobID, workerID string) error {
	params := sqlcgen.CompleteJobWithOwnershipCheckParams{
		ID:        jobID,
		ClaimedBy: sql.Null[string]{V: workerID, Valid: true},
	}

	rows, err := c.queries.CompleteJobWithOwnershipCheck(ctx, params)
	if err != nil {
		slog.ErrorContext(ctx, "failed to complete job",
			"job_id", jobID,
			"worker_id", workerID,
			"error", err)
		return fmt.Errorf("failed to complete job: %w", err)
	}
	if rows == 0 {
		slog.WarnContext(ctx, "lost job ownership during completion",
			"job_id", jobID,
			"worker_id", workerID)
		return domain.ErrJobOwnershipLost
	}
	return nil
}

func (c *PostgresCoordinator) FailJob(ctx context.Context, jobID, workerID, errMsg string, cfg worker.RetryConfig) (willRetry bool, err error) {
	// Fetch current job to check retry count
	job, err := c.queries.FindGenerationJobByID(ctx, jobID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to fetch job for failure handling",
			"job_id", jobID,
			"worker_id", workerID,
			"error", err)
		return false, fmt.Errorf("failed to get job: %w", err)
	}

	newRetryCount := job.RetryCount + 1

	// Check if we've exhausted retries
	if newRetryCount > int32(cfg.MaxRetries) {
		slog.WarnContext(ctx, "job exhausted retries, moving to dead letter queue",
			"job_id", jobID,
			"template_id", job.TemplateID,
			"worker_id", workerID,
			"retry_count", newRetryCount,
			"max_retries", cfg.MaxRetries,
			"error", errMsg)

		// Max retries exceeded - atomically move to DLQ and discard
		tx, err := c.pool.Begin(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "failed to begin transaction for DLQ move",
				"job_id", jobID,
				"error", err)
			return false, fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		qtx := c.queries.WithTx(tx)

		// Convert to domain model for DLQ insertion
		domainJob := &domain.GenerationJob{
			ID:            job.ID,
			TemplateID:    job.TemplateID,
			GenerateFrom:  job.GenerateFrom,
			GenerateUntil: job.GenerateUntil,
			ScheduledFor:  job.ScheduledFor,
			RetryCount:    int(newRetryCount), // Use incremented count
			CreatedAt:     job.CreatedAt,
			ClaimedBy:     &workerID, // Include worker ID for DLQ tracking
		}

		// 1. Move to dead letter queue
		if err := c.moveJobToDeadLetterTx(ctx, qtx, domainJob, "exhausted", errMsg); err != nil {
			slog.ErrorContext(ctx, "failed to insert job into dead letter queue",
				"job_id", jobID,
				"template_id", job.TemplateID,
				"error", err)
			return false, fmt.Errorf("failed to move to dead letter: %w", err)
		}

		// 2. Discard job with ownership verification
		discardParams := sqlcgen.DiscardJobWithOwnershipCheckParams{
			ID:           jobID,
			ClaimedBy:    sql.Null[string]{V: workerID, Valid: true},
			ErrorMessage: sql.Null[string]{V: errMsg, Valid: true},
		}
		rows, err := qtx.DiscardJobWithOwnershipCheck(ctx, discardParams)
		if err != nil {
			slog.ErrorContext(ctx, "failed to discard job after DLQ insert",
				"job_id", jobID,
				"error", err)
			return false, fmt.Errorf("failed to discard job: %w", err)
		}
		if rows == 0 {
			// Lost ownership during transaction - job was reclaimed by another worker
			slog.WarnContext(ctx, "lost job ownership during DLQ transaction",
				"job_id", jobID,
				"worker_id", workerID)
			return false, domain.ErrJobOwnershipLost
		}

		// 3. Commit both operations atomically
		if err := tx.Commit(ctx); err != nil {
			slog.ErrorContext(ctx, "failed to commit DLQ transaction",
				"job_id", jobID,
				"error", err)
			return false, fmt.Errorf("failed to commit transaction: %w", err)
		}

		slog.InfoContext(ctx, "job moved to dead letter queue",
			"job_id", jobID,
			"template_id", job.TemplateID,
			"retry_count", newRetryCount)

		return false, nil // Job will not retry, moved to DLQ
	}

	// Calculate retry delay with exponential backoff + full jitter
	retryDelay := calculateRetryDelay(int(newRetryCount), cfg)
	scheduledFor := time.Now().UTC().Add(retryDelay)

	slog.InfoContext(ctx, "scheduling job retry",
		"job_id", jobID,
		"template_id", job.TemplateID,
		"worker_id", workerID,
		"retry_count", newRetryCount,
		"max_retries", cfg.MaxRetries,
		"retry_delay_seconds", retryDelay.Seconds(),
		"scheduled_for", scheduledFor,
		"error", errMsg)

	retryParams := sqlcgen.ScheduleJobRetryParams{
		ID:           jobID,
		RetryCount:   newRetryCount,
		ErrorMessage: sql.Null[string]{V: errMsg, Valid: true},
		ScheduledFor: scheduledFor,
		ClaimedBy:    sql.Null[string]{V: workerID, Valid: true},
	}

	rows, err := c.queries.ScheduleJobRetry(ctx, retryParams)
	if err != nil {
		slog.ErrorContext(ctx, "failed to schedule job retry",
			"job_id", jobID,
			"retry_count", newRetryCount,
			"error", err)
		return false, fmt.Errorf("failed to schedule retry: %w", err)
	}
	if rows == 0 {
		slog.WarnContext(ctx, "lost job ownership during retry scheduling",
			"job_id", jobID,
			"worker_id", workerID)
		return false, domain.ErrJobOwnershipLost
	}

	return true, nil
}

// === Job Cancellation ===

func (c *PostgresCoordinator) CancelJob(ctx context.Context, jobID string) error {
	// Try to cancel pending/scheduled job immediately
	rows, err := c.queries.CancelPendingJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to cancel pending job: %w", err)
	}
	if rows > 0 {
		return nil // Successfully cancelled pending job
	}

	// If not pending, try to request cancellation for running job
	rows, err = c.queries.RequestCancellationForRunningJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to request cancellation: %w", err)
	}
	if rows == 0 {
		return domain.ErrJobNotCancellable
	}

	// Notify workers about the cancellation request
	_, err = c.pool.Exec(ctx, "SELECT pg_notify('job_cancellations', $1)", jobID)
	if err != nil {
		// Log but don't fail - the job is already marked as cancelling,
		// worker will eventually detect via polling if NOTIFY fails
		slog.WarnContext(ctx, "failed to send cancellation notification",
			"job_id", jobID,
			"error", err,
		)
	}

	return nil
}

func (c *PostgresCoordinator) RequestCancellation(ctx context.Context, jobID string) (int64, error) {
	rows, err := c.queries.RequestCancellationForRunningJob(ctx, jobID)
	if err != nil {
		return 0, fmt.Errorf("failed to request cancellation: %w", err)
	}
	return rows, nil
}

func (c *PostgresCoordinator) MarkJobAsCancelled(ctx context.Context, jobID, workerID string) (int64, error) {
	params := sqlcgen.MarkJobAsCancelledParams{
		ID:        jobID,
		ClaimedBy: sql.Null[string]{V: workerID, Valid: true},
	}
	rows, err := c.queries.MarkJobAsCancelled(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("failed to mark job as cancelled: %w", err)
	}
	return rows, nil
}

func (c *PostgresCoordinator) SubscribeToCancellations(ctx context.Context) (<-chan string, error) {
	// Acquire a dedicated connection for LISTEN/NOTIFY
	conn, err := c.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}

	_, err = conn.Exec(ctx, "LISTEN job_cancellations")
	if err != nil {
		conn.Release()
		return nil, fmt.Errorf("failed to listen to channel: %w", err)
	}

	ch := make(chan string, 10)

	go func() {
		defer close(ch)
		defer conn.Release()
		defer func() {
			_, _ = conn.Exec(context.Background(), "UNLISTEN job_cancellations")
		}()

		for {
			notification, err := conn.Conn().WaitForNotification(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return // Context cancelled
				}
				continue
			}
			ch <- notification.Payload
		}
	}()

	return ch, nil
}

// === Dead Letter Queue ===

// moveJobToDeadLetterTx moves a job to dead letter queue within a transaction.
// Used internally by FailJob when max retries exhausted.
func (c *PostgresCoordinator) moveJobToDeadLetterTx(
	ctx context.Context,
	qtx *sqlcgen.Queries,
	job *domain.GenerationJob,
	errType string,
	errMsg string,
) error {
	jobID, err := uuid.Parse(job.ID)
	if err != nil {
		return fmt.Errorf("invalid job ID: %w", err)
	}

	templateID, err := uuid.Parse(job.TemplateID)
	if err != nil {
		return fmt.Errorf("invalid template ID: %w", err)
	}

	params := sqlcgen.InsertDeadLetterJobParams{
		OriginalJobID: pgtype.UUID{Bytes: jobID, Valid: true},
		TemplateID:    pgtype.UUID{Bytes: templateID, Valid: true},
		GenerateFrom:  timeToTimestamptz(job.GenerateFrom),
		GenerateUntil: timeToTimestamptz(job.GenerateUntil),
		ErrorType:     errType,
		ErrorMessage:  sql.Null[string]{V: errMsg, Valid: errMsg != ""},
		StackTrace:    sql.Null[string]{}, // No stack trace for exhausted retries
		RetryCount:    int32(job.RetryCount),
		LastWorkerID: sql.Null[string]{V: func() string {
			if job.ClaimedBy != nil {
				return *job.ClaimedBy
			}
			return ""
		}(), Valid: job.ClaimedBy != nil},
		OriginalScheduledFor: timeToTimestamptz(job.ScheduledFor),
		OriginalCreatedAt:    timeToTimestamptz(job.CreatedAt),
	}

	return qtx.InsertDeadLetterJob(ctx, params)
}

func (c *PostgresCoordinator) MoveToDeadLetter(ctx context.Context, job *domain.GenerationJob, workerID, errType, errMsg string, stackTrace *string) error {
	slog.WarnContext(ctx, "moving job to dead letter queue",
		"job_id", job.ID,
		"template_id", job.TemplateID,
		"worker_id", workerID,
		"error_type", errType,
		"retry_count", job.RetryCount)

	// Begin transaction for atomicity: both DLQ insert and job discard must succeed together
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin transaction for dead letter move",
			"job_id", job.ID,
			"template_id", job.TemplateID,
			"error_type", errType,
			"error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := c.queries.WithTx(tx)

	// 1. Insert into dead letter queue
	jobID, err := uuid.Parse(job.ID)
	if err != nil {
		slog.ErrorContext(ctx, "invalid job ID format",
			"job_id", job.ID,
			"error", err)
		return fmt.Errorf("invalid job ID: %w", err)
	}

	templateID, err := uuid.Parse(job.TemplateID)
	if err != nil {
		slog.ErrorContext(ctx, "invalid template ID format",
			"template_id", job.TemplateID,
			"error", err)
		return fmt.Errorf("invalid template ID: %w", err)
	}

	params := sqlcgen.InsertDeadLetterJobParams{
		OriginalJobID: pgtype.UUID{Bytes: jobID, Valid: true},
		TemplateID:    pgtype.UUID{Bytes: templateID, Valid: true},
		GenerateFrom:  timeToTimestamptz(job.GenerateFrom),
		GenerateUntil: timeToTimestamptz(job.GenerateUntil),
		ErrorType:     errType,
		ErrorMessage:  sql.Null[string]{V: errMsg, Valid: errMsg != ""},
		StackTrace: func() sql.Null[string] {
			if stackTrace != nil {
				return sql.Null[string]{V: *stackTrace, Valid: true}
			}
			return sql.Null[string]{}
		}(),
		LastWorkerID:         sql.Null[string]{V: workerID, Valid: true},
		RetryCount:           int32(job.RetryCount),
		OriginalScheduledFor: timeToTimestamptz(job.ScheduledFor),
		OriginalCreatedAt:    timeToTimestamptz(job.CreatedAt),
	}

	if err := qtx.InsertDeadLetterJob(ctx, params); err != nil {
		slog.ErrorContext(ctx, "failed to insert into dead letter queue",
			"job_id", job.ID,
			"template_id", job.TemplateID,
			"error_type", errType,
			"error", err)
		return fmt.Errorf("failed to insert dead letter job: %w", err)
	}

	// 2. Discard original job with ownership check
	discardParams := sqlcgen.DiscardJobWithOwnershipCheckParams{
		ID:           job.ID,
		ClaimedBy:    sql.Null[string]{V: workerID, Valid: true},
		ErrorMessage: sql.Null[string]{V: errMsg, Valid: errMsg != ""},
	}
	rows, err := qtx.DiscardJobWithOwnershipCheck(ctx, discardParams)
	if err != nil {
		slog.ErrorContext(ctx, "failed to discard job after DLQ insert",
			"job_id", job.ID,
			"error", err)
		return fmt.Errorf("failed to discard job: %w", err)
	}
	if rows == 0 {
		slog.WarnContext(ctx, "lost job ownership during dead letter move",
			"job_id", job.ID,
			"worker_id", workerID)
		return domain.ErrJobOwnershipLost
	}

	// 3. Commit both operations atomically
	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit dead letter transaction",
			"job_id", job.ID,
			"error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	slog.InfoContext(ctx, "job moved to dead letter queue",
		"job_id", job.ID,
		"template_id", job.TemplateID,
		"error_type", errType,
		"worker_id", workerID)

	return nil
}

func (c *PostgresCoordinator) ListDeadLetterJobs(ctx context.Context, limit int) ([]*domain.DeadLetterJob, error) {
	rows, err := c.queries.ListPendingDeadLetterJobs(ctx, int32(limit))
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

func (c *PostgresCoordinator) RetryDeadLetterJob(ctx context.Context, deadLetterID string) (newJobID string, err error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := c.queries.WithTx(tx)

	newJobID, err = retryDeadLetterJobTx(ctx, qtx, deadLetterID)
	if err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return newJobID, nil
}

func (c *PostgresCoordinator) DiscardDeadLetterJob(ctx context.Context, deadLetterID, note string) error {
	dlID, err := uuid.Parse(deadLetterID)
	if err != nil {
		return fmt.Errorf("invalid dead letter ID: %w", err)
	}

	params := sqlcgen.MarkDeadLetterAsDiscardedParams{
		ID:           pgtype.UUID{Bytes: dlID, Valid: true},
		ReviewedBy:   sql.Null[string]{Valid: false},
		ReviewerNote: sql.Null[string]{V: note, Valid: note != ""},
	}

	rows, err := c.queries.MarkDeadLetterAsDiscarded(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to mark dead letter as discarded: %w", err)
	}
	if rows == 0 {
		return domain.ErrDeadLetterNotFound
	}

	return nil
}

// === Exclusive Execution ===

func (c *PostgresCoordinator) TryAcquireExclusiveRun(ctx context.Context, runType string, holderID string, leaseDuration time.Duration) (release func(), acquired bool, err error) {
	expiresAt := time.Now().UTC().Add(leaseDuration)

	params := sqlcgen.TryAcquireLeaseParams{
		RunType:   runType,
		HolderID:  holderID,
		ExpiresAt: timeToTimestamptz(expiresAt),
	}

	lease, err := c.queries.TryAcquireLease(ctx, params)
	if err != nil {
		// No rows means lease is held by another worker - this is normal contention
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to acquire lease: %w", err)
	}

	// Check if we acquired the lease
	if lease.HolderID != holderID {
		return nil, false, nil
	}

	// Create release function
	releaseFunc := func() {
		releaseParams := sqlcgen.ReleaseLeaseParams{
			RunType:  runType,
			HolderID: holderID,
		}
		_, _ = c.queries.ReleaseLease(context.Background(), releaseParams)
	}

	return releaseFunc, true, nil
}

// === Helper Functions ===

func domainJobToInsertParams(job *domain.GenerationJob) sqlcgen.InsertGenerationJobParams {
	return sqlcgen.InsertGenerationJobParams{
		ID:            job.ID,
		TemplateID:    job.TemplateID,
		GenerateFrom:  job.GenerateFrom,
		GenerateUntil: job.GenerateUntil,
		ScheduledFor:  job.ScheduledFor,
		Status:        "pending",
		RetryCount:    int32(job.RetryCount),
		CreatedAt:     job.CreatedAt,
	}
}

func sqlcDeadLetterToDomain(row sqlcgen.DeadLetterJob) *domain.DeadLetterJob {
	// Handle nullable original_job_id (may be NULL if original job was cleaned up)
	originalJobID := ""
	if row.OriginalJobID.Valid {
		originalJobID = uuid.UUID(row.OriginalJobID.Bytes).String()
	}

	return &domain.DeadLetterJob{
		ID:            uuid.UUID(row.ID.Bytes).String(),
		OriginalJobID: originalJobID,
		TemplateID:    uuid.UUID(row.TemplateID.Bytes).String(),
		GenerateFrom:  timestamptzToTime(row.GenerateFrom),
		GenerateUntil: timestamptzToTime(row.GenerateUntil),
		ErrorType:     row.ErrorType,
		ErrorMessage:  row.ErrorMessage.V,
		StackTrace: func() *string {
			if row.StackTrace.Valid {
				v := row.StackTrace.V
				return &v
			}
			return nil
		}(),
		FailedAt:   timestamptzToTime(row.FailedAt),
		RetryCount: int(row.RetryCount),
		LastWorkerID: func() string {
			if row.LastWorkerID.Valid {
				return row.LastWorkerID.V
			}
			return ""
		}(),
		ResolvedAt: func() *time.Time {
			if row.ReviewedAt.Valid {
				t := timestamptzToTime(row.ReviewedAt)
				return &t
			}
			return nil
		}(),
		ResolvedBy: func() *string {
			if row.ReviewedBy.Valid {
				v := row.ReviewedBy.V
				return &v
			}
			return nil
		}(),
		Resolution: func() *string {
			if row.Resolution.Valid {
				v := row.Resolution.V
				return &v
			}
			return nil
		}(),
	}
}

func timestamptzToTime(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time.UTC()
}

// calculateRetryDelay computes exponential backoff with full jitter.
// Formula: random(0, min(max_delay, base_delay * 2^attempt))
func calculateRetryDelay(attempt int, cfg worker.RetryConfig) time.Duration {
	// Calculate exponential backoff: base * 2^attempt
	backoff := float64(cfg.BaseDelay) * math.Pow(2, float64(attempt-1))

	// Cap at max delay
	if backoff > float64(cfg.MaxDelay) {
		backoff = float64(cfg.MaxDelay)
	}

	// Full jitter: random(0, backoff)
	maxJitter := int64(backoff)
	if maxJitter <= 0 {
		return cfg.BaseDelay
	}

	jitter, err := rand.Int(rand.Reader, big.NewInt(maxJitter))
	if err != nil {
		// Fallback to base delay if random fails
		return cfg.BaseDelay
	}

	return time.Duration(jitter.Int64())
}
