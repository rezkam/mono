package auth

import (
	"context"
	"crypto/subtle"
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
	appCtx           context.Context // Application context, cancelled on shutdown
	lastUsedUpdates  chan lastUsedUpdate
	shutdownChan     chan struct{}
	shutdownOnce     sync.Once // Ensures shutdown is idempotent
	wg               sync.WaitGroup
	operationTimeout time.Duration // Timeout for storage operations
}

// NewAuthenticator creates a new authenticator and starts the background worker
// for processing last_used_at updates.
// The ctx parameter should be an application-level context that gets cancelled on shutdown.
// Applies application defaults for negative config values.
// Zero OperationTimeout means no timeout (operations wait indefinitely).
// Zero UpdateQueueSize gets the default (must be > 0 to avoid blocking).
func NewAuthenticator(ctx context.Context, repo Repository, config Config) *Authenticator {
	// Apply defaults only for negative values
	// Zero timeout means "no timeout" (wait indefinitely)
	// Zero queue size is invalid (would block), so use default
	if config.OperationTimeout < 0 {
		config.OperationTimeout = DefaultOperationTimeout
	}
	if config.UpdateQueueSize <= 0 {
		config.UpdateQueueSize = DefaultUpdateQueueSize
	}

	a := &Authenticator{
		repo:             repo,
		appCtx:           ctx,
		lastUsedUpdates:  make(chan lastUsedUpdate, config.UpdateQueueSize),
		shutdownChan:     make(chan struct{}),
		operationTimeout: config.OperationTimeout,
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
			// Derive context from application context (respects shutdown)
			// Note: We call cancel() explicitly instead of using defer because
			// defer in a loop defers until function exit, causing resource leaks.
			ctx, cancel := context.WithTimeout(a.appCtx, a.operationTimeout)

			if err := a.repo.UpdateLastUsed(ctx, update.keyID, update.timestamp); err != nil {
				// Log failure but continue processing (last_used_at is non-critical)
				slog.WarnContext(ctx, "Failed to update API key last_used_at",
					slog.String("key_id", update.keyID),
					slog.String("error", err.Error()))
			}
			cancel() // Release context resources immediately after each iteration

		case <-a.shutdownChan:
			// Drain remaining updates before shutdown
			for {
				select {
				case update := <-a.lastUsedUpdates:
					// Use context.Background() for cleanup operations to ensure they complete
					// even though appCtx is cancelled during shutdown. This is defensive:
					// cleanup operations should succeed regardless of application state.
					// The timeout still applies to prevent hanging on storage issues.
					ctx, cancel := context.WithTimeout(context.Background(), a.operationTimeout)
					_ = a.repo.UpdateLastUsed(ctx, update.keyID, update.timestamp)
					cancel() // Release context resources immediately after each iteration
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
// This method is idempotent and safe to call multiple times.
func (a *Authenticator) Shutdown(ctx context.Context) error {
	var shutdownErr error
	a.shutdownOnce.Do(func() {
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
			shutdownErr = nil
		case <-ctx.Done():
			shutdownErr = fmt.Errorf("shutdown timeout: %w", ctx.Err())
		}
	})
	return shutdownErr
}

// ValidateAPIKey validates an API key and returns the key information if valid.
// Returns domain.ErrUnauthorized if the key is invalid, expired, or not found.
// This is the public method for HTTP middleware and other transport layers.
func (a *Authenticator) ValidateAPIKey(ctx context.Context, apiKey string) (*domain.APIKey, error) {
	// Parse API key into components
	keyParts, err := keygen.ParseAPIKey(apiKey)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	// Create timeout context for storage operation
	opCtx, cancel := context.WithTimeout(ctx, a.operationTimeout)
	defer cancel()

	key, err := a.repo.FindByShortToken(opCtx, keyParts.ShortToken)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	// Verify the long secret using BLAKE2b-256 with constant-time comparison
	providedHash := keygen.HashSecret(keyParts.LongSecret)
	if subtle.ConstantTimeCompare([]byte(key.LongSecretHash), []byte(providedHash)) != 1 {
		return nil, domain.ErrUnauthorized
	}

	// Check expiration
	if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now().UTC()) {
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
