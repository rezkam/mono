package middleware

import (
	"net/http"
	"strings"

	"github.com/rezkam/mono/internal/application/auth"
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
			response.Unauthorized(w, "missing Authorization header")
			return
		}

		// Parse Bearer token
		apiKey, found := strings.CutPrefix(authHeader, "Bearer ")
		if !found {
			response.Unauthorized(w, "invalid Authorization header format, expected: Bearer <token>")
			return
		}

		// Validate API key using the authenticator
		_, err := a.authenticator.ValidateAPIKey(r.Context(), apiKey)
		if err != nil {
			response.Unauthorized(w, "invalid or expired API key")
			return
		}

		// Call next handler
		next.ServeHTTP(w, r)
	})
}
