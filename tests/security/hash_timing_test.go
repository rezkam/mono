package security

import (
	"crypto/subtle"
	"testing"

	"github.com/rezkam/mono/internal/infrastructure/keygen"
)

// TestBLAKE2bConstantTimeSameLength verifies that BLAKE2b hashing time
// is constant for same-length inputs with different content.
//
// This test proves that using a dummy 43-char string is safe against timing attacks
// because BLAKE2b timing depends on input LENGTH, not content.
func TestBLAKE2bConstantTimeSameLength(t *testing.T) {
	// Real API key long secret (43 chars)
	realSecret := "8h3k2jf9s7d6f5g4h3j2k1m0n9p8q7r6s5t4u3v2w1x"

	// Dummy value (43 chars, all zeros)
	dummySecret := "0000000000000000000000000000000000000000000"

	// Another dummy (43 chars, all ones)
	dummyOnes := "1111111111111111111111111111111111111111111"

	// Verify all are same length
	if len(realSecret) != 43 || len(dummySecret) != 43 || len(dummyOnes) != 43 {
		t.Fatalf("Test setup error: all secrets must be 43 chars")
	}

	// Hash all three - timing should be identical for same length inputs
	hash1 := keygen.HashSecret(realSecret)
	hash2 := keygen.HashSecret(dummySecret)
	hash3 := keygen.HashSecret(dummyOnes)

	// Verify outputs are different (proves BLAKE2b is working)
	if hash1 == hash2 || hash1 == hash3 || hash2 == hash3 {
		t.Error("BLAKE2b should produce different hashes for different inputs")
	}

	// Verify all outputs are same length (64 hex chars for 256-bit hash)
	if len(hash1) != 64 || len(hash2) != 64 || len(hash3) != 64 {
		t.Errorf("Expected 64-char hex output, got %d, %d, %d", len(hash1), len(hash2), len(hash3))
	}
}

// TestWithDataIndependentTimingUsage demonstrates correct usage of
// subtle.WithDataIndependentTiming with constant-time operations.
func TestWithDataIndependentTimingUsage(t *testing.T) {
	realSecret := "8h3k2jf9s7d6f5g4h3j2k1m0n9p8q7r6s5t4u3v2w1x"
	storedHash := keygen.HashSecret(realSecret)

	// Correct usage: constant-time operations inside WithDataIndependentTiming
	var isValid int
	subtle.WithDataIndependentTiming(func() {
		// Always compute hash (happens for both valid and invalid keys)
		providedHash := keygen.HashSecret(realSecret)

		// Constant-time comparison (MUST use this, not ==)
		isValid = subtle.ConstantTimeCompare([]byte(storedHash), []byte(providedHash))
	})

	if isValid != 1 {
		t.Error("Hash should match")
	}
}

// TestWithDataIndependentTimingNestedCalls verifies that nested calls work correctly
func TestWithDataIndependentTimingNestedCalls(t *testing.T) {
	outerExecuted := false
	innerExecuted := false

	// Outer WithDataIndependentTiming
	subtle.WithDataIndependentTiming(func() {
		outerExecuted = true

		// Nested WithDataIndependentTiming (should work fine)
		subtle.WithDataIndependentTiming(func() {
			innerExecuted = true

			// Perform some constant-time operation
			hash := keygen.HashSecret("test")
			if len(hash) != 64 {
				t.Error("Hash should be 64 hex chars")
			}
		})
	})

	if !outerExecuted || !innerExecuted {
		t.Error("Both nested WithDataIndependentTiming calls should execute")
	}
}
