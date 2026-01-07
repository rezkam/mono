package handler

import (
	"encoding/base64"
	"strconv"

	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/ptr"
)

// generatePageToken creates a pagination token from an offset value.
// Returns nil if there are no more pages.
func generatePageToken(offset int, hasMore bool) *string {
	if !hasMore {
		return nil
	}

	// Encode the next offset as a base64 string
	token := base64.URLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
	return &token
}

// parsePageToken decodes a pagination token to get the offset.
// Returns (0, nil) if token is nil or empty (first page).
// Returns (0, ErrInvalidPageToken) if token is malformed or invalid.
func parsePageToken(token *string) (int, error) {
	// nil or empty token is valid - means first page
	if token == nil || *token == "" {
		return 0, nil
	}

	// Decode base64
	decoded, err := base64.URLEncoding.DecodeString(*token)
	if err != nil {
		return 0, domain.ErrInvalidPageToken
	}

	// Parse integer
	offset, err := strconv.Atoi(string(decoded))
	if err != nil {
		return 0, domain.ErrInvalidPageToken
	}

	// Reject negative offsets
	if offset < 0 {
		return 0, domain.ErrInvalidPageToken
	}

	return offset, nil
}

// getPageSize returns the requested page size, or 0 if not specified.
// The service layer applies configured defaults and limits.
func getPageSize(pageSize *int) int {
	return ptr.Deref(pageSize, 0)
}
