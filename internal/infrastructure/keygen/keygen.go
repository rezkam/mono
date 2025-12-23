package keygen

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/rezkam/mono/internal/domain"
	"golang.org/x/crypto/blake2b"
)

// APIKeyParts represents the components of an API key.
type APIKeyParts struct {
	KeyType    string // e.g., "sk" (secret key) or "pk" (public key)
	Service    string // e.g., "mono"
	Version    string // e.g., "v1"
	ShortToken string // Short token for lookup (12 hex chars from BLAKE2b hash prefix)
	LongSecret string // Long secret for authentication (43 chars base64)
	FullKey    string // Complete assembled key
}

// GenerateAPIKey creates a new API key following the pattern:
// {key_type}-{service}-{version}-{short_token}-{long_secret}
// Example: sk-mono-v1-a3f5d8c2b4e6-8h3k2jf9s7d6f5g4h3j2k1m0n9p8q7r6s5t4u3v2w1x
func GenerateAPIKey(keyType, service, version string) (*APIKeyParts, error) {
	// Generate long secret (32 random bytes = 43 chars in base64)
	longBytes := make([]byte, 32)
	if _, err := rand.Read(longBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	longSecret := base64.RawURLEncoding.EncodeToString(longBytes)

	// Derive short token from BLAKE2b hash of the long secret
	// Using first 12 hex chars (48 bits) from 256-bit hash output
	// This is secure because:
	// - Backed by 256 bits of crypto/rand entropy
	// - BLAKE2b output is uniformly distributed
	// - 48 bits = ~281 trillion combinations (collision-resistant)
	hash := blake2b.Sum256([]byte(longSecret))
	shortToken := hex.EncodeToString(hash[:6]) // 6 bytes = 12 hex chars

	// Assemble full key
	fullKey := fmt.Sprintf("%s-%s-%s-%s-%s", keyType, service, version, shortToken, longSecret)

	return &APIKeyParts{
		KeyType:    keyType,
		Service:    service,
		Version:    version,
		ShortToken: shortToken,
		LongSecret: longSecret,
		FullKey:    fullKey,
	}, nil
}

// ParseAPIKey parses an API key string into its components.
// Expected format: {key_type}-{service}-{version}-{short_token}-{long_secret}
// The long_secret part uses base64 URL encoding and may contain hyphens (-) and underscores (_).
func ParseAPIKey(apiKey string) (*APIKeyParts, error) {
	// Use SplitN to split into at most 5 parts
	// This allows the long_secret (last part) to contain hyphens
	parts := strings.SplitN(apiKey, "-", 5)
	if len(parts) != 5 {
		return nil, fmt.Errorf("%w: expected 5 parts, got %d", domain.ErrInvalidAPIKeyFormat, len(parts))
	}

	return &APIKeyParts{
		KeyType:    parts[0],
		Service:    parts[1],
		Version:    parts[2],
		ShortToken: parts[3],
		LongSecret: parts[4],
		FullKey:    apiKey,
	}, nil
}

// GetDisplayKey returns a safe-to-display version of the key showing only prefix and short token.
// Example: "sk-mono-v1-a3f5d8c2b4e6-****"
func (k *APIKeyParts) GetDisplayKey() string {
	return fmt.Sprintf("%s-%s-%s-%s-****", k.KeyType, k.Service, k.Version, k.ShortToken)
}

// HashSecret computes BLAKE2b-256 hash of the secret and returns hex-encoded string.
// BLAKE2b is faster than SHA-256 while maintaining security for high-entropy API keys.
func HashSecret(secret string) string {
	hash := blake2b.Sum256([]byte(secret))
	return hex.EncodeToString(hash[:])
}

// MaskAPIKey returns a safe-to-log version of an API key showing only the prefix.
// Example: "sk-mono-v1-a3f5d8c2b4e6-****" → "sk-***"
func MaskAPIKey(apiKey string) string {
	parts, err := ParseAPIKey(apiKey)
	if err != nil {
		return "***"
	}
	return parts.KeyType + "-***"
}
