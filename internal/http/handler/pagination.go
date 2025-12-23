package handler

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/rezkam/mono/internal/application/todo"
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

// parseOrderBy parses an AIP-132 order_by expression.
// Supports single-field sorting with optional direction.
// Full AIP-132 spec: https://google.aip.dev/132
//
// Examples:
//   - "due_time" -> field="due_time", direction="desc" (default)
//   - "due_time desc" -> field="due_time", direction="desc"
//   - "priority asc" -> field="priority", direction="asc"
//
// Returns field name and direction ("asc" or "desc").
func parseOrderBy(orderBy string) (field string, direction string, err error) {
	if orderBy == "" {
		return "created_at", "desc", nil // Default
	}

	// Split on whitespace to separate field and direction
	parts := strings.Fields(orderBy)

	if len(parts) == 0 {
		return "created_at", "desc", nil
	}

	field = parts[0]

	// Default direction is descending (common for time-based sorting)
	direction = "desc"

	if len(parts) >= 2 {
		dir := strings.ToLower(parts[1])
		if dir == "asc" {
			direction = "asc"
		} else if dir == "desc" {
			direction = "desc"
		} else {
			return "", "", fmt.Errorf("%w: '%s' (must be 'asc' or 'desc')", domain.ErrInvalidSortDirection, parts[1])
		}
	}

	// Ignore additional parts (multi-field not supported yet)
	return field, direction, nil
}
