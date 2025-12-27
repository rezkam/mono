package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
)

// payloadTooLargeJSON is a pre-marshaled error response for 413 Request Entity Too Large.
// Using a constant ensures we can always respond even if marshaling fails.
// Follows the standard error response format with code, message, and empty details array.
const payloadTooLargeJSON = `{"error":{"code":"PAYLOAD_TOO_LARGE","message":"request body exceeds size limit","details":[]}}`

// MaxBodyBytes creates a middleware that limits request body size.
// Uses a two-phase approach:
// 1. Fast path: Check Content-Length header for early rejection
// 2. Slow path: Read and verify body (handles chunked encoding and missing headers)
//
// Returns 413 Request Entity Too Large with standard error format if limit exceeded.
func MaxBodyBytes(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Fast path: Check Content-Length header first (if present and valid)
			// Content-Length of -1 means unknown (chunked encoding), so skip this check
			if r.ContentLength > 0 && r.ContentLength > maxBytes {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				if _, err := w.Write([]byte(payloadTooLargeJSON)); err != nil {
					slog.ErrorContext(r.Context(), "Failed to write payload too large response", "error", err)
				}
				return
			}

			// Slow path: Read body to verify size
			// Necessary because:
			// - Content-Length can be missing (chunked encoding)
			// - Content-Length can be spoofed
			// - MaxBytesReader enforces the limit during actual read
			body := http.MaxBytesReader(w, r.Body, maxBytes)
			buf, err := io.ReadAll(body)

			if err != nil {
				// Log the rejection for debugging
				slog.WarnContext(r.Context(), "Request body size limit exceeded",
					"method", r.Method,
					"path", r.URL.Path,
					"content_length", r.ContentLength,
					"limit", maxBytes,
					"error", err)

				// Body exceeded limit during read - return 413
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				if _, err := w.Write([]byte(payloadTooLargeJSON)); err != nil {
					slog.ErrorContext(r.Context(), "Failed to write payload too large response", "error", err)
				}
				return
			}

			// Body is within limit - replace it so handlers can read it
			r.Body = io.NopCloser(bytes.NewReader(buf))
			next.ServeHTTP(w, r)
		})
	}
}
