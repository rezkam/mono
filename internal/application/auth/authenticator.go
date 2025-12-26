package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/keygen"
)

// Default configuration values.
const (
	DefaultOperationTimeout = 5 * time.Second
	DefaultUpdateQueueSize  = 1000
)

// Config holds configuration for the Authenticator.
type Config struct {
	OperationTimeout time.Duration // Timeout for storage operations
	UpdateQueueSize  int           // Buffer size for last_used_at updates
}

// lastUsedUpdate holds information for updating an API key's last_used_at timestamp.
type lastUsedUpdate struct {
	keyID     string
	timestamp time.Time
}

// Authenticator handles API key authentication.
type Authenticator struct {
	repo             Repository
	lastUsedUpdates  chan lastUsedUpdate
	wg               sync.WaitGroup
	operationTimeout time.Duration // Timeout for storage operations

	// Shutdown coordination
	shutdownChan chan struct{} // Closed to signal shutdown (instant notification)
	shutdownOnce sync.Once     // Ensures Shutdown logic runs exactly once

	// Lifecycle context for cancelling all operations on shutdown.
	// This is NOT a request-scoped context passed from callers.
	// It's created internally to manage the authenticator's operation lifecycle.
	// See: https://go.dev/blog/context-and-structs (acceptable for lifecycle management)
	opsCtx    context.Context
	opsCancel context.CancelFunc
}

// NewAuthenticator creates a new authenticator and starts the background worker
// for processing last_used_at updates.
// Call Shutdown(ctx) to stop the authenticator during application shutdown.
// Applies application defaults for negative config values.
// Zero OperationTimeout means no timeout (operations wait indefinitely).
// Zero UpdateQueueSize gets the default (must be > 0 to avoid blocking).
func NewAuthenticator(repo Repository, config Config) *Authenticator {
	// Apply defaults only for negative values
	// Zero timeout means "no timeout" (wait indefinitely)
	// Zero queue size is invalid (would block), so use default
	if config.OperationTimeout < 0 {
		config.OperationTimeout = DefaultOperationTimeout
	}
	if config.UpdateQueueSize <= 0 {
		config.UpdateQueueSize = DefaultUpdateQueueSize
	}

	// Create lifecycle context for all operations
	opsCtx, opsCancel := context.WithCancel(context.Background())

	a := &Authenticator{
		repo:             repo,
		lastUsedUpdates:  make(chan lastUsedUpdate, config.UpdateQueueSize),
		operationTimeout: config.OperationTimeout,
		opsCtx:           opsCtx,
		opsCancel:        opsCancel,
		shutdownChan:     make(chan struct{}),
	}

	// Start background worker for processing last_used_at updates
	a.wg.Add(1)
	go a.processLastUsedUpdates()

	return a
}

// processLastUsedUpdates is a background worker that processes last_used_at updates
// from a buffered channel. This prevents goroutine explosion under high load.
func (a *Authenticator) processLastUsedUpdates() {
	defer a.wg.Done()

	for {
		select {
		case update := <-a.lastUsedUpdates:
			a.processUpdate(update)

		case <-a.shutdownChan:
			// Graceful shutdown requested - drain remaining updates
			a.drainQueue()
			slog.Info("Authenticator worker stopped")
			return

		case <-a.opsCtx.Done():
			// All operations cancelled - exit immediately
			slog.Info("Authenticator operations cancelled, exiting")
			return
		}
	}
}

// drainQueue processes remaining updates until queue is empty or shutdown timeout.
func (a *Authenticator) drainQueue() {
	slog.Info("Draining remaining last_used updates...")

	for {
		select {
		case update := <-a.lastUsedUpdates:
			a.processUpdate(update)

		case <-a.opsCtx.Done():
			// Shutdown timeout - stop draining
			remaining := len(a.lastUsedUpdates)
			if remaining > 0 {
				slog.Warn("Shutdown cancelled operations, dropping remaining updates",
					slog.Int("dropped_count", remaining))
			}
			return

		default:
			// Queue empty
			return
		}
	}
}

// processUpdate handles a single last_used_at update.
func (a *Authenticator) processUpdate(update lastUsedUpdate) {
	// All operations derive from opsCtx (cancellable on shutdown)
	ctx := a.opsCtx
	var cancel context.CancelFunc
	if a.operationTimeout > 0 {
		ctx, cancel = context.WithTimeout(a.opsCtx, a.operationTimeout)
		defer cancel()
	}

	if err := a.repo.UpdateLastUsed(ctx, update.keyID, update.timestamp); err != nil {
		// Don't log if cancelled (shutdown in progress)
		if errors.Is(err, context.Canceled) {
			return
		}
		slog.WarnContext(ctx, "Failed to update API key last_used_at",
			slog.String("key_id", update.keyID),
			slog.String("error", err.Error()))
	}
}

// Shutdown gracefully stops the authenticator.
// Drains remaining updates while ctx is valid.
// When ctx expires, cancels all in-flight operations and returns immediately.
// Safe to call multiple times - only the first call has effect.
func (a *Authenticator) Shutdown(ctx context.Context) error {
	var shutdownErr error

	a.shutdownOnce.Do(func() {
		slog.InfoContext(ctx, "Authenticator shutdown initiated")

		// Signal worker to stop accepting new work (instant notification)
		close(a.shutdownChan)

		// Wait for worker to finish OR timeout
		workerDone := make(chan struct{})
		go func() {
			a.wg.Wait()
			close(workerDone)
		}()

		select {
		case <-workerDone:
			slog.InfoContext(ctx, "Authenticator shutdown completed")
			shutdownErr = nil

		case <-ctx.Done():
			// Timeout expired - cancel ALL operations and return immediately
			slog.WarnContext(ctx, "Authenticator shutdown timeout, cancelled all operations")
			shutdownErr = ctx.Err()
		}

		// Always release lifecycle context resources (on both success and timeout)
		a.opsCancel()
	})

	return shutdownErr
}

// ValidateAPIKey validates an API key and returns the key information if valid.
// Returns domain.ErrUnauthorized if the key is invalid, expired, or not found.
//
// Security: This implementation is resistant to timing attacks that could reveal
// which short tokens exist in the database. It uses WithDataIndependentTiming
// to enable hardware-level timing guarantees and always computes the hash
// (even for non-existent keys) to maintain constant timing regardless of whether
// the short token exists.
func (a *Authenticator) ValidateAPIKey(ctx context.Context, apiKey string) (*domain.APIKey, error) {
	// Parse API key into components
	keyParts, err := keygen.ParseAPIKey(apiKey)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	// Create timeout context for storage operation (if timeout is configured)
	// Zero timeout means no timeout - use parent context directly
	opCtx := ctx
	if a.operationTimeout > 0 {
		var cancel context.CancelFunc
		opCtx, cancel = context.WithTimeout(ctx, a.operationTimeout)
		defer cancel()
	}

	// Lookup short token in database
	key, lookupErr := a.repo.FindByShortToken(opCtx, keyParts.ShortToken)

	// Perform all cryptographic validation in constant-time region
	// This prevents timing attacks by ensuring operations take the same time
	// regardless of whether the key exists or the comparison succeeds.
	var isValid, isExpired int
	subtle.WithDataIndependentTiming(func() {
		// CRITICAL: Always compute hash, even if key doesn't exist
		// This prevents timing attacks that distinguish between:
		//   - Non-existent keys (fast - no hash computation)
		//   - Existing keys (slow - hash computation)
		// BLAKE2b timing depends on input length, not content.
		// All API key secrets are 43 chars, so timing is constant.
		providedHash := keygen.HashSecret(keyParts.LongSecret)

		// Select hash to compare against
		var storedHash string
		if key != nil && lookupErr == nil {
			storedHash = key.LongSecretHash
		} else {
			// Dummy hash for non-existent keys (64 hex chars for BLAKE2b-256)
			// This ensures constant-time comparison path is taken even when key doesn't exist
			storedHash = "0000000000000000000000000000000000000000000000000000000000000000"
		}

		// Constant-time comparison (required even inside WithDataIndependentTiming)
		// WithDataIndependentTiming provides hardware-level protection,
		// but you still need algorithm-level constant-time operations.
		isValid = subtle.ConstantTimeCompare([]byte(storedHash), []byte(providedHash))

		// Check expiration in constant-time region
		if key != nil && key.ExpiresAt != nil {
			if key.ExpiresAt.Before(time.Now().UTC()) {
				isExpired = 1
			}
		}
	})

	// Log lookup errors (outside timing-critical region)
	if lookupErr != nil && !errors.Is(lookupErr, domain.ErrNotFound) {
		slog.ErrorContext(ctx, "Failed to look up API key", "error", lookupErr)
	}

	// Check all failure conditions
	if lookupErr != nil || isValid != 1 || isExpired == 1 {
		return nil, domain.ErrUnauthorized
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
			slog.String("key_id", key.ID))
	}

	return key, nil
}

// CreateAPIKey creates a new API key and returns the plain key (only shown once).
func CreateAPIKey(ctx context.Context, repo Repository, keyType, service, version, name string, expiresAt *time.Time) (string, error) {
	// Generate API key with short+long pattern
	keyParts, err := keygen.GenerateAPIKey(keyType, service, version)
	if err != nil {
		return "", fmt.Errorf("failed to generate API key: %w", err)
	}

	// Hash the long secret using BLAKE2b-256
	longSecretHash := keygen.HashSecret(keyParts.LongSecret)

	// Store in repository
	keyID, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("failed to generate key ID: %w", err)
	}

	err = repo.Create(ctx, &domain.APIKey{
		ID:             keyID.String(),
		KeyType:        keyParts.KeyType,
		Service:        keyParts.Service,
		Version:        keyParts.Version,
		ShortToken:     keyParts.ShortToken,
		LongSecretHash: longSecretHash,
		Name:           name,
		IsActive:       true,
		CreatedAt:      time.Now().UTC(),
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create API key: %w", err)
	}

	// Return the FULL plain key (this is the ONLY time it will be visible)
	return keyParts.FullKey, nil
}
