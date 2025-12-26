package auth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/domain"
)

// Realistic timing constants based on actual DB performance (~100ms per operation)
const (
	// Realistic DB operation time
	realisticDBLatency = 100 * time.Millisecond

	// Slow DB operations for testing timeouts
	slowDBLatency = 2 * time.Second

	// Very slow operations for testing hard cancellation
	verySlowDBLatency = 10 * time.Second

	// Per-operation timeout (should be > realistic latency)
	realisticOperationTimeout = 500 * time.Millisecond

	// Short operation timeout for testing timeout behavior
	shortOperationTimeout = 200 * time.Millisecond

	// Shutdown timeout for normal cases
	normalShutdownTimeout = 10 * time.Second

	// Short shutdown timeout for testing cancellation
	shortShutdownTimeout = 300 * time.Millisecond
)

// mockRepository is a configurable mock for testing.
type mockRepository struct {
	mu sync.Mutex

	// Tracking
	updateLastUsedCalls   []updateLastUsedCall
	findByShortTokenCalls []string
	createCalls           []*domain.APIKey

	// Behavior configuration
	updateLastUsedDelay time.Duration
	updateLastUsedErr   error
	findByShortTokenFn  func(ctx context.Context, shortToken string) (*domain.APIKey, error)
	createErr           error

	// Counters for concurrent access verification
	updateLastUsedCount atomic.Int64
	cancelledCount      atomic.Int64
}

type updateLastUsedCall struct {
	KeyID     string
	Timestamp time.Time
}

func newMockRepository() *mockRepository {
	return &mockRepository{}
}

func (m *mockRepository) FindByShortToken(ctx context.Context, shortToken string) (*domain.APIKey, error) {
	m.mu.Lock()
	m.findByShortTokenCalls = append(m.findByShortTokenCalls, shortToken)
	m.mu.Unlock()

	if m.findByShortTokenFn != nil {
		return m.findByShortTokenFn(ctx, shortToken)
	}
	return nil, domain.ErrNotFound
}

func (m *mockRepository) UpdateLastUsed(ctx context.Context, keyID string, timestamp time.Time) error {
	m.updateLastUsedCount.Add(1)

	// Simulate delay if configured
	if m.updateLastUsedDelay > 0 {
		select {
		case <-time.After(m.updateLastUsedDelay):
			// Delay completed
		case <-ctx.Done():
			m.cancelledCount.Add(1)
			return ctx.Err()
		}
	}

	// Check if context was cancelled
	if ctx.Err() != nil {
		m.cancelledCount.Add(1)
		return ctx.Err()
	}

	m.mu.Lock()
	m.updateLastUsedCalls = append(m.updateLastUsedCalls, updateLastUsedCall{
		KeyID:     keyID,
		Timestamp: timestamp,
	})
	m.mu.Unlock()

	return m.updateLastUsedErr
}

func (m *mockRepository) Create(ctx context.Context, key *domain.APIKey) error {
	m.mu.Lock()
	m.createCalls = append(m.createCalls, key)
	m.mu.Unlock()
	return m.createErr
}

func (m *mockRepository) getUpdateLastUsedCalls() []updateLastUsedCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]updateLastUsedCall, len(m.updateLastUsedCalls))
	copy(result, m.updateLastUsedCalls)
	return result
}

// =============================================================================
// HAPPY PATH TESTS
// =============================================================================

func TestAuthenticator_Shutdown_EmptyQueue(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  10,
		OperationTimeout: realisticOperationTimeout,
	})

	// Immediate shutdown with empty queue
	ctx, cancel := context.WithTimeout(context.Background(), normalShutdownTimeout)
	defer cancel()

	err := auth.Shutdown(ctx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify no updates were processed (queue was empty)
	calls := repo.getUpdateLastUsedCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

func TestAuthenticator_Shutdown_WithPendingUpdates(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	// Simulate realistic DB latency
	repo.updateLastUsedDelay = realisticDBLatency

	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  100,
		OperationTimeout: realisticOperationTimeout,
	})

	// Queue some updates
	numUpdates := 5
	for i := 0; i < numUpdates; i++ {
		auth.lastUsedUpdates <- lastUsedUpdate{
			keyID:     "key-" + string(rune('0'+i)),
			timestamp: time.Now().UTC(),
		}
	}

	// Give worker time to start processing
	time.Sleep(150 * time.Millisecond)

	// Shutdown with generous timeout (should drain all)
	ctx, cancel := context.WithTimeout(context.Background(), normalShutdownTimeout)
	defer cancel()

	err := auth.Shutdown(ctx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// All updates should have been processed
	calls := repo.getUpdateLastUsedCalls()
	if len(calls) != numUpdates {
		t.Errorf("expected %d calls, got %d", numUpdates, len(calls))
	}
}

func TestAuthenticator_Shutdown_DrainsQueueBeforeReturning(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	// Fast but realistic DB operations
	repo.updateLastUsedDelay = 50 * time.Millisecond

	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  100,
		OperationTimeout: realisticOperationTimeout,
	})

	// Queue updates
	numUpdates := 10
	for i := 0; i < numUpdates; i++ {
		auth.lastUsedUpdates <- lastUsedUpdate{
			keyID:     "key-drain-" + string(rune('a'+i%26)),
			timestamp: time.Now().UTC(),
		}
	}

	// Shutdown should drain all queued updates
	ctx, cancel := context.WithTimeout(context.Background(), normalShutdownTimeout)
	defer cancel()

	err := auth.Shutdown(ctx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	calls := repo.getUpdateLastUsedCalls()
	if len(calls) != numUpdates {
		t.Errorf("expected %d calls after drain, got %d", numUpdates, len(calls))
	}
}

// =============================================================================
// TIMEOUT AND CANCELLATION TESTS
// =============================================================================

func TestAuthenticator_Shutdown_Timeout_CancelsOperations(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	// Simulate slow database operations
	repo.updateLastUsedDelay = slowDBLatency

	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  100,
		OperationTimeout: 0, // No per-operation timeout, rely on shutdown
	})

	// Queue many updates that will take a long time
	for i := 0; i < 20; i++ {
		auth.lastUsedUpdates <- lastUsedUpdate{
			keyID:     "slow-key-" + string(rune('0'+i%10)),
			timestamp: time.Now().UTC(),
		}
	}

	// Shutdown with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), shortShutdownTimeout)
	defer cancel()

	start := time.Now()
	err := auth.Shutdown(ctx)
	elapsed := time.Since(start)

	// Should return with timeout error
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got: %v", err)
	}

	// Should return within timeout + margin
	maxExpected := shortShutdownTimeout + 200*time.Millisecond
	if elapsed > maxExpected {
		t.Errorf("shutdown took too long: %v (expected < %v)", elapsed, maxExpected)
	}

	// Some operations should have been cancelled
	cancelled := repo.cancelledCount.Load()
	t.Logf("Cancelled operations: %d", cancelled)
}

func TestAuthenticator_Shutdown_CancelsInFlightOperations(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	// Very slow operation to ensure we catch it mid-flight
	repo.updateLastUsedDelay = verySlowDBLatency

	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  10,
		OperationTimeout: 0, // No per-op timeout, rely on shutdown cancellation
	})

	// Queue one update
	auth.lastUsedUpdates <- lastUsedUpdate{
		keyID:     "in-flight-key",
		timestamp: time.Now().UTC(),
	}

	// Wait for operation to start
	time.Sleep(200 * time.Millisecond)

	// Verify operation started
	if repo.updateLastUsedCount.Load() != 1 {
		t.Fatal("expected operation to have started")
	}

	// Shutdown with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), shortShutdownTimeout)
	defer cancel()

	err := auth.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got: %v", err)
	}

	// Wait a bit for cancellation to propagate
	time.Sleep(100 * time.Millisecond)

	// Operation should have been cancelled
	if repo.cancelledCount.Load() != 1 {
		t.Errorf("expected 1 cancelled operation, got %d", repo.cancelledCount.Load())
	}

	// No successful completions (operation was cancelled)
	calls := repo.getUpdateLastUsedCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 successful calls, got %d", len(calls))
	}
}

func TestAuthenticator_Shutdown_OperationTimeout_Respected(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	// Operation takes longer than per-operation timeout
	repo.updateLastUsedDelay = slowDBLatency

	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  10,
		OperationTimeout: shortOperationTimeout, // Per-operation timeout
	})

	// Queue update
	auth.lastUsedUpdates <- lastUsedUpdate{
		keyID:     "timeout-key",
		timestamp: time.Now().UTC(),
	}

	// Give time for operation to timeout
	time.Sleep(shortOperationTimeout + 200*time.Millisecond)

	// Shutdown should complete quickly
	ctx, cancel := context.WithTimeout(context.Background(), normalShutdownTimeout)
	defer cancel()

	err := auth.Shutdown(ctx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Operation should have been cancelled due to per-operation timeout
	if repo.cancelledCount.Load() != 1 {
		t.Errorf("expected 1 cancelled operation, got %d", repo.cancelledCount.Load())
	}
}

// =============================================================================
// IDEMPOTENCY TESTS
// =============================================================================

func TestAuthenticator_Shutdown_Idempotent(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  10,
		OperationTimeout: realisticOperationTimeout,
	})

	ctx := context.Background()

	// First shutdown
	err1 := auth.Shutdown(ctx)
	if err1 != nil {
		t.Fatalf("first shutdown failed: %v", err1)
	}

	// Second shutdown should return immediately (sync.Once)
	start := time.Now()
	err2 := auth.Shutdown(ctx)
	elapsed := time.Since(start)

	if err2 != nil {
		t.Errorf("second shutdown returned error: %v", err2)
	}

	if elapsed > 50*time.Millisecond {
		t.Errorf("second shutdown took too long: %v (expected immediate)", elapsed)
	}

	// Third shutdown
	err3 := auth.Shutdown(ctx)
	if err3 != nil {
		t.Errorf("third shutdown returned error: %v", err3)
	}
}

func TestAuthenticator_Shutdown_ConcurrentCalls(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  10,
		OperationTimeout: realisticOperationTimeout,
	})

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)

	// Many concurrent shutdown calls
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), normalShutdownTimeout)
			defer cancel()
			if err := auth.Shutdown(ctx); err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// All calls should succeed (sync.Once ensures only one executes)
	for err := range errors {
		t.Errorf("shutdown returned error: %v", err)
	}
}

// =============================================================================
// CONCURRENCY AND RACE CONDITION TESTS
// =============================================================================

func TestAuthenticator_ConcurrentValidationAndShutdown(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	repo.findByShortTokenFn = func(ctx context.Context, shortToken string) (*domain.APIKey, error) {
		// Simulate realistic DB lookup
		select {
		case <-time.After(realisticDBLatency):
			return nil, domain.ErrNotFound
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  1000,
		OperationTimeout: realisticOperationTimeout,
	})

	var wg sync.WaitGroup
	const numValidators = 20
	shutdownStarted := make(chan struct{})

	// Start validators
	wg.Add(numValidators)
	for i := 0; i < numValidators; i++ {
		go func() {
			defer wg.Done()
			<-shutdownStarted // Wait for shutdown to start

			// Keep validating during shutdown
			for j := 0; j < 5; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), realisticOperationTimeout)
				_, _ = auth.ValidateAPIKey(ctx, "test_mono_v1_abc123.secret456")
				cancel()
			}
		}()
	}

	// Start shutdown
	go func() {
		close(shutdownStarted)
	}()

	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), normalShutdownTimeout)
	defer cancel()

	err := auth.Shutdown(ctx)
	// May or may not timeout depending on timing
	t.Logf("Shutdown result: %v", err)

	wg.Wait()
}

func TestAuthenticator_HighConcurrencyUpdates(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	// Very fast DB operations for this stress test (tests concurrency, not timing)
	repo.updateLastUsedDelay = 1 * time.Millisecond

	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  1000, // Smaller queue to test backpressure
		OperationTimeout: realisticOperationTimeout,
	})

	const numGoroutines = 20
	const updatesPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Many concurrent updates
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < updatesPerGoroutine; j++ {
				select {
				case auth.lastUsedUpdates <- lastUsedUpdate{
					keyID:     "concurrent-key",
					timestamp: time.Now().UTC(),
				}:
				default:
					// Queue full, skip (this is expected behavior)
				}
			}
		}(i)
	}

	wg.Wait()

	// Let worker process some updates
	time.Sleep(200 * time.Millisecond)

	// Shutdown with generous timeout for draining
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := auth.Shutdown(ctx)
	if err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	// Should have processed updates without panics or races
	calls := repo.getUpdateLastUsedCalls()
	t.Logf("Processed %d updates", len(calls))
}

func TestAuthenticator_ShutdownDuringQueueing(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	// Realistic DB latency
	repo.updateLastUsedDelay = realisticDBLatency

	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  100,
		OperationTimeout: realisticOperationTimeout,
	})

	var wg sync.WaitGroup
	stopQueueing := make(chan struct{})

	// Goroutine that keeps queueing updates
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopQueueing:
				return
			default:
				select {
				case auth.lastUsedUpdates <- lastUsedUpdate{
					keyID:     "queue-during-shutdown",
					timestamp: time.Now().UTC(),
				}:
				default:
					// Queue full
				}
				time.Sleep(10 * time.Millisecond) // Realistic queueing rate
			}
		}
	}()

	// Let it queue for a bit
	time.Sleep(200 * time.Millisecond)

	// Stop queueing before shutdown to allow drain
	close(stopQueueing)
	wg.Wait()

	// Now initiate shutdown
	ctx, cancel := context.WithTimeout(context.Background(), normalShutdownTimeout)
	defer cancel()

	err := auth.Shutdown(ctx)

	// Should complete without timeout
	if err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
}

func TestAuthenticator_ConcurrentQueueingDuringShutdown(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	// Realistic DB latency
	repo.updateLastUsedDelay = realisticDBLatency

	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  100,
		OperationTimeout: realisticOperationTimeout,
	})

	var wg sync.WaitGroup
	stopQueueing := make(chan struct{})

	// Goroutine that keeps queueing updates during shutdown
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopQueueing:
				return
			default:
				select {
				case auth.lastUsedUpdates <- lastUsedUpdate{
					keyID:     "concurrent-queue",
					timestamp: time.Now().UTC(),
				}:
				default:
					// Queue full - expected during heavy load
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()

	// Let it queue for a bit
	time.Sleep(100 * time.Millisecond)

	// Initiate shutdown while queueing continues
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := auth.Shutdown(ctx)
	close(stopQueueing)
	wg.Wait()

	// Either success or timeout is acceptable - the key is no panics/races
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Logf("Shutdown result: %v", err)
}

// =============================================================================
// RESOURCE CLEANUP TESTS
// =============================================================================

func TestAuthenticator_ResourcesReleasedAfterShutdown(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  10,
		OperationTimeout: realisticOperationTimeout,
	})

	// Shutdown
	ctx := context.Background()
	err := auth.Shutdown(ctx)
	if err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	// Verify opsCtx is cancelled (resources released)
	select {
	case <-auth.opsCtx.Done():
		// Expected - context should be cancelled
	default:
		t.Error("opsCtx should be cancelled after shutdown")
	}

	// Verify shutdownChan is closed
	select {
	case <-auth.shutdownChan:
		// Expected - channel should be closed
	default:
		t.Error("shutdownChan should be closed after shutdown")
	}
}

func TestAuthenticator_ShutdownTimeout_StillCleansUpResources(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	repo.updateLastUsedDelay = verySlowDBLatency // Very slow

	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  10,
		OperationTimeout: 0, // No per-op timeout
	})

	// Queue update that will be very slow
	auth.lastUsedUpdates <- lastUsedUpdate{
		keyID:     "very-slow",
		timestamp: time.Now().UTC(),
	}

	// Wait for operation to start
	time.Sleep(200 * time.Millisecond)

	// Shutdown with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), shortShutdownTimeout)
	defer cancel()

	err := auth.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got: %v", err)
	}

	// Resources should still be cleaned up despite timeout
	select {
	case <-auth.opsCtx.Done():
		// Expected - context should be cancelled even on timeout
	default:
		t.Error("opsCtx should be cancelled even after timeout")
	}
}

// =============================================================================
// EDGE CASE TESTS
// =============================================================================

func TestAuthenticator_ZeroQueueSize_UsesDefault(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize: 0, // Should use default
	})
	defer func() {
		ctx := context.Background()
		_ = auth.Shutdown(ctx)
	}()

	// Should have default queue size, not panic
	if cap(auth.lastUsedUpdates) != DefaultUpdateQueueSize {
		t.Errorf("expected queue size %d, got %d", DefaultUpdateQueueSize, cap(auth.lastUsedUpdates))
	}
}

func TestAuthenticator_NegativeTimeout_UsesDefault(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	auth := NewAuthenticator(repo, Config{
		OperationTimeout: -1 * time.Second, // Should use default
	})
	defer func() {
		ctx := context.Background()
		_ = auth.Shutdown(ctx)
	}()

	if auth.operationTimeout != DefaultOperationTimeout {
		t.Errorf("expected timeout %v, got %v", DefaultOperationTimeout, auth.operationTimeout)
	}
}

func TestAuthenticator_ZeroTimeout_MeansNoTimeout(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	auth := NewAuthenticator(repo, Config{
		OperationTimeout: 0, // Zero means no per-operation timeout
	})
	defer func() {
		ctx := context.Background()
		_ = auth.Shutdown(ctx)
	}()

	if auth.operationTimeout != 0 {
		t.Errorf("expected timeout 0, got %v", auth.operationTimeout)
	}
}

func TestAuthenticator_QueueFull_DropsUpdate(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	// Slow processing to fill queue
	repo.updateLastUsedDelay = slowDBLatency

	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  5, // Small queue
		OperationTimeout: 0, // No per-op timeout
	})

	// Fill the queue (worker is slow, so queue fills up)
	droppedCount := 0
	for i := 0; i < 10; i++ {
		select {
		case auth.lastUsedUpdates <- lastUsedUpdate{
			keyID:     "fill-queue",
			timestamp: time.Now().UTC(),
		}:
			// Queued
		default:
			// Expected - queue is full, update dropped
			droppedCount++
		}
	}

	t.Logf("Dropped %d updates (queue full) - expected behavior", droppedCount)

	// Shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), shortShutdownTimeout)
	defer cancel()

	err := auth.Shutdown(ctx)
	// May timeout since operations are slow
	t.Logf("Shutdown result: %v", err)
}

func TestAuthenticator_ShutdownWithCancelledContext(t *testing.T) {
	t.Parallel()

	repo := newMockRepository()
	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  10,
		OperationTimeout: realisticOperationTimeout,
	})

	// Already cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := auth.Shutdown(ctx)
	elapsed := time.Since(start)

	// Should return immediately with context error
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}

	if elapsed > 100*time.Millisecond {
		t.Errorf("should return immediately with cancelled context, took %v", elapsed)
	}
}

// =============================================================================
// STRESS TESTS
// =============================================================================

func TestAuthenticator_StressTest_ManyShutdownsAndRecreations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	repo := newMockRepository()
	// Fast operations for stress test
	repo.updateLastUsedDelay = 10 * time.Millisecond

	for i := 0; i < 20; i++ {
		auth := NewAuthenticator(repo, Config{
			UpdateQueueSize:  100,
			OperationTimeout: realisticOperationTimeout,
		})

		// Queue some updates
		for j := 0; j < 5; j++ {
			auth.lastUsedUpdates <- lastUsedUpdate{
				keyID:     "stress-key",
				timestamp: time.Now().UTC(),
			}
		}

		// Shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := auth.Shutdown(ctx)
		cancel()

		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
	}
}

func TestAuthenticator_StressTest_RapidFireUpdates(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	repo := newMockRepository()
	// Fast operations
	repo.updateLastUsedDelay = 5 * time.Millisecond

	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  10000,
		OperationTimeout: realisticOperationTimeout,
	})

	const numUpdates = 50000
	start := time.Now()

	// Rapid fire updates
	queued := 0
	for i := 0; i < numUpdates; i++ {
		select {
		case auth.lastUsedUpdates <- lastUsedUpdate{
			keyID:     "rapid-fire",
			timestamp: time.Now().UTC(),
		}:
			queued++
		default:
			// Queue full, expected
		}
	}

	elapsed := time.Since(start)
	t.Logf("Queued %d/%d updates in %v", queued, numUpdates, elapsed)

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err := auth.Shutdown(ctx)
	if err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	calls := repo.getUpdateLastUsedCalls()
	t.Logf("Processed %d updates", len(calls))
}

// =============================================================================
// BENCHMARKS
// =============================================================================

func BenchmarkAuthenticator_Shutdown_EmptyQueue(b *testing.B) {
	repo := newMockRepository()

	for b.Loop() {
		auth := NewAuthenticator(repo, Config{
			UpdateQueueSize:  100,
			OperationTimeout: realisticOperationTimeout,
		})

		ctx := context.Background()
		_ = auth.Shutdown(ctx)
	}
}

func BenchmarkAuthenticator_Shutdown_WithUpdates(b *testing.B) {
	repo := newMockRepository()
	// Fast operations for benchmark
	repo.updateLastUsedDelay = 1 * time.Millisecond

	for b.Loop() {
		auth := NewAuthenticator(repo, Config{
			UpdateQueueSize:  100,
			OperationTimeout: realisticOperationTimeout,
		})

		// Queue 5 updates
		for i := 0; i < 5; i++ {
			auth.lastUsedUpdates <- lastUsedUpdate{
				keyID:     "bench-key",
				timestamp: time.Now().UTC(),
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), normalShutdownTimeout)
		_ = auth.Shutdown(ctx)
		cancel()
	}
}

func BenchmarkAuthenticator_QueueUpdate(b *testing.B) {
	repo := newMockRepository()
	auth := NewAuthenticator(repo, Config{
		UpdateQueueSize:  b.N + 1000,
		OperationTimeout: realisticOperationTimeout,
	})
	defer func() {
		ctx := context.Background()
		_ = auth.Shutdown(ctx)
	}()

	update := lastUsedUpdate{
		keyID:     "bench-key",
		timestamp: time.Now().UTC(),
	}

	b.ResetTimer()
	for b.Loop() {
		select {
		case auth.lastUsedUpdates <- update:
		default:
		}
	}
}
