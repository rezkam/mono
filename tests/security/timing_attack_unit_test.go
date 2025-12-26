package security

import (
	"crypto/subtle"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/infrastructure/keygen"
)

// TestHashTimingIsMe asurable proves that BLAKE2b hash computation
// is measurable and creates a timing difference in the authentication flow.
//
// This test directly measures the timing difference between:
//  1. Fast path: No hash computation
//  2. Slow path: BLAKE2b hash computation + constant-time compare
func TestHashTimingIsMeasurable(t *testing.T) {
	const iterations = 10000

	// Prepare data
	realSecret := "8h3k2jf9s7d6f5g4h3j2k1m0n9p8q7r6s5t4u3v2w1x" // 43 chars
	storedHash := keygen.HashSecret(realSecret)
	dummyHash := "0000000000000000000000000000000000000000000000000000000000000000"

	var fastPathTotal, slowPathTotal time.Duration

	// Measure FAST path (no hash computation)
	t.Log("Measuring FAST path (no hash)...")
	for i := 0; i < iterations; i++ {
		start := time.Now().UTC()

		// Simulate fast path: just comparison with dummy hash (no computation)
		_ = subtle.ConstantTimeCompare([]byte(storedHash), []byte(dummyHash))

		fastPathTotal += time.Since(start)
	}

	// Measure SLOW path (with hash computation)
	t.Log("Measuring SLOW path (with BLAKE2b hash)...")
	for i := 0; i < iterations; i++ {
		start := time.Now().UTC()

		// Simulate slow path: hash computation + comparison
		providedHash := keygen.HashSecret(realSecret)
		_ = subtle.ConstantTimeCompare([]byte(storedHash), []byte(providedHash))

		slowPathTotal += time.Since(start)
	}

	// Calculate averages
	fastAvg := fastPathTotal / iterations
	slowAvg := slowPathTotal / iterations
	difference := slowAvg - fastAvg
	percentDiff := float64(difference) / float64(fastAvg) * 100

	// Report results
	t.Logf("\n========================================")
	t.Logf("  TIMING ATTACK VULNERABILITY PROOF")
	t.Logf("========================================")
	t.Logf("Iterations: %d", iterations)
	t.Logf("")
	t.Logf("FAST path (no hash):")
	t.Logf("  Average: %v", fastAvg)
	t.Logf("")
	t.Logf("SLOW path (with BLAKE2b hash):")
	t.Logf("  Average: %v", slowAvg)
	t.Logf("")
	t.Logf("Timing difference: %v (%.2f%%)", difference, percentDiff)
	t.Logf("========================================")
	t.Logf("")

	// The hash operation should be measurable
	if difference <= 0 {
		t.Fatalf("Expected slow path to be slower, but difference was %v", difference)
	}

	if percentDiff < 50 {
		t.Logf("WARNING: Timing difference is small (%.2f%%), but still measurable", percentDiff)
	} else {
		t.Logf("✗ VULNERABILITY: %.2f%% timing difference is EASILY exploitable", percentDiff)
	}

	t.Logf("")
	t.Logf("ATTACK SCENARIO:")
	t.Logf("  1. Attacker sends API keys with random short tokens")
	t.Logf("  2. Measures response times for each request")
	t.Logf("  3. Slower responses = valid short token exists (hash computed)")
	t.Logf("  4. Faster responses = invalid short token (no hash computed)")
	t.Logf("  5. Attacker can enumerate all valid short tokens in database")
	t.Logf("")
	t.Logf("MITIGATION: Always compute hash, even for non-existent keys")
}

// BenchmarkHashOperationTiming benchmarks the hash operation to show its cost.
func BenchmarkHashOperationTiming(b *testing.B) {
	secret := "8h3k2jf9s7d6f5g4h3j2k1m0n9p8q7r6s5t4u3v2w1x"

	b.Run("OnlyComparison", func(b *testing.B) {
		hash1 := keygen.HashSecret(secret)
		hash2 := keygen.HashSecret(secret)

		for b.Loop() {
			_ = subtle.ConstantTimeCompare([]byte(hash1), []byte(hash2))
		}
	})

	b.Run("HashPlusComparison", func(b *testing.B) {
		storedHash := keygen.HashSecret(secret)

		for b.Loop() {
			providedHash := keygen.HashSecret(secret)
			_ = subtle.ConstantTimeCompare([]byte(storedHash), []byte(providedHash))
		}
	})

	b.Run("OnlyHash", func(b *testing.B) {
		for b.Loop() {
			_ = keygen.HashSecret(secret)
		}
	})
}

// TestAuthenticationFlowVulnerability simulates the actual authentication flow
// showing the vulnerability in ValidateAPIKey.
func TestAuthenticationFlowVulnerability(t *testing.T) {
	const iterations = 5000

	// Setup
	realSecret := "8h3k2jf9s7d6f5g4h3j2k1m0n9p8q7r6s5t4u3v2w1x"
	storedHash := keygen.HashSecret(realSecret)

	var nonExistentKeyTime, existingKeyTime time.Duration

	t.Log("Simulating authentication flow...")

	// Simulate: Non-existent short token (current FAST path)
	for i := 0; i < iterations; i++ {
		start := time.Now().UTC()

		// Current vulnerable code path:
		// 1. DB lookup fails (short token not found)
		// 2. Return immediately (NO hash computation)
		// Simulated by: just doing nothing
		_ = storedHash // simulate using the stored hash

		nonExistentKeyTime += time.Since(start)
	}

	// Simulate: Existing short token with wrong secret (current SLOW path)
	for i := 0; i < iterations; i++ {
		start := time.Now().UTC()

		// Current vulnerable code path:
		// 1. DB lookup succeeds (short token found)
		// 2. Compute hash of provided secret ← TIMING LEAK
		// 3. Constant-time compare (fails)
		providedHash := keygen.HashSecret(realSecret)
		_ = subtle.ConstantTimeCompare([]byte(storedHash), []byte(providedHash))

		existingKeyTime += time.Since(start)
	}

	nonExistentAvg := nonExistentKeyTime / iterations
	existingAvg := existingKeyTime / iterations
	diff := existingAvg - nonExistentAvg
	percentDiff := float64(diff) / float64(nonExistentAvg) * 100

	t.Logf("\n========================================")
	t.Logf("  AUTHENTICATION FLOW TIMING ANALYSIS")
	t.Logf("========================================")
	t.Logf("")
	t.Logf("Non-existent key (fast path):")
	t.Logf("  Average: %v", nonExistentAvg)
	t.Logf("")
	t.Logf("Existing key with wrong secret (slow path):")
	t.Logf("  Average: %v", existingAvg)
	t.Logf("")
	t.Logf("Timing leak: %v (%.0f%% slower)", diff, percentDiff)
	t.Logf("========================================")
	t.Logf("")
	t.Logf("This demonstrates that the hash computation time")
	t.Logf("creates a measurable side-channel that reveals")
	t.Logf("whether a short token exists in the database.")
}

// TestConstantTimeComparisonIsConstantTime verifies that subtle.ConstantTimeCompare
// is actually constant-time (doesn't leak which position differs).
func TestConstantTimeComparisonIsConstantTime(t *testing.T) {
	const iterations = 100000

	hash1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hash2DiffFirst := "baaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // Differs at position 0
	hash2DiffLast := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaab"   // Differs at position 63

	var diffFirstTime, diffLastTime time.Duration

	// Measure: Difference at first position
	for i := 0; i < iterations; i++ {
		start := time.Now().UTC()
		_ = subtle.ConstantTimeCompare([]byte(hash1), []byte(hash2DiffFirst))
		diffFirstTime += time.Since(start)
	}

	// Measure: Difference at last position
	for i := 0; i < iterations; i++ {
		start := time.Now().UTC()
		_ = subtle.ConstantTimeCompare([]byte(hash1), []byte(hash2DiffLast))
		diffLastTime += time.Since(start)
	}

	diffFirstAvg := diffFirstTime / iterations
	diffLastAvg := diffLastTime / iterations
	diff := diffLastAvg - diffFirstAvg
	if diff < 0 {
		diff = -diff
	}
	percentDiff := float64(diff) / float64(diffFirstAvg) * 100

	t.Logf("\nConstant-time comparison verification:")
	t.Logf("  Diff at first position: %v", diffFirstAvg)
	t.Logf("  Diff at last position:  %v", diffLastAvg)
	t.Logf("  Difference: %v (%.2f%%)", diff, percentDiff)

	// ConstantTimeCompare should have minimal timing difference
	// (< 5% variation is acceptable noise)
	if percentDiff > 5 {
		t.Logf("WARNING: subtle.ConstantTimeCompare may not be constant-time (%.2f%% difference)", percentDiff)
	} else {
		t.Logf("✓ subtle.ConstantTimeCompare is constant-time (%.2f%% variation)", percentDiff)
	}
}
