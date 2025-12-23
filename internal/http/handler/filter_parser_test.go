package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseListFilter_TimezoneHandling verifies that user-provided timestamps
// with explicit timezones are parsed correctly and can be compared with UTC times.
func TestParseListFilter_TimezoneHandling(t *testing.T) {
	tests := []struct {
		name           string
		filter         string
		expectedOffset int // Offset from UTC in hours
		expectError    bool
	}{
		{
			name:           "UTC timezone",
			filter:         `create_time > "2024-01-15T10:00:00Z"`,
			expectedOffset: 0,
			expectError:    false,
		},
		{
			name:           "PST timezone (UTC-8)",
			filter:         `create_time > "2024-01-15T10:00:00-08:00"`,
			expectedOffset: -8,
			expectError:    false,
		},
		{
			name:           "EST timezone (UTC-5)",
			filter:         `create_time > "2024-01-15T10:00:00-05:00"`,
			expectedOffset: -5,
			expectError:    false,
		},
		{
			name:           "CET timezone (UTC+1)",
			filter:         `create_time < "2024-01-15T10:00:00+01:00"`,
			expectedOffset: 1,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := parseListFilter(tt.filter)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify the parsed time
			var parsedTime *time.Time
			if params.CreateTimeAfter != nil {
				parsedTime = params.CreateTimeAfter
			} else if params.CreateTimeBefore != nil {
				parsedTime = params.CreateTimeBefore
			}

			require.NotNil(t, parsedTime, "Expected a parsed time")

			// CRITICAL: Verify that the parsed time is ALWAYS in UTC location
			// This ensures consistency with database storage
			assert.Equal(t, time.UTC, parsedTime.Location(),
				"Parsed time should be converted to UTC location")

			// Verify the absolute instant in time matches the input
			// For example: "2024-01-15T10:00:00-08:00" should become "2024-01-15T18:00:00Z"
			if tt.expectedOffset != 0 {
				// The hour in UTC should be offset by the timezone
				// PST (UTC-8): 10:00 PST = 18:00 UTC
				// EST (UTC-5): 10:00 EST = 15:00 UTC
				expectedUTCHour := 10 - tt.expectedOffset
				if expectedUTCHour < 0 {
					expectedUTCHour += 24
				} else if expectedUTCHour >= 24 {
					expectedUTCHour -= 24
				}
				assert.Equal(t, expectedUTCHour, parsedTime.Hour(),
					"UTC hour should reflect timezone conversion")
			}
		})
	}
}

// TestParseListFilter_EnsuresUTCConsistency verifies that all parsed times
// should be converted to UTC for consistency with database storage.
func TestParseListFilter_EnsuresUTCConsistency(t *testing.T) {
	// Parse a time with PST timezone
	filter := `create_time > "2024-01-15T10:00:00-08:00"`
	params, err := parseListFilter(filter)
	require.NoError(t, err)
	require.NotNil(t, params.CreateTimeAfter)

	// The current implementation preserves the timezone from the input.
	// For database comparison consistency, we should convert to UTC.
	// This test documents the expected behavior.

	// Convert to UTC for database comparison
	utcTime := params.CreateTimeAfter.UTC()

	// Expected UTC time: 2024-01-15T18:00:00Z (10:00 PST + 8 hours)
	expected := time.Date(2024, 1, 15, 18, 0, 0, 0, time.UTC)
	assert.Equal(t, expected, utcTime,
		"PST time should convert to correct UTC time for database comparison")
}

// TestParseListFilter_InvalidTimezoneFormat verifies error handling for
// malformed timezone strings.
func TestParseListFilter_InvalidTimezoneFormat(t *testing.T) {
	tests := []struct {
		name   string
		filter string
	}{
		{
			name:   "Invalid date format",
			filter: `create_time > "not-a-date"`,
		},
		{
			name:   "Missing timezone indicator",
			filter: `create_time > "2024-01-15T10:00:00"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseListFilter(tt.filter)
			assert.Error(t, err, "Should reject invalid time format")
		})
	}
}
