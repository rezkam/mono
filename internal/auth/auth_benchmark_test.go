package auth_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/auth"
	sqlstorage "github.com/rezkam/mono/internal/storage/sql"
	"golang.org/x/crypto/bcrypt"
)

// BenchmarkAuthO1Lookup demonstrates O(1) authentication performance.
// This benchmark proves that authentication time is CONSTANT regardless of total API keys.
//
// Run: BENCHMARK_POSTGRES_URL="postgres://..." go test -bench=BenchmarkAuthO1Lookup -benchmem ./internal/auth/
//
// Expected: ~50-100ms per auth (bcrypt cost), consistent across 10/100/1000 keys
func BenchmarkAuthO1Lookup(b *testing.B) {
	pgURL := os.Getenv("BENCHMARK_POSTGRES_URL")
	if pgURL == "" {
		b.Skip("BENCHMARK_POSTGRES_URL not set")
	}

	ctx := context.Background()
	store, err := sqlstorage.NewPostgresStore(ctx, pgURL)
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
				key, err := auth.CreateAPIKey(ctx, store.Queries(), "sk", "mono", "v1",
					fmt.Sprintf("Bench Key %d", i), nil)
				if err != nil {
					b.Fatalf("Failed to create key: %v", err)
				}
				if i == targetPosition {
					targetKey = key
				}
			}

			// Parse key once (not part of measurement)
			keyParts, err := auth.ParseAPIKey(targetKey)
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

				// Verify with bcrypt
				if err := bcrypt.CompareHashAndPassword([]byte(apiKey.LongSecretHash), []byte(keyParts.LongSecret)); err != nil {
					b.Fatalf("Verification failed: %v", err)
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
	store, err := sqlstorage.NewPostgresStore(ctx, pgURL)
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
			key, err := auth.CreateAPIKey(ctx, store.Queries(), "sk", "mono", "v1",
				fmt.Sprintf("Key %d", i), nil)
			if err != nil {
				b.Fatalf("Failed to create key: %v", err)
			}
			if i == numKeys-1 {
				targetKey = key // Use last key (worst case for O(n))
			}
		}

		keyParts, _ := auth.ParseAPIKey(targetKey)

		// Benchmark using Go 1.24 B.Loop()
		for b.Loop() {
			apiKey, err := store.Queries().GetAPIKeyByShortToken(ctx, keyParts.ShortToken)
			if err != nil {
				b.Fatalf("Lookup failed: %v", err)
			}

			if err := bcrypt.CompareHashAndPassword([]byte(apiKey.LongSecretHash), []byte(keyParts.LongSecret)); err != nil {
				b.Fatalf("Verification failed: %v", err)
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
			key, err := auth.CreateAPIKey(ctx, store.Queries(), "sk", "mono", "v1",
				fmt.Sprintf("Key %d", i), nil)
			if err != nil {
				b.Fatalf("Failed to create key: %v", err)
			}
			if i == numKeys-1 {
				targetKey = key
			}
		}

		keyParts, _ := auth.ParseAPIKey(targetKey)

		// Load all keys (simulating old O(n) approach)
		allKeys, err := store.Queries().ListActiveAPIKeys(ctx)
		if err != nil {
			b.Fatalf("Failed to list keys: %v", err)
		}

		// Benchmark: O(n) linear scan through all keys
		for b.Loop() {
			found := false
			// Iterate through ALL keys checking each hash
			for _, key := range allKeys {
				if err := bcrypt.CompareHashAndPassword([]byte(key.LongSecretHash), []byte(keyParts.LongSecret)); err == nil {
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
		_, err := auth.GenerateAPIKey("sk", "mono", "v1")
		if err != nil {
			b.Fatalf("Failed to generate key: %v", err)
		}
	}
}

// BenchmarkKeyParsing benchmarks API key parsing performance.
func BenchmarkKeyParsing(b *testing.B) {
	key := "sk-mono-v1-a7f3d8e2-8h3k2jf9s7d6f5g4h3j2k1m0n9p8q7r6s5t4u3v2w1x"

	for b.Loop() {
		_, err := auth.ParseAPIKey(key)
		if err != nil {
			b.Fatalf("Failed to parse key: %v", err)
		}
	}
}

// BenchmarkBcryptCost shows the security/performance tradeoff of bcrypt.
// This explains why authentication takes ~50-100ms regardless of implementation.
func BenchmarkBcryptCost(b *testing.B) {
	secret := "8h3k2jf9s7d6f5g4h3j2k1m0n9p8q7r6s5t4u3v2w1x"

	b.Run("Hash_Cost14", func(b *testing.B) {
		for b.Loop() {
			_, err := bcrypt.GenerateFromPassword([]byte(secret), 14)
			if err != nil {
				b.Fatalf("Failed to hash: %v", err)
			}
		}
	})

	b.Run("Compare_Cost14", func(b *testing.B) {
		// Setup: Generate hash once
		hash, _ := bcrypt.GenerateFromPassword([]byte(secret), 14)

		for b.Loop() {
			err := bcrypt.CompareHashAndPassword(hash, []byte(secret))
			if err != nil {
				b.Fatalf("Failed to compare: %v", err)
			}
		}
	})
}
