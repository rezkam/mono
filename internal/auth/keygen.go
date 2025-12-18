package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// APIKeyParts represents the components of an API key.
type APIKeyParts struct {
	KeyType    string // e.g., "sk" (secret key) or "pk" (public key)
	Service    string // e.g., "mono"
	Version    string // e.g., "v1"
	ShortToken string // Short token for lookup (8 chars from UUID)
	LongSecret string // Long secret for authentication (43 chars base64)
	FullKey    string // Complete assembled key
}

// GenerateAPIKey creates a new API key following the pattern:
// {key_type}-{service}-{version}-{short_token}-{long_secret}
// Example: sk-mono-v1-a7f3d8e2-8h3k2jf9s7d6f5g4h3j2k1m0n9p8q7r6s5t4u3v2w1x
func GenerateAPIKey(keyType, service, version string) (*APIKeyParts, error) {
	// Generate short token from UUIDv7 (time-ordered)
	shortUUID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate UUID: %w", err)
	}
	shortToken := strings.ReplaceAll(shortUUID.String()[:8], "-", "")

	// Generate long secret (32 random bytes = 43 chars in base64)
	longBytes := make([]byte, 32)
	if _, err := rand.Read(longBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	longSecret := base64.RawURLEncoding.EncodeToString(longBytes)

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
func ParseAPIKey(apiKey string) (*APIKeyParts, error) {
	parts := strings.Split(apiKey, "-")
	if len(parts) != 5 {
		return nil, fmt.Errorf("invalid API key format: expected 5 parts, got %d", len(parts))
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
// Example: "sk-mono-v1-a7f3d8e2-****"
func (k *APIKeyParts) GetDisplayKey() string {
	return fmt.Sprintf("%s-%s-%s-%s-****", k.KeyType, k.Service, k.Version, k.ShortToken)
}
