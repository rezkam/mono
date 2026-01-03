package keygen_test

import (
	"testing"

	"github.com/rezkam/mono/internal/infrastructure/keygen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	for i := range numKeys {
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

	for range numKeys {
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
			require.NoError(t, err)
			assert.Equal(t, tt.want, *got, "parsed API key should match expected values")
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
		{"wrong separator", "sk_mono_v1_token_secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := keygen.ParseAPIKey(tt.apiKey)
			assert.Error(t, err, "should return error for invalid API key format")
		})
	}
}

// TestParseAPIKey_SpecialCharactersInLongSecret tests that ParseAPIKey correctly handles
// hyphens and underscores in the long secret (base64 URL encoding).
func TestParseAPIKey_SpecialCharactersInLongSecret(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
		want   keygen.APIKeyParts
	}{
		{
			name:   "long secret with hyphens",
			apiKey: "sk-mono-v1-a3f5d8c2b4e6-abc-def-ghi-jkl-mno-pqr-stu-vwx-yz",
			want: keygen.APIKeyParts{
				KeyType:    "sk",
				Service:    "mono",
				Version:    "v1",
				ShortToken: "a3f5d8c2b4e6",
				LongSecret: "abc-def-ghi-jkl-mno-pqr-stu-vwx-yz",
				FullKey:    "sk-mono-v1-a3f5d8c2b4e6-abc-def-ghi-jkl-mno-pqr-stu-vwx-yz",
			},
		},
		{
			name:   "long secret with underscores",
			apiKey: "sk-mono-v1-a3f5d8c2b4e6-abc_def_ghi_jkl_mno_pqr_stu_vwx_yz",
			want: keygen.APIKeyParts{
				KeyType:    "sk",
				Service:    "mono",
				Version:    "v1",
				ShortToken: "a3f5d8c2b4e6",
				LongSecret: "abc_def_ghi_jkl_mno_pqr_stu_vwx_yz",
				FullKey:    "sk-mono-v1-a3f5d8c2b4e6-abc_def_ghi_jkl_mno_pqr_stu_vwx_yz",
			},
		},
		{
			name:   "long secret with mixed hyphens and underscores",
			apiKey: "sk-mono-v1-a3f5d8c2b4e6-abc-def_ghi-jkl_mno-pqr_stu",
			want: keygen.APIKeyParts{
				KeyType:    "sk",
				Service:    "mono",
				Version:    "v1",
				ShortToken: "a3f5d8c2b4e6",
				LongSecret: "abc-def_ghi-jkl_mno-pqr_stu",
				FullKey:    "sk-mono-v1-a3f5d8c2b4e6-abc-def_ghi-jkl_mno-pqr_stu",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := keygen.ParseAPIKey(tt.apiKey)
			require.NoError(t, err)
			assert.Equal(t, tt.want, *got, "parsed API key should handle special characters correctly")
		})
	}
}

// TestGenerateAndParse_RoundTrip tests that generated keys can be parsed correctly,
// including keys whose long secrets happen to contain hyphens or underscores.
func TestGenerateAndParse_RoundTrip(t *testing.T) {
	for range 100 {
		// Generate key
		generated, err := keygen.GenerateAPIKey("sk", "mono", "v1")
		require.NoError(t, err)

		// Parse it back
		parsed, err := keygen.ParseAPIKey(generated.FullKey)
		require.NoError(t, err, "should parse generated key: %s", generated.FullKey)

		// Verify all fields match using general struct comparison
		assert.Equal(t, *generated, *parsed, "round-trip should preserve all fields")
	}
}
