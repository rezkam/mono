package worker

import (
	"context"
	"time"

	"github.com/rezkam/mono/internal/domain"
)

// GenerationCoordinator manages background job execution for recurring task generation.
// All methods are safe for concurrent use by multiple workers.
// Job claiming operations are atomic to prevent duplicate processing.
type GenerationCoordinator interface {
	// === Job Insertion ===

	// InsertJob inserts a single job.
	InsertJob(ctx context.Context, job *domain.GenerationJob) error

	// InsertMany inserts multiple jobs atomically.
	// All jobs succeed or fail together.
	InsertMany(ctx context.Context, jobs []*domain.GenerationJob) error

	// === Job Claiming & Processing ===

	// ClaimNextJob atomically claims the next available job.
	// Returns nil if no jobs are available.
	// The claimed job is locked to workerID for availabilityTimeout duration.
	// Safe for concurrent workers - each worker will claim a different job.
	ClaimNextJob(ctx context.Context, workerID string, availabilityTimeout time.Duration) (*domain.GenerationJob, error)

	// ExtendAvailability extends the lock duration for a running job.
	// Returns domain.ErrJobOwnershipLost if job is no longer claimed by this worker.
	// Used as heartbeat to prevent job reclamation while still processing.
	ExtendAvailability(ctx context.Context, jobID, workerID string, extension time.Duration) error

	// === Job Completion ===

	// CompleteJob marks a job as successfully completed.
	// Returns domain.ErrJobOwnershipLost if job is no longer claimed by this worker.
	CompleteJob(ctx context.Context, jobID, workerID string) error

	// FailJob marks a job as failed and schedules retry with increasing delays.
	// Returns domain.ErrJobOwnershipLost if job is no longer claimed by this worker.
	// If max retries exceeded, moves job to dead letter queue for manual review.
	// Returns true if job will be retried, false if moved to dead letter queue.
	FailJob(ctx context.Context, jobID, workerID, errMsg string, cfg RetryConfig) (willRetry bool, err error)

	// === Job Cancellation ===

	// CancelJob requests cancellation of a job by ID.
	// Pending jobs are cancelled immediately.
	// Running jobs receive cancellation signal and stop cooperatively.
	// Returns domain.ErrJobNotCancellable if job is already completed or failed.
	CancelJob(ctx context.Context, jobID string) error

	// SubscribeToCancellations returns a channel that receives cancelled job IDs.
	// Workers should select on this channel to handle cooperative cancellation.
	// The channel is closed when the provided context is cancelled.
	SubscribeToCancellations(ctx context.Context) (<-chan string, error)

	// === Dead Letter Queue ===

	// MoveToDeadLetter atomically moves a failed job to the dead letter queue.
	// The job is marked as permanently failed and requires manual intervention.
	// Returns domain.ErrJobOwnershipLost if job is no longer claimed by this worker.
	// errType: "permanent", "exhausted", or "panic"
	MoveToDeadLetter(ctx context.Context, job *domain.GenerationJob, workerID, errType, errMsg string, stackTrace *string) error

	// ListDeadLetterJobs returns unresolved dead letter jobs for manual review.
	// Results are limited to the specified count.
	ListDeadLetterJobs(ctx context.Context, limit int) ([]*domain.DeadLetterJob, error)

	// RetryDeadLetterJob creates a new job from a dead letter entry and marks it as retried.
	// Returns the ID of the newly created job.
	// Returns domain.ErrDeadLetterNotFound if the dead letter entry doesn't exist.
	RetryDeadLetterJob(ctx context.Context, deadLetterID string) (newJobID string, err error)

	// DiscardDeadLetterJob marks a dead letter job as permanently discarded.
	// Returns domain.ErrDeadLetterNotFound if the dead letter entry doesn't exist.
	DiscardDeadLetterJob(ctx context.Context, deadLetterID, note string) error

	// === Exclusive Execution ===

	// TryAcquireExclusiveRun attempts to acquire an exclusive execution lock.
	// Returns (releaseFunc, true, nil) if lock acquired successfully.
	// Returns (nil, false, nil) if lock is held by another process.
	// The lock automatically expires after leaseDuration for crash recovery.
	TryAcquireExclusiveRun(ctx context.Context, runType string, holderID string, leaseDuration time.Duration) (release func(), acquired bool, err error)
}

// RetryConfig configures retry behavior for failed jobs.
type RetryConfig struct {
	MaxRetries int           // Maximum retry attempts (default: 3)
	BaseDelay  time.Duration // Initial delay between retries (default: 1 minute)
	MaxDelay   time.Duration // Maximum delay cap (default: 1 hour)
}

// WorkerConfig configures worker process behavior.
type WorkerConfig struct {
	WorkerID            string        // Unique worker identifier (e.g., hostname-pid-uuid)
	Concurrency         int           // Max concurrent jobs (default: 10, must be > 0)
	AvailabilityTimeout time.Duration // Job reclaim timeout (default: 5min)
	HeartbeatInterval   time.Duration // Lock extension frequency (default: 1min, should be < AvailabilityTimeout)
	PollInterval        time.Duration // Job polling frequency (default: 1s)
	GenerationBatchDays int           // Days per generation batch (default: 100, range: 1-365)
	ErrorHandler        ErrorHandler  // Custom error/panic handler (default: DefaultErrorHandler)
	RetryConfig         RetryConfig   // Retry policy configuration
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
