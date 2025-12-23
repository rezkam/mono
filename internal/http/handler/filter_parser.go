package handler

import (
	"fmt"
	"strings"
	"time"

	"github.com/rezkam/mono/internal/domain"
)

// parseListFilter parses an AIP-160 filter expression for list filtering.
// Supports simple expressions like:
//   - title:"project"
//   - create_time > "2024-01-01T00:00:00Z"
//   - create_time < "2024-12-31T23:59:59Z"
//
// Returns a populated ListListsParams with filter fields set.
func parseListFilter(filter string) (domain.ListListsParams, error) {
	params := domain.ListListsParams{}

	if filter == "" {
		return params, nil
	}

	// Split by AND (simple implementation)
	clauses := strings.Split(filter, " AND ")

	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}

		// Parse title filter: title:"value" or title LIKE "value"
		if strings.HasPrefix(clause, "title:") || strings.Contains(clause, "title LIKE") {
			value := extractQuotedValue(clause)
			if value != "" {
				params.TitleContains = &value
			}
			continue
		}

		// Parse create_time filters
		if strings.Contains(clause, "create_time") {
			if strings.Contains(clause, ">") {
				timeStr := extractQuotedValue(clause)
				if timeStr != "" {
					t, err := time.Parse(time.RFC3339, timeStr)
					if err != nil {
						return params, fmt.Errorf("invalid create_time_after format: %w", err)
					}
					// Convert to UTC for consistency with database storage
					utcTime := t.UTC()
					params.CreateTimeAfter = &utcTime
				}
			} else if strings.Contains(clause, "<") {
				timeStr := extractQuotedValue(clause)
				if timeStr != "" {
					t, err := time.Parse(time.RFC3339, timeStr)
					if err != nil {
						return params, fmt.Errorf("invalid create_time_before format: %w", err)
					}
					// Convert to UTC for consistency with database storage
					utcTime := t.UTC()
					params.CreateTimeBefore = &utcTime
				}
			}
			continue
		}
	}

	return params, nil
}

// extractQuotedValue extracts a value from within quotes (single or double).
// Examples:
//   - "value" -> value
//   - 'value' -> value
//   - title:"value" -> value
func extractQuotedValue(s string) string {
	// Find the first quote
	startSingle := strings.Index(s, "'")
	startDouble := strings.Index(s, "\"")

	var start int
	var quote rune

	if startSingle >= 0 && (startDouble < 0 || startSingle < startDouble) {
		start = startSingle
		quote = '\''
	} else if startDouble >= 0 {
		start = startDouble
		quote = '"'
	} else {
		return ""
	}

	// Find the closing quote
	end := strings.IndexRune(s[start+1:], quote)
	if end < 0 {
		return ""
	}

	return s[start+1 : start+1+end]
}
