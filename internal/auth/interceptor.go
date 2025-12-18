package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/storage/sql/sqlcgen"
	"golang.org/x/crypto/blake2b"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// hashSecret computes BLAKE2b-256 hash of the secret and returns hex-encoded string.
// BLAKE2b is faster than SHA-256 while maintaining security for high-entropy API keys.
func hashSecret(secret string) string {
	hash := blake2b.Sum256([]byte(secret))
	return hex.EncodeToString(hash[:])
}

// Authenticator handles API key authentication.
type Authenticator struct {
	queries *sqlcgen.Queries
	db      *sql.DB
}

// NewAuthenticator creates a new authenticator.
func NewAuthenticator(db *sql.DB, queries *sqlcgen.Queries) *Authenticator {
	return &Authenticator{
		queries: queries,
		db:      db,
	}
}

// UnaryInterceptor is a gRPC unary interceptor for API key authentication.
func (a *Authenticator) UnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	// Extract API key from metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization header")
	}

	// Extract Bearer token
	authHeader := authHeaders[0]
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, status.Error(codes.Unauthenticated, "invalid authorization header format")
	}

	apiKey := strings.TrimPrefix(authHeader, "Bearer ")
	if apiKey == "" {
		return nil, status.Error(codes.Unauthenticated, "empty API key")
	}

	// Validate API key
	if err := a.validateAPIKey(ctx, apiKey); err != nil {
		return nil, status.Error(codes.Unauthenticated, fmt.Sprintf("invalid API key: %v", err))
	}

	// Call the handler
	return handler(ctx, req)
}

// validateAPIKey checks if the API key is valid and updates last_used_at.
// Uses O(1) indexed lookup via short_token instead of O(n) iteration.
func (a *Authenticator) validateAPIKey(ctx context.Context, apiKey string) error {
	// Parse API key into components
	keyParts, err := ParseAPIKey(apiKey)
	if err != nil {
		return fmt.Errorf("invalid API key format: %w", err)
	}

	// O(1) indexed lookup by short_token
	key, err := a.queries.GetAPIKeyByShortToken(ctx, keyParts.ShortToken)
	if err != nil {
		return fmt.Errorf("API key not found")
	}

	// Verify the long secret using BLAKE2b-256 with constant-time comparison
	providedHash := hashSecret(keyParts.LongSecret)
	if subtle.ConstantTimeCompare([]byte(key.LongSecretHash), []byte(providedHash)) != 1 {
		return fmt.Errorf("invalid API key")
	}

	// Check expiration
	if key.ExpiresAt.Valid && key.ExpiresAt.Time.Before(time.Now().UTC()) {
		return fmt.Errorf("API key expired")
	}

	// Update last used time (async to not slow down requests)
	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := a.queries.UpdateAPIKeyLastUsed(updateCtx, sqlcgen.UpdateAPIKeyLastUsedParams{
			LastUsedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true},
			ID:         key.ID,
		}); err != nil {
			// Log but don't fail authentication
			fmt.Printf("Warning: Failed to update API key last_used_at: %v\n", err)
		}
	}()

	return nil // Authentication successful
}

// CreateAPIKey creates a new API key and returns the plain key (only shown once).
func CreateAPIKey(ctx context.Context, queries *sqlcgen.Queries, keyType, service, version, name string, expiresAt *time.Time) (string, error) {
	// Generate API key with short+long pattern
	keyParts, err := GenerateAPIKey(keyType, service, version)
	if err != nil {
		return "", fmt.Errorf("failed to generate API key: %w", err)
	}

	// Hash the long secret using BLAKE2b-256
	longSecretHash := hashSecret(keyParts.LongSecret)

	// Store in database
	keyID, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("failed to generate key ID: %w", err)
	}
	var expiresAtSQL sql.NullTime
	if expiresAt != nil {
		expiresAtSQL = sql.NullTime{Time: *expiresAt, Valid: true}
	}

	err = queries.CreateAPIKey(ctx, sqlcgen.CreateAPIKeyParams{
		ID:             keyID,
		KeyType:        keyParts.KeyType,
		Service:        keyParts.Service,
		Version:        keyParts.Version,
		ShortToken:     keyParts.ShortToken,
		LongSecretHash: longSecretHash,
		Name:           name,
		IsActive:       true,
		CreatedAt:      time.Now().UTC(),
		ExpiresAt:      expiresAtSQL,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create API key: %w", err)
	}

	// Return the FULL plain key (this is the ONLY time it will be visible)
	return keyParts.FullKey, nil
}
