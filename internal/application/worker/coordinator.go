package worker

import (
	"context"
	"time"

	"github.com/rezkam/mono/internal/domain"
)

// GenerationCoordinator abstracts coordination for background generation workers.
// Implements industry-standard patterns: batch insertion, panic handling,
// job cancellation, and availability timeout for stuck job recovery.
//
// Design principles:
// - Availability timeout pattern (inspired by SQS visibility timeout) enables stuck job recovery
// - Worker ownership verification prevents race conditions when claiming jobs
// - Dead letter queue captures permanently failed jobs for manual review
type GenerationCoordinator interface {
	// === Job Insertion ===

	// InsertJob inserts a single job.
	InsertJob(ctx context.Context, job *domain.GenerationJob) error

	// InsertMany inserts multiple jobs in a single operation (atomic).
	// All jobs succeed or fail together. Reduces database round-trips.
	InsertMany(ctx context.Context, jobs []*domain.GenerationJob) error

	// === Job Claiming & Processing ===

	// ClaimNextJob atomically claims the next pending job OR stuck running job.
	// Sets claimed_by to workerID and available_at to now + availabilityTimeout.
	// Returns nil if no jobs available. Safe for concurrent workers.
	// Query: WHERE (status='pending' AND scheduled_for<=NOW()) OR (status='running' AND available_at<=NOW())
	ClaimNextJob(ctx context.Context, workerID string, availabilityTimeout time.Duration) (*domain.GenerationJob, error)

	// ExtendAvailability extends the availability timeout for a running job (heartbeat).
	// Only succeeds if job is still claimed by this worker.
	// Called periodically by worker to prevent job from being reclaimed.
	ExtendAvailability(ctx context.Context, jobID, workerID string, extension time.Duration) error

	// === Job Completion ===

	// CompleteJob marks a job as completed successfully.
	// Only succeeds if job is still claimed by this worker (ownership check).
	CompleteJob(ctx context.Context, jobID, workerID string) error

	// FailJob marks a job as failed and schedules retry with exponential backoff + full jitter.
	// Only succeeds if job is still claimed by this worker.
	// If max retries exceeded, moves to dead letter queue.
	// Returns true if job will be retried, false if sent to dead letter.
	FailJob(ctx context.Context, jobID, workerID, errMsg string, cfg RetryConfig) (willRetry bool, err error)

	// === Job Cancellation ===

	// CancelJob cancels a job by ID.
	// - Pending/scheduled jobs: immediately marked as cancelled
	// - Running jobs: sends cancellation signal via LISTEN/NOTIFY
	// - Completed/failed jobs: no-op
	CancelJob(ctx context.Context, jobID string) error

	// SubscribeToCancellations returns a channel that receives job IDs when cancelled.
	// Workers select on this channel to handle cooperative cancellation.
	// Channel is closed when context is cancelled.
	SubscribeToCancellations(ctx context.Context) (<-chan string, error)

	// === Dead Letter Queue ===

	// MoveToDeadLetter atomically moves a job to DLQ and marks it discarded.
	// Inserts into dead_letter_jobs and updates original job status in one transaction.
	// Returns ErrJobOwnershipLost if job is no longer claimed by this worker.
	// errType: "permanent", "exhausted", or "panic"
	MoveToDeadLetter(ctx context.Context, job *domain.GenerationJob, workerID, errType, errMsg string, stackTrace *string) error

	// ListDeadLetterJobs returns pending dead letter jobs for admin review.
	ListDeadLetterJobs(ctx context.Context, limit int) ([]*domain.DeadLetterJob, error)

	// RetryDeadLetterJob creates a new job from a dead letter entry.
	// Marks dead letter as resolved with "retried" resolution.
	RetryDeadLetterJob(ctx context.Context, deadLetterID, reviewedBy string) (newJobID string, err error)

	// DiscardDeadLetterJob marks a dead letter job as discarded.
	// Used when admin determines the job should not be retried.
	DiscardDeadLetterJob(ctx context.Context, deadLetterID, reviewedBy, note string) error

	// === Exclusive Execution ===

	// TryAcquireExclusiveRun attempts to acquire an exclusive lock for the given run type.
	// Returns (releaseFunc, true, nil) if acquired, (nil, false, nil) if another worker holds it.
	// Used by reconciliation worker to ensure only one instance runs at a time.
	// Uses cron_job_leases table with crash recovery via lease expiration.
	TryAcquireExclusiveRun(ctx context.Context, runType string, holderID string, leaseDuration time.Duration) (release func(), acquired bool, err error)
}

// RetryConfig holds retry policy parameters (exponential backoff + full jitter).
type RetryConfig struct {
	MaxRetries int           // Maximum retry attempts (default: 3)
	BaseDelay  time.Duration // Initial delay between retries (default: 1 minute)
	MaxDelay   time.Duration // Maximum delay cap (default: 1 hour)
}

// WorkerConfig holds worker process configuration.
type WorkerConfig struct {
	WorkerID            string        // Unique ID for this worker (e.g., hostname-pid-uuid)
	Concurrency         int           // Max concurrent jobs (default: 10)
	AvailabilityTimeout time.Duration // How long before stuck job is reclaimed (default: 5min)
	HeartbeatInterval   time.Duration // How often to extend availability (default: 1min)
	PollInterval        time.Duration // How often to poll for new jobs (default: 1s)
	GenerationBatchDays int           // Days per batch when generating large date ranges (default: 100)
	ErrorHandler        ErrorHandler  // Custom error/panic handler (optional)
	RetryConfig         RetryConfig
}

// DefaultRetryConfig returns default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  time.Minute,
		MaxDelay:   time.Hour,
	}
}

// DefaultWorkerConfig returns default worker configuration.
func DefaultWorkerConfig(workerID string) WorkerConfig {
	return WorkerConfig{
		WorkerID:            workerID,
		Concurrency:         10,
		AvailabilityTimeout: 5 * time.Minute,
		HeartbeatInterval:   time.Minute,
		PollInterval:        time.Second,
		GenerationBatchDays: 100,
		ErrorHandler:        &DefaultErrorHandler{},
		RetryConfig:         DefaultRetryConfig(),
	}
}
