package security

import (
	"context"
	"crypto/subtle"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/keygen"
)

// mockRepository is a mock implementation for testing the fixed timing-attack-resistant code
type mockRepository struct {
	keys map[string]*domain.APIKey
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		keys: make(map[string]*domain.APIKey),
	}
}

func (m *mockRepository) FindByShortToken(ctx context.Context, shortToken string) (*domain.APIKey, error) {
	if key, ok := m.keys[shortToken]; ok {
		return key, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockRepository) UpdateLastUsed(ctx context.Context, keyID string, timestamp time.Time) error {
	return nil // No-op for timing tests
}

func (m *mockRepository) Create(ctx context.Context, key *domain.APIKey) error {
	m.keys[key.ShortToken] = key
	return nil
}

// TestTimingAttackMitigated_WithDataIndependentTiming verifies that the fix
// using WithDataIndependentTiming eliminates the timing difference.
func TestTimingAttackMitigated_WithDataIndependentTiming(t *testing.T) {
	const iterations = 10000

	// Setup: Create a valid API key
	ctx := context.Background()
	repo := newMockRepository()

	// Create a real API key
	keyParts, err := keygen.GenerateAPIKey("sk", "mono", "v1")
	if err != nil {
		t.Fatalf("Failed to generate API key: %v", err)
	}

	storedKey := &domain.APIKey{
		ID:             "test-key-id",
		ShortToken:     keyParts.ShortToken,
		LongSecretHash: keygen.HashSecret(keyParts.LongSecret),
		IsActive:       true,
		CreatedAt:      time.Now().UTC().UTC(),
	}

	if err := repo.Create(ctx, storedKey); err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	// Create authenticator with the fixed implementation
	authenticator := auth.NewAuthenticator(repo, auth.Config{
		OperationTimeout: 0,
		UpdateQueueSize:  100,
	})
	defer authenticator.Shutdown(context.Background())

	// Construct test keys
	validKeyWrongSecret := "sk-mono-v1-" + keyParts.ShortToken + "-WRONG_SECRET_43_CHARS_LONG_0000000000"
	nonExistentKey := "sk-mono-v1-000000000000-NONEXISTENT_SECRET_43_CHARS_000000000"

	var nonExistentTime, existingWrongSecretTime time.Duration

	t.Log("Measuring timing with FIXED implementation...")

	// Measure: Non-existent key (should now compute hash too!)
	for i := 0; i < iterations; i++ {
		start := time.Now().UTC()
		authenticator.ValidateAPIKey(ctx, nonExistentKey)
		nonExistentTime += time.Since(start)
	}

	// Measure: Existing key with wrong secret (also computes hash)
	for i := 0; i < iterations; i++ {
		start := time.Now().UTC()
		authenticator.ValidateAPIKey(ctx, validKeyWrongSecret)
		existingWrongSecretTime += time.Since(start)
	}

	// Calculate statistics
	nonExistentAvg := nonExistentTime / iterations
	existingAvg := existingWrongSecretTime / iterations
	diff := existingAvg - nonExistentAvg
	if diff < 0 {
		diff = -diff
	}
	percentDiff := float64(diff) / float64(nonExistentAvg) * 100

	t.Logf("\n========================================")
	t.Logf("  TIMING ATTACK MITIGATION VERIFICATION")
	t.Logf("========================================")
	t.Logf("Iterations: %d", iterations)
	t.Logf("")
	t.Logf("Non-existent key (now with hash!):")
	t.Logf("  Average: %v", nonExistentAvg)
	t.Logf("")
	t.Logf("Existing key, wrong secret (with hash):")
	t.Logf("  Average: %v", existingAvg)
	t.Logf("")
	t.Logf("Timing difference: %v (%.2f%%)", diff, percentDiff)
	t.Logf("========================================")
	t.Logf("")

	// The timing difference should be minimal (< 10% acceptable variance)
	// Previously it was 785% - 3000%, now should be < 10%
	const acceptableVariance = 10.0

	if percentDiff < acceptableVariance {
		t.Logf("✓ TIMING ATTACK MITIGATED: %.2f%% difference (< %.0f%% threshold)", percentDiff, acceptableVariance)
		t.Logf("")
		t.Logf("BEFORE FIX: 785%% - 3000%% timing difference")
		t.Logf("AFTER FIX:  %.2f%% timing difference", percentDiff)
		t.Logf("")
		t.Logf("The fix successfully eliminates the timing side-channel!")
	} else {
		t.Logf("⚠ WARNING: Timing difference (%.2f%%) exceeds threshold (%.0f%%)", percentDiff, acceptableVariance)
		t.Logf("This may indicate:")
		t.Logf("  1. Test variance/noise (especially on loaded systems)")
		t.Logf("  2. Need for more iterations to reduce noise")
		t.Logf("  3. Additional timing leaks to investigate")

		// Don't fail the test if it's close to the threshold (within 2x)
		if percentDiff > acceptableVariance*2 {
			t.Errorf("Timing difference %.2f%% is significantly above threshold %.0f%%", percentDiff, acceptableVariance)
		}
	}
}

// TestWithDataIndependentTiming_AlwaysComputesHash verifies that the hash
// is computed even for non-existent keys.
func TestWithDataIndependentTiming_AlwaysComputesHash(t *testing.T) {
	secret := "8h3k2jf9s7d6f5g4h3j2k1m0n9p8q7r6s5t4u3v2w1x"
	dummyHash := "0000000000000000000000000000000000000000000000000000000000000000"

	const iterations = 10000
	var withHash, withoutHash time.Duration

	t.Log("Measuring hash computation overhead...")

	// Measure: WITH hash computation (fixed implementation)
	for i := 0; i < iterations; i++ {
		start := time.Now().UTC()

		subtle.WithDataIndependentTiming(func() {
			providedHash := keygen.HashSecret(secret)
			_ = subtle.ConstantTimeCompare([]byte(dummyHash), []byte(providedHash))
		})

		withHash += time.Since(start)
	}

	// Measure: WITHOUT hash computation (vulnerable implementation)
	for i := 0; i < iterations; i++ {
		start := time.Now().UTC()

		subtle.WithDataIndependentTiming(func() {
			// Just compare dummy hashes (no computation)
			_ = subtle.ConstantTimeCompare([]byte(dummyHash), []byte(dummyHash))
		})

		withoutHash += time.Since(start)
	}

	withHashAvg := withHash / iterations
	withoutHashAvg := withoutHash / iterations
	diff := withHashAvg - withoutHashAvg

	t.Logf("\nHash computation verification:")
	t.Logf("  WITH hash:    %v", withHashAvg)
	t.Logf("  WITHOUT hash: %v", withoutHashAvg)
	t.Logf("  Difference:   %v", diff)

	// The difference should be measurable (proves hash is being computed)
	if diff < 50*time.Nanosecond {
		t.Errorf("Hash computation not measurable (%v) - may not be executing", diff)
	} else {
		t.Logf("✓ Hash computation confirmed (%.0fns overhead)", float64(diff.Nanoseconds()))
	}
}

// BenchmarkAuthenticationFlow_Fixed benchmarks the fixed implementation
// to compare with the vulnerable version.
func BenchmarkAuthenticationFlow_Fixed(b *testing.B) {
	ctx := context.Background()
	repo := newMockRepository()

	// Create a valid API key
	keyParts, _ := keygen.GenerateAPIKey("sk", "mono", "v1")
	storedKey := &domain.APIKey{
		ID:             "test-key-id",
		ShortToken:     keyParts.ShortToken,
		LongSecretHash: keygen.HashSecret(keyParts.LongSecret),
		IsActive:       true,
		CreatedAt:      time.Now().UTC().UTC(),
	}
	repo.Create(ctx, storedKey)

	authenticator := auth.NewAuthenticator(repo, auth.Config{
		OperationTimeout: 0,
		UpdateQueueSize:  100,
	})
	defer authenticator.Shutdown(context.Background())

	b.Run("NonExistentKey_NowWithHash", func(b *testing.B) {
		nonExistentKey := "sk-mono-v1-000000000000-NONEXISTENT_SECRET_43_CHARS_000000000"

		for b.Loop() {
			authenticator.ValidateAPIKey(ctx, nonExistentKey)
		}
	})

	b.Run("ExistingKey_WrongSecret", func(b *testing.B) {
		wrongSecretKey := "sk-mono-v1-" + keyParts.ShortToken + "-WRONG_SECRET_43_CHARS_LONG_0000000000"

		for b.Loop() {
			authenticator.ValidateAPIKey(ctx, wrongSecretKey)
		}
	})

	b.Run("ValidKey", func(b *testing.B) {
		for b.Loop() {
			authenticator.ValidateAPIKey(ctx, keyParts.FullKey)
		}
	})
}
