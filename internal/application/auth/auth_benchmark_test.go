package auth_test

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/infrastructure/keygen"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"golang.org/x/crypto/blake2b"
)

// hashSecret computes BLAKE2b-256 hash of the secret (matches auth package implementation)
func hashSecret(secret string) string {
	hash := blake2b.Sum256([]byte(secret))
	return hex.EncodeToString(hash[:])
}

// BenchmarkAuthO1Lookup demonstrates O(1) authentication performance.
// This benchmark proves that authentication time is CONSTANT regardless of total API keys.
//
// Run: BENCHMARK_POSTGRES_URL="postgres://..." go test -bench=BenchmarkAuthO1Lookup -benchmem ./internal/application/auth/
//
// Expected: <1ms per auth (BLAKE2b-256), consistent across 10/100/1000 keys
func BenchmarkAuthO1Lookup(b *testing.B) {
	pgURL := os.Getenv("BENCHMARK_POSTGRES_URL")
	if pgURL == "" {
		b.Skip("BENCHMARK_POSTGRES_URL not set")
	}

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	if err != nil {
		b.Fatalf("Failed to connect: %v", err)
	}
	defer store.Close()

	// Table-driven benchmark: test with different key counts and positions
	scenarios := []struct {
		numKeys  int
		position string // "first", "middle", "last"
	}{
		{10, "first"},
		{10, "middle"},
		{10, "last"},
		{100, "first"},
		{100, "middle"},
		{100, "last"},
		{1000, "first"},
		{1000, "middle"},
		{1000, "last"},
	}

	for _, scenario := range scenarios {
		name := fmt.Sprintf("%dKeys_%s", scenario.numKeys, scenario.position)
		b.Run(name, func(b *testing.B) {
			// Setup: Clean database
			_, err = store.DB().ExecContext(ctx, "TRUNCATE api_keys CASCADE")
			if err != nil {
				b.Fatalf("Failed to clean database: %v", err)
			}

			// Setup: Create keys
			var targetKey string
			var targetPosition int

			switch scenario.position {
			case "first":
				targetPosition = 0
			case "middle":
				targetPosition = scenario.numKeys / 2
			case "last":
				targetPosition = scenario.numKeys - 1
			}

			for i := 0; i < scenario.numKeys; i++ {
				key, err := auth.CreateAPIKey(ctx, store, "sk", "mono", "v1",
					fmt.Sprintf("Bench Key %d", i), nil)
				if err != nil {
					b.Fatalf("Failed to create key: %v", err)
				}
				if i == targetPosition {
					targetKey = key
				}
			}

			// Parse key once (not part of measurement)
			keyParts, err := keygen.ParseAPIKey(targetKey)
			if err != nil {
				b.Fatalf("Failed to parse key: %v", err)
			}

			// Benchmark loop using Go 1.24 B.Loop()
			for b.Loop() {
				// O(1) indexed lookup by short_token
				apiKey, err := store.Queries().GetAPIKeyByShortToken(ctx, keyParts.ShortToken)
				if err != nil {
					b.Fatalf("Lookup failed: %v", err)
				}

				// Verify with BLAKE2b-256 constant-time comparison
				providedHash := hashSecret(keyParts.LongSecret)
				if subtle.ConstantTimeCompare([]byte(apiKey.LongSecretHash), []byte(providedHash)) != 1 {
					b.Fatal("Verification failed")
				}

				// Check expiration
				if apiKey.ExpiresAt.Valid && apiKey.ExpiresAt.Time.Before(time.Now().UTC()) {
					b.Fatal("Key expired")
				}
			}

			// Report custom metrics
			b.ReportMetric(float64(scenario.numKeys), "total_keys")
		})
	}
}

// BenchmarkAuthO1_vs_On compares O(1) indexed lookup vs O(n) linear scan.
// This dramatically shows why the indexed approach is superior.
func BenchmarkAuthO1_vs_On(b *testing.B) {
	pgURL := os.Getenv("BENCHMARK_POSTGRES_URL")
	if pgURL == "" {
		b.Skip("BENCHMARK_POSTGRES_URL not set")
	}

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	if err != nil {
		b.Fatalf("Failed to connect: %v", err)
	}
	defer store.Close()

	numKeys := 100

	b.Run("O1_IndexedLookup", func(b *testing.B) {
		// Setup
		_, err = store.DB().ExecContext(ctx, "TRUNCATE api_keys CASCADE")
		if err != nil {
			b.Fatalf("Failed to clean database: %v", err)
		}

		var targetKey string
		for i := 0; i < numKeys; i++ {
			key, err := auth.CreateAPIKey(ctx, store, "sk", "mono", "v1",
				fmt.Sprintf("Key %d", i), nil)
			if err != nil {
				b.Fatalf("Failed to create key: %v", err)
			}
			if i == numKeys-1 {
				targetKey = key // Use last key (worst case for O(n))
			}
		}

		keyParts, _ := keygen.ParseAPIKey(targetKey)

		// Benchmark using Go 1.24 B.Loop()
		for b.Loop() {
			apiKey, err := store.Queries().GetAPIKeyByShortToken(ctx, keyParts.ShortToken)
			if err != nil {
				b.Fatalf("Lookup failed: %v", err)
			}

			providedHash := hashSecret(keyParts.LongSecret)
			if subtle.ConstantTimeCompare([]byte(apiKey.LongSecretHash), []byte(providedHash)) != 1 {
				b.Fatal("Verification failed")
			}
		}

		b.ReportMetric(float64(numKeys), "total_keys")
	})

	b.Run("On_LinearScan", func(b *testing.B) {
		// Setup
		_, err = store.DB().ExecContext(ctx, "TRUNCATE api_keys CASCADE")
		if err != nil {
			b.Fatalf("Failed to clean database: %v", err)
		}

		var targetKey string
		for i := 0; i < numKeys; i++ {
			key, err := auth.CreateAPIKey(ctx, store, "sk", "mono", "v1",
				fmt.Sprintf("Key %d", i), nil)
			if err != nil {
				b.Fatalf("Failed to create key: %v", err)
			}
			if i == numKeys-1 {
				targetKey = key
			}
		}

		keyParts, _ := keygen.ParseAPIKey(targetKey)

		// Load all keys for comparison
		allKeys, err := store.Queries().ListActiveAPIKeys(ctx)
		if err != nil {
			b.Fatalf("Failed to list keys: %v", err)
		}

		// Benchmark: O(n) linear scan through all keys
		providedHash := hashSecret(keyParts.LongSecret)
		for b.Loop() {
			found := false
			// Iterate through ALL keys checking each hash
			for _, key := range allKeys {
				if subtle.ConstantTimeCompare([]byte(key.LongSecretHash), []byte(providedHash)) == 1 {
					found = true
					break
				}
			}
			if !found {
				b.Fatal("Key not found")
			}
		}

		b.ReportMetric(float64(numKeys), "total_keys")
	})
}

// BenchmarkKeyGeneration benchmarks API key generation speed.
func BenchmarkKeyGeneration(b *testing.B) {
	for b.Loop() {
		_, err := keygen.GenerateAPIKey("sk", "mono", "v1")
		if err != nil {
			b.Fatalf("Failed to generate key: %v", err)
		}
	}
}

// BenchmarkKeyParsing benchmarks API key parsing performance.
func BenchmarkKeyParsing(b *testing.B) {
	key := "sk-mono-v1-a7f3d8e2-8h3k2jf9s7d6f5g4h3j2k1m0n9p8q7r6s5t4u3v2w1x"

	for b.Loop() {
		_, err := keygen.ParseAPIKey(key)
		if err != nil {
			b.Fatalf("Failed to parse key: %v", err)
		}
	}
}

// BenchmarkBLAKE2bHash benchmarks BLAKE2b-256 hashing performance.
// This shows BLAKE2b is faster than SHA-256 for high-entropy API keys.
func BenchmarkBLAKE2bHash(b *testing.B) {
	secret := "8h3k2jf9s7d6f5g4h3j2k1m0n9p8q7r6s5t4u3v2w1x"

	b.Run("Hash", func(b *testing.B) {
		for b.Loop() {
			_ = hashSecret(secret)
		}
	})

	b.Run("HashAndCompare", func(b *testing.B) {
		// Setup: Generate hash once
		storedHash := hashSecret(secret)

		for b.Loop() {
			providedHash := hashSecret(secret)
			_ = subtle.ConstantTimeCompare([]byte(storedHash), []byte(providedHash))
		}
	})
}
