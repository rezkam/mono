package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
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

// maskAPIKey returns a safe-to-log version of an API key showing only the prefix.
// Example: "sk-mono-v1-a3f5d8c2b4e6-****" → "sk-***"
func maskAPIKey(apiKey string) string {
	parts := strings.Split(apiKey, "-")
	if len(parts) >= 1 {
		return parts[0] + "-***"
	}
	return "***"
}

// lastUsedUpdate holds information for updating an API key's last_used_at timestamp.
type lastUsedUpdate struct {
	keyID     uuid.UUID
	timestamp time.Time
}

// Authenticator handles API key authentication.
type Authenticator struct {
	queries         *sqlcgen.Queries
	db              *sql.DB
	appCtx          context.Context // Application context, cancelled on shutdown
	lastUsedUpdates chan lastUsedUpdate
	shutdownChan    chan struct{}
	wg              sync.WaitGroup
}

// NewAuthenticator creates a new authenticator and starts the background worker
// for processing last_used_at updates.
// The ctx parameter should be an application-level context that gets cancelled on shutdown.
func NewAuthenticator(ctx context.Context, db *sql.DB, queries *sqlcgen.Queries) *Authenticator {
	a := &Authenticator{
		queries:         queries,
		db:              db,
		appCtx:          ctx,
		lastUsedUpdates: make(chan lastUsedUpdate, 1000), // buffered to handle bursts
		shutdownChan:    make(chan struct{}),
	}

	// Start background worker for processing last_used_at updates
	a.wg.Add(1)
	go a.processLastUsedUpdates()

	return a
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
		// Log detailed error internally for debugging and security monitoring
		// DO NOT expose detailed error to client - prevents information disclosure attacks
		slog.WarnContext(ctx, "Authentication failed",
			slog.String("key_prefix", maskAPIKey(apiKey)),
			slog.String("error", err.Error()))

		// Return generic error (same message for all failure types)
		// This prevents attackers from:
		// - Enumerating valid short tokens
		// - Distinguishing between "not found" vs "wrong secret"
		// - Identifying expired keys
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	// Call the handler
	return handler(ctx, req)
}

// processLastUsedUpdates is a background worker that processes last_used_at updates
// from a buffered channel. This prevents goroutine explosion under high load.
func (a *Authenticator) processLastUsedUpdates() {
	defer a.wg.Done()

	for {
		select {
		case update := <-a.lastUsedUpdates:
			// Derive context from application context (respects shutdown)
			ctx, cancel := context.WithTimeout(a.appCtx, 5*time.Second)

			if err := a.queries.UpdateAPIKeyLastUsed(ctx, sqlcgen.UpdateAPIKeyLastUsedParams{
				LastUsedAt: sql.NullTime{Time: update.timestamp, Valid: true},
				ID:         update.keyID,
			}); err != nil {
				// Log failure but continue processing (last_used_at is non-critical)
				slog.WarnContext(ctx, "Failed to update API key last_used_at",
					slog.String("key_id", update.keyID.String()),
					slog.String("error", err.Error()))
			}
			cancel()

		case <-a.shutdownChan:
			// Drain remaining updates before shutdown
			for {
				select {
				case update := <-a.lastUsedUpdates:
					ctx, cancel := context.WithTimeout(a.appCtx, 5*time.Second)
					_ = a.queries.UpdateAPIKeyLastUsed(ctx, sqlcgen.UpdateAPIKeyLastUsedParams{
						LastUsedAt: sql.NullTime{Time: update.timestamp, Valid: true},
						ID:         update.keyID,
					})
					cancel()
				default:
					// No more updates, exit cleanly
					return
				}
			}
		}
	}
}

// Shutdown gracefully shuts down the authenticator by signaling the worker to stop
// and waiting for it to finish processing remaining updates.
// It respects the provided context's deadline for shutdown timeout.
func (a *Authenticator) Shutdown(ctx context.Context) error {
	// Signal worker to stop
	close(a.shutdownChan)

	// Wait for worker to finish, with timeout
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown timeout: %w", ctx.Err())
	}
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

	// Queue last_used_at update (non-blocking, processed by background worker)
	select {
	case a.lastUsedUpdates <- lastUsedUpdate{
		keyID:     key.ID,
		timestamp: time.Now().UTC(),
	}:
		// Successfully queued for async processing
	default:
		// Channel full, drop update (last_used_at is non-critical)
		// This provides backpressure - prevents unbounded goroutine spawning
		slog.WarnContext(ctx, "Dropped last_used_at update due to full queue",
			slog.String("key_id", key.ID.String()))
	}

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
