package keygen_test

import (
	"testing"

	"github.com/rezkam/mono/internal/infrastructure/keygen"
)

// TestGenerateAPIKey_UniqueShortTokens tests that short tokens are unique
// even when generating multiple keys rapidly (within same millisecond).
//
// Short tokens are now derived from BLAKE2b hash of the long secret,
// which is backed by 256 bits of crypto/rand entropy, ensuring uniqueness.
func TestGenerateAPIKey_UniqueShortTokens(t *testing.T) {
	const numKeys = 1000
	seen := make(map[string]bool)
	duplicates := []string{}

	// Generate keys as fast as possible (simulates high-throughput scenario)
	for i := 0; i < numKeys; i++ {
		keyParts, err := keygen.GenerateAPIKey("sk", "mono", "v1")
		if err != nil {
			t.Fatalf("Failed to generate key %d: %v", i, err)
		}

		if seen[keyParts.ShortToken] {
			duplicates = append(duplicates, keyParts.ShortToken)
		}
		seen[keyParts.ShortToken] = true
	}

	if len(duplicates) > 0 {
		t.Errorf("Found %d duplicate short tokens out of %d keys", len(duplicates), numKeys)
		t.Errorf("First few duplicates: %v", duplicates[:min(5, len(duplicates))])
		t.Errorf("Unique short tokens: %d", len(seen))
		t.Errorf("\nThis is a CRITICAL bug: database has unique constraint on short_token.")
		t.Errorf("Multiple keys in same millisecond will fail to insert.")
	}
}

// TestGenerateAPIKey_ShortTokenCollisionRate measures collision probability
// for short tokens when generating keys rapidly.
func TestGenerateAPIKey_ShortTokenCollisionRate(t *testing.T) {
	const numKeys = 100
	seen := make(map[string]int)

	for i := 0; i < numKeys; i++ {
		keyParts, err := keygen.GenerateAPIKey("sk", "mono", "v1")
		if err != nil {
			t.Fatalf("Failed to generate key: %v", err)
		}
		seen[keyParts.ShortToken]++
	}

	// Report collision statistics
	maxCollisions := 0
	for token, count := range seen {
		if count > maxCollisions {
			maxCollisions = count
		}
		if count > 1 {
			t.Logf("Short token %s appeared %d times", token, count)
		}
	}

	collisionRate := float64(numKeys-len(seen)) / float64(numKeys) * 100
	t.Logf("Generated %d keys", numKeys)
	t.Logf("Unique short tokens: %d", len(seen))
	t.Logf("Collision rate: %.2f%%", collisionRate)
	t.Logf("Max collisions for single token: %d", maxCollisions)

	// Fail if collision rate is above 1%
	// (For truly unique tokens, collision rate should be ~0%)
	if collisionRate > 1.0 {
		t.Errorf("Collision rate too high: %.2f%% (expected <1%%)", collisionRate)
		t.Errorf("This indicates short tokens are not sufficiently unique for high-throughput scenarios")
	}
}

// TestParseAPIKey_ValidFormat tests parsing of valid API keys
func TestParseAPIKey_ValidFormat(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
		want   keygen.APIKeyParts
	}{
		{
			name:   "valid key",
			apiKey: "sk-mono-v1-a3f5d8c2b4e6-8h3k2jf9s7d6f5g4h3j2k1m0n9p8q7r6s5t4u3v2w1x",
			want: keygen.APIKeyParts{
				KeyType:    "sk",
				Service:    "mono",
				Version:    "v1",
				ShortToken: "a3f5d8c2b4e6",
				LongSecret: "8h3k2jf9s7d6f5g4h3j2k1m0n9p8q7r6s5t4u3v2w1x",
				FullKey:    "sk-mono-v1-a3f5d8c2b4e6-8h3k2jf9s7d6f5g4h3j2k1m0n9p8q7r6s5t4u3v2w1x",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := keygen.ParseAPIKey(tt.apiKey)
			if err != nil {
				t.Fatalf("ParseAPIKey() error = %v", err)
			}
			if got.KeyType != tt.want.KeyType {
				t.Errorf("KeyType = %v, want %v", got.KeyType, tt.want.KeyType)
			}
			if got.Service != tt.want.Service {
				t.Errorf("Service = %v, want %v", got.Service, tt.want.Service)
			}
			if got.Version != tt.want.Version {
				t.Errorf("Version = %v, want %v", got.Version, tt.want.Version)
			}
			if got.ShortToken != tt.want.ShortToken {
				t.Errorf("ShortToken = %v, want %v", got.ShortToken, tt.want.ShortToken)
			}
			if got.LongSecret != tt.want.LongSecret {
				t.Errorf("LongSecret = %v, want %v", got.LongSecret, tt.want.LongSecret)
			}
		})
	}
}

// TestParseAPIKey_InvalidFormat tests parsing of invalid API keys
func TestParseAPIKey_InvalidFormat(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
	}{
		{"empty", ""},
		{"missing parts", "sk-mono-v1"},
		{"too many parts", "sk-mono-v1-token-secret-extra"},
		{"wrong separator", "sk_mono_v1_token_secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := keygen.ParseAPIKey(tt.apiKey)
			if err == nil {
				t.Errorf("ParseAPIKey() expected error for invalid format, got nil")
			}
		})
	}
}
