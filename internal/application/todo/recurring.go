package todo

import (
	"context"
	"time"

	"github.com/rezkam/mono/internal/domain"
)

// RecurringOperations extends Repository with minimal operations needed
// for recurring template provisioning (create/update with sync generation).
//
// This interface is ONLY used within AtomicRecurring callbacks.
// Pure todo operations should use the standard Atomic method with Repository.
type RecurringOperations interface {
	Repository // All standard todo operations

	// Recurring template provisioning operations
	BatchInsertItemsIgnoreConflict(ctx context.Context, items []*domain.TodoItem) (int, error)
	DeleteFuturePendingItems(ctx context.Context, templateID string, from time.Time) (int64, error)
	SetGeneratedThrough(ctx context.Context, templateID string, generatedThrough time.Time) error
	ScheduleGenerationJob(ctx context.Context, templateID string, scheduledFor, from, until time.Time) (string, error)
}
