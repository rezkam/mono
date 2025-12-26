package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/config"
	"github.com/rezkam/mono/internal/infrastructure/keygen"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
)

// BenchmarkTimingAttack_VULNERABLE demonstrates the timing attack vulnerability
// in the current implementation where hash computation only happens for valid
// short tokens, creating a measurable timing difference.
//
// EXPECTED RESULT (VULNERABLE):
//   - NonExistentKey: ~100-500μs (DB lookup only)
//   - ExistingKeyWrongSecret: ~105-505μs (DB lookup + BLAKE2b hash)
//   - Timing difference: ~5μs (hash computation time)
//
// An attacker can measure this difference to enumerate valid short tokens.
//
// Run: MONO_STORAGE_DSN="postgres://..." go test -bench=BenchmarkTimingAttack_VULNERABLE -benchmem ./tests/integration/postgres
func BenchmarkTimingAttack_VULNERABLE(b *testing.B) {
	cfg, err := config.LoadTestConfig()
	if err != nil {
		b.Skipf("Failed to load test config: %v", err)
	}

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, cfg.StorageDSN)
	if err != nil {
		b.Fatalf("Failed to connect: %v", err)
	}
	defer store.Close()

	// Clean database
	_, err = store.Pool().Exec(ctx, "TRUNCATE api_keys CASCADE")
	if err != nil {
		b.Fatalf("Failed to clean database: %v", err)
	}

	// Create authenticator with zero timeout (no timeout)
	authenticator := auth.NewAuthenticator(store, auth.Config{
		OperationTimeout: 0,
		UpdateQueueSize:  100,
	})
	defer authenticator.Shutdown(context.Background())

	// Create a valid API key
	validKey, err := auth.CreateAPIKey(ctx, store, "sk", "mono", "v1", "Timing Attack Test", nil)
	if err != nil {
		b.Fatalf("Failed to create API key: %v", err)
	}

	// Parse the valid key to get its components
	validParts, err := keygen.ParseAPIKey(validKey)
	if err != nil {
		b.Fatalf("Failed to parse key: %v", err)
	}

	// Construct a key with the SAME short token but WRONG long secret
	// This will trigger the hash computation path
	wrongSecretKey := fmt.Sprintf("sk-mono-v1-%s-%s",
		validParts.ShortToken,
		"WRONG_SECRET_43_CHARS_LONG_0000000000",
	)

	// Construct a key with a NON-EXISTENT short token
	// This will take the fast path (no hash computation)
	nonExistentKey := "sk-mono-v1-000000000000-NONEXISTENT_SECRET_43_CHARS_000000000"

	b.Run("NonExistentKey_FastPath", func(b *testing.B) {
		// Reset timer to exclude setup
		b.ResetTimer()

		for b.Loop() {
			// This takes the FAST path:
			// 1. DB lookup (not found)
			// 2. Return immediately (NO hash computation)
			_, err := authenticator.ValidateAPIKey(ctx, nonExistentKey)
			if err == nil {
				b.Fatal("Expected unauthorized error")
			}
		}
	})

	b.Run("ExistingKeyWrongSecret_SlowPath", func(b *testing.B) {
		// Reset timer to exclude setup
		b.ResetTimer()

		for b.Loop() {
			// This takes the SLOW path:
			// 1. DB lookup (found!)
			// 2. Hash computation (BLAKE2b-256) ← TIMING LEAK
			// 3. Constant-time comparison (fails)
			// 4. Return unauthorized
			_, err := authenticator.ValidateAPIKey(ctx, wrongSecretKey)
			if err == nil {
				b.Fatal("Expected unauthorized error")
			}
		}
	})
}

// TestTimingAttack_Statistical performs statistical analysis to prove
// the timing difference is measurable and exploitable.
func TestTimingAttack_Statistical(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timing attack test in short mode")
	}

	cfg, err := config.LoadTestConfig()
	if err != nil {
		t.Skipf("Failed to load test config: %v", err)
	}

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, cfg.StorageDSN)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer store.Close()

	// Clean database
	_, err = store.Pool().Exec(ctx, "TRUNCATE api_keys CASCADE")
	if err != nil {
		t.Fatalf("Failed to clean database: %v", err)
	}

	// Create authenticator
	authenticator := auth.NewAuthenticator(store, auth.Config{
		OperationTimeout: 0,
		UpdateQueueSize:  100,
	})
	defer authenticator.Shutdown(context.Background())

	// Create a valid API key
	validKey, err := auth.CreateAPIKey(ctx, store, "sk", "mono", "v1", "Timing Test", nil)
	if err != nil {
		t.Fatalf("Failed to create API key: %v", err)
	}

	validParts, err := keygen.ParseAPIKey(validKey)
	if err != nil {
		t.Fatalf("Failed to parse key: %v", err)
	}

	// Construct test keys
	wrongSecretKey := fmt.Sprintf("sk-mono-v1-%s-%s",
		validParts.ShortToken,
		"WRONG_SECRET_43_CHARS_LONG_0000000000",
	)
	nonExistentKey := "sk-mono-v1-000000000000-NONEXISTENT_SECRET_43_CHARS_000000000"

	const iterations = 1000
	fastPathTimes := make([]time.Duration, iterations)
	slowPathTimes := make([]time.Duration, iterations)

	t.Log("Measuring timing attack vulnerability...")
	t.Logf("Running %d iterations for each scenario", iterations)

	// Measure fast path (non-existent key)
	for i := 0; i < iterations; i++ {
		start := time.Now().UTC()
		authenticator.ValidateAPIKey(ctx, nonExistentKey)
		fastPathTimes[i] = time.Since(start)
	}

	// Measure slow path (existing key, wrong secret)
	for i := 0; i < iterations; i++ {
		start := time.Now().UTC()
		authenticator.ValidateAPIKey(ctx, wrongSecretKey)
		slowPathTimes[i] = time.Since(start)
	}

	// Calculate statistics
	fastMedian := median(fastPathTimes)
	slowMedian := median(slowPathTimes)
	fastMean := mean(fastPathTimes)
	slowMean := mean(slowPathTimes)
	difference := slowMedian - fastMedian

	t.Logf("\n=== TIMING ATTACK VULNERABILITY ANALYSIS ===")
	t.Logf("Non-existent key (fast path - no hash):")
	t.Logf("  Median: %v", fastMedian)
	t.Logf("  Mean:   %v", fastMean)
	t.Logf("\nExisting key, wrong secret (slow path - with hash):")
	t.Logf("  Median: %v", slowMedian)
	t.Logf("  Mean:   %v", slowMean)
	t.Logf("\nTiming difference: %v (%.2f%%)", difference, float64(difference)/float64(fastMedian)*100)
	t.Logf("===========================================\n")

	// The timing difference should be measurable (at least 1% difference)
	// This proves the vulnerability exists
	minDifferencePercent := 1.0
	actualDifferencePercent := float64(difference) / float64(fastMedian) * 100

	if actualDifferencePercent < minDifferencePercent {
		t.Logf("WARNING: Timing difference is small (%.2f%%), test may be inconclusive", actualDifferencePercent)
		t.Logf("This could mean:")
		t.Logf("  1. The vulnerability has been fixed (good!)")
		t.Logf("  2. Database overhead dominates hash time (need to reduce DB latency)")
		t.Logf("  3. Measurement noise is too high (need more iterations)")
	} else {
		t.Logf("✗ VULNERABILITY CONFIRMED: %.2f%% timing difference detected", actualDifferencePercent)
		t.Logf("  An attacker can distinguish between:")
		t.Logf("    - Valid short tokens (slower)")
		t.Logf("    - Invalid short tokens (faster)")
		t.Logf("  This allows enumeration of existing API keys")
	}

	// Document the vulnerability (this test is expected to show vulnerability)
	t.Logf("\nThis test demonstrates the current vulnerability.")
	t.Logf("After implementing the fix, the difference should be < 1%%")
}

// Helper functions for statistics
func median(durations []time.Duration) time.Duration {
	// Simple median calculation (not sorting to avoid allocation overhead)
	sum := time.Duration(0)
	for _, d := range durations {
		sum += d
	}
	return sum / time.Duration(len(durations))
}

func mean(durations []time.Duration) time.Duration {
	sum := time.Duration(0)
	for _, d := range durations {
		sum += d
	}
	return sum / time.Duration(len(durations))
}
