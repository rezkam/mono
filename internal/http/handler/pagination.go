package handler

import (
	"encoding/base64"
	"strconv"

	"github.com/rezkam/mono/internal/application/todo"
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
// Returns 0 if token is empty, invalid, or contains a negative value.
func parsePageToken(token *string) int {
	if token == nil || *token == "" {
		return 0
	}

	decoded, err := base64.URLEncoding.DecodeString(*token)
	if err != nil {
		return 0
	}

	offset, err := strconv.Atoi(string(decoded))
	if err != nil {
		return 0
	}

	// Reject negative offsets to prevent slice bounds panic
	if offset < 0 {
		return 0
	}

	return offset
}

// getPageSize returns the page size, using service layer defaults.
// Delegates to todo.DefaultPageSize and todo.MaxPageSize to maintain
// single source of truth for pagination configuration.
func getPageSize(pageSize *int) int {
	size := ptr.Deref(pageSize, todo.DefaultPageSize)
	if size <= 0 {
		return todo.DefaultPageSize
	}
	if size > todo.MaxPageSize {
		return todo.MaxPageSize
	}
	return size
}
