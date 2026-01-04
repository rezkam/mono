package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/recurring"
)

// ReconciliationConfig holds configuration for the reconciliation worker.
// Follows Kubernetes controller-runtime patterns for rate limiting and backoff.
type ReconciliationConfig struct {
	// WorkerID is the unique identifier for this worker instance
	// Used for lease ownership verification
	WorkerID string

	// Interval between reconciliation runs (default: 15min)
	Interval time.Duration

	// MaxStartupJitter is the maximum random delay before first run (default: 30s)
	// Prevents thundering herd when multiple workers start simultaneously
	MaxStartupJitter time.Duration

	// TemplateGracePeriod skips templates updated within this window (default: 5min)
	// Newly created/updated templates have ASYNC jobs scheduled, no need to reconcile
	TemplateGracePeriod time.Duration

	// RateLimitDelay is the pause between processing each template (default: 100ms)
	// Prevents database overload when reconciling many templates
	RateLimitDelay time.Duration

	// BatchSize limits templates processed per run (default: 100, 0 = unlimited)
	// Prevents long-running reconciliation cycles
	BatchSize int

	// LeaseDuration is how long the exclusive lease is valid (default: 5min)
	LeaseDuration time.Duration

	// GenerationHorizonDays is how far ahead to generate recurring items (default: 365)
	// Templates are reconciled up to today + this many days
	GenerationHorizonDays int
}

// DefaultReconciliationConfig returns sensible defaults.
func DefaultReconciliationConfig(workerID string) ReconciliationConfig {
	return ReconciliationConfig{
		WorkerID:              workerID,
		Interval:              15 * time.Minute,
		MaxStartupJitter:      30 * time.Second,
		TemplateGracePeriod:   5 * time.Minute,
		RateLimitDelay:        100 * time.Millisecond,
		BatchSize:             100,
		LeaseDuration:         5 * time.Minute,
		GenerationHorizonDays: domain.DefaultGenerationHorizonDays,
	}
}

// ReconciliationWorker ensures templates are generated up to their horizon.
// Implements Kubernetes controller pattern: single-instance, level-triggered, rate-limited.
type ReconciliationWorker struct {
	coordinator GenerationCoordinator
	repo        Repository
	generator   *recurring.DomainGenerator
	cfg         ReconciliationConfig
}

const ReconciliationRunType = "recurring-template-reconciliation"

func NewReconciliationWorker(
	coordinator GenerationCoordinator,
	repo Repository,
	generator *recurring.DomainGenerator,
	cfg ReconciliationConfig,
) *ReconciliationWorker {
	return &ReconciliationWorker{
		coordinator: coordinator,
		repo:        repo,
		generator:   generator,
		cfg:         cfg,
	}
}

// Run starts the reconciliation loop with jittered startup.
func (w *ReconciliationWorker) Run(ctx context.Context) error {
	// Jittered startup delay to avoid thundering herd
	// When multiple workers start together (deploy, restart), they'd all try
	// to acquire the lock at the same instant without jitter
	if w.cfg.MaxStartupJitter > 0 {
		jitter := rand.N(w.cfg.MaxStartupJitter)
		slog.InfoContext(ctx, "reconciliation worker starting",
			"startup_jitter", jitter,
			"interval", w.cfg.Interval)

		timer := time.NewTimer(jitter)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	// Run immediately after jitter, then on interval
	if err := w.reconcileOnce(ctx); err != nil {
		slog.ErrorContext(ctx, "initial reconciliation failed", "error", err)
	}

	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "reconciliation worker stopping")
			return ctx.Err()
		case <-ticker.C:
			if err := w.reconcileOnce(ctx); err != nil {
				slog.ErrorContext(ctx, "reconciliation failed", "error", err)
			}
		}
	}
}

// reconcileOnce runs a single reconciliation cycle.
// Acquires exclusive lease, finds stale templates, and generates missing items.
func (w *ReconciliationWorker) reconcileOnce(ctx context.Context) error {
	startTime := time.Now().UTC()

	// Acquire exclusive lease for single-instance execution
	release, acquired, err := w.coordinator.TryAcquireExclusiveRun(
		ctx,
		ReconciliationRunType,
		w.cfg.WorkerID,
		w.cfg.LeaseDuration,
	)
	if err != nil {
		return fmt.Errorf("failed to acquire lease: %w", err)
	}
	if !acquired {
		slog.DebugContext(ctx, "reconciliation skipped, another instance holds the lease")
		return nil
	}
	defer release()

	// Find templates needing reconciliation
	// This query excludes:
	// - Templates with pending/running generation jobs
	// - Templates updated within grace period
	// - Templates already generated through their horizon
	targetDate := time.Now().UTC().AddDate(0, 0, w.cfg.GenerationHorizonDays)
	gracePeriodCutoff := time.Now().UTC().Add(-w.cfg.TemplateGracePeriod)

	templates, err := w.repo.FindStaleTemplatesForReconciliation(ctx, FindStaleParams{
		TargetDate:     targetDate,
		UpdatedBefore:  gracePeriodCutoff,
		ExcludePending: true, // Skip templates with pending/running jobs
		Limit:          w.cfg.BatchSize,
	})
	if err != nil {
		// Check if context was cancelled (shutdown)
		if errors.Is(err, context.Canceled) {
			slog.WarnContext(ctx, "reconciliation aborted: shutdown requested")
			return nil
		}
		return fmt.Errorf("failed to find stale templates: %w", err)
	}

	if len(templates) == 0 {
		slog.DebugContext(ctx, "reconciliation: no templates need processing")
		return nil
	}

	slog.InfoContext(ctx, "reconciliation started",
		"templates_to_process", len(templates),
		"target_date", targetDate.Format("2006-01-02"))

	var reconciled, skipped, failed int

	for _, template := range templates {
		// Check for context cancellation (graceful shutdown)
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "reconciliation interrupted",
				"reason", ctx.Err(),
				"processed", reconciled,
				"remaining", len(templates)-reconciled-skipped-failed)
			return nil // Don't return error - this is expected during shutdown
		default:
		}

		// Rate limiting between templates
		if w.cfg.RateLimitDelay > 0 && reconciled > 0 {
			time.Sleep(w.cfg.RateLimitDelay)
		}

		// Calculate desired state
		generateUntil := time.Now().UTC().AddDate(0, 0, template.GenerationHorizonDays)

		// Already at desired state?
		if !template.GeneratedThrough.Before(generateUntil) {
			skipped++
			continue
		}

		// Fetch exceptions for this template in the generation range
		exceptions, err := w.repo.FindExceptions(ctx, template.ID, template.GeneratedThrough, generateUntil)
		if err != nil {
			slog.ErrorContext(ctx, "reconciliation: failed to fetch exceptions",
				"template_id", template.ID,
				"error", err)
			failed++
			continue
		}

		// Generate items to reach desired state with exception filtering
		items, err := w.generator.GenerateTasksForTemplateWithExceptions(
			ctx,
			template,
			template.GeneratedThrough,
			generateUntil,
			exceptions,
		)
		if err != nil {
			slog.ErrorContext(ctx, "reconciliation: failed to generate tasks",
				"template_id", template.ID,
				"template_title", template.Title,
				"error", err)
			failed++
			continue
		}

		// Update generation marker even if no items (advances the window)
		if len(items) == 0 {
			if err := w.repo.SetGeneratedThrough(ctx, template.ID, generateUntil); err != nil {
				slog.ErrorContext(ctx, "reconciliation: failed to update marker",
					"template_id", template.ID,
					"error", err)
				failed++
			} else {
				reconciled++
			}
			continue
		}

		// Batch insert with conflict handling (idempotent)
		inserted, err := w.repo.BatchInsertItemsIgnoreConflict(ctx, items)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				slog.WarnContext(ctx, "reconciliation aborted during insert: shutdown requested")
				return nil
			}
			slog.ErrorContext(ctx, "reconciliation: failed to generate items",
				"template_id", template.ID,
				"template_title", template.Title,
				"error", err)
			failed++
			continue // Don't stop the loop, try other templates
		}

		// Update generation marker
		if err := w.repo.SetGeneratedThrough(ctx, template.ID, generateUntil); err != nil {
			if errors.Is(err, context.Canceled) {
				slog.WarnContext(ctx, "reconciliation aborted during marker update: shutdown requested")
				return nil
			}
			slog.ErrorContext(ctx, "reconciliation: failed to update marker",
				"template_id", template.ID,
				"error", err)
			failed++
			continue
		}

		reconciled++
		slog.DebugContext(ctx, "reconciliation: generated items",
			"template_id", template.ID,
			"items_generated", len(items),
			"items_inserted", inserted) // inserted <= generated due to dedup
	}

	duration := time.Since(startTime)
	slog.InfoContext(ctx, "reconciliation completed",
		"reconciled", reconciled,
		"skipped", skipped,
		"failed", failed,
		"duration", duration)

	return nil
}

// FindStaleParams holds parameters for finding templates that need reconciliation.
type FindStaleParams struct {
	TargetDate     time.Time // Templates with generated_through < this need work
	UpdatedBefore  time.Time // Skip templates updated after this (grace period)
	ExcludePending bool      // Skip templates with pending/running generation jobs
	Limit          int       // Max templates to return (0 = unlimited)
}
