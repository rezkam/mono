package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/http/response"
)

// Auth is HTTP middleware for API key authentication.
type Auth struct {
	authenticator *auth.Authenticator
}

// NewAuth creates a new auth middleware.
func NewAuth(authenticator *auth.Authenticator) *Auth {
	return &Auth{
		authenticator: authenticator,
	}
}

// Validate is a Chi middleware that validates API keys from Authorization header.
// Expects format: "Authorization: Bearer <api-key>"
func (a *Auth) Validate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract API key from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			slog.WarnContext(r.Context(), "authentication failed: missing Authorization header",
				"path", r.URL.Path,
				"method", r.Method)
			response.Unauthorized(w, "missing Authorization header")
			return
		}

		// Parse Bearer token
		apiKey, found := strings.CutPrefix(authHeader, "Bearer ")
		if !found {
			slog.WarnContext(r.Context(), "authentication failed: invalid Authorization header format",
				"path", r.URL.Path,
				"method", r.Method)
			response.Unauthorized(w, "invalid Authorization header format, expected: Bearer <token>")
			return
		}

		// Validate API key using the authenticator
		validatedKey, err := a.authenticator.ValidateAPIKey(r.Context(), apiKey)
		if err != nil {
			// Log based on error type
			if errors.Is(err, domain.ErrUnauthorized) {
				slog.WarnContext(r.Context(), "authentication failed: invalid or expired API key",
					"path", r.URL.Path,
					"method", r.Method)
			} else {
				slog.ErrorContext(r.Context(), "authentication failed: unexpected error",
					"path", r.URL.Path,
					"method", r.Method,
					"error", err)
			}
			response.Unauthorized(w, "invalid or expired API key")
			return
		}

		// Optional: log successful authentication at DEBUG level
		slog.DebugContext(r.Context(), "authentication successful",
			"path", r.URL.Path,
			"method", r.Method,
			"key_id", validatedKey.ID,
			"key_name", validatedKey.Name)

		// Call next handler
		next.ServeHTTP(w, r)
	})
}
