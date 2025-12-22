package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDuration_ISO8601(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		// Basic ISO 8601 formats
		{"minutes only", "PT10M", 10 * time.Minute},
		{"hours only", "PT2H", 2 * time.Hour},
		{"seconds only", "PT30S", 30 * time.Second},
		{"hours and minutes", "PT1H30M", 1*time.Hour + 30*time.Minute},
		{"hours minutes seconds", "PT2H30M15S", 2*time.Hour + 30*time.Minute + 15*time.Second},
		{"minutes and seconds", "PT5M30S", 5*time.Minute + 30*time.Second},

		// Fractional values
		{"fractional hours", "PT1.5H", 1*time.Hour + 30*time.Minute},
		{"fractional minutes", "PT30.5M", 30*time.Minute + 30*time.Second},
		{"fractional seconds", "PT10.5S", 10*time.Second + 500*time.Millisecond},

		// Edge cases
		{"zero duration", "PT0S", 0},
		{"large values", "PT24H", 24 * time.Hour},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseDuration(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestParseDuration_GoFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{"minutes only", "10m", 10 * time.Minute},
		{"hours only", "2h", 2 * time.Hour},
		{"seconds only", "30s", 30 * time.Second},
		{"hours and minutes", "1h30m", 1*time.Hour + 30*time.Minute},
		{"complex", "2h30m15s", 2*time.Hour + 30*time.Minute + 15*time.Second},
		{"milliseconds", "500ms", 500 * time.Millisecond},
		{"microseconds", "100us", 100 * time.Microsecond},
		{"nanoseconds", "1000ns", 1000 * time.Nanosecond},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseDuration(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestParseDuration_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"invalid format", "invalid"},
		{"just P", "P"},
		{"date only (no T)", "P1D"},
		{"missing number", "PTM"},
		{"trailing number", "PT10"},
		{"unknown unit", "PT10X"},
		{"random text", "hello"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseDuration(tc.input)
			assert.Error(t, err)
		})
	}
}

func TestParseISO8601Duration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		// Valid formats
		{"basic minutes", "PT10M", 10 * time.Minute, false},
		{"basic hours", "PT2H", 2 * time.Hour, false},
		{"basic seconds", "PT45S", 45 * time.Second, false},
		{"combined HMS", "PT1H30M45S", 1*time.Hour + 30*time.Minute + 45*time.Second, false},
		{"combined HM", "PT2H15M", 2*time.Hour + 15*time.Minute, false},
		{"combined MS", "PT15M30S", 15*time.Minute + 30*time.Second, false},

		// Fractional values
		{"fractional hours", "PT0.5H", 30 * time.Minute, false},
		{"fractional minutes", "PT1.5M", 90 * time.Second, false},
		{"fractional seconds", "PT0.001S", time.Millisecond, false},

		// With date portion (skipped)
		{"date and time", "P1DT2H", 2 * time.Hour, false},

		// Edge case: PT with nothing after = 0 duration (valid)
		{"empty after PT", "PT", 0, false},

		// Invalid formats
		{"not ISO format", "10m", 0, true},
		{"no P prefix", "T10M", 0, true},
		{"date only", "P1D", 0, true},
		{"P1Y date", "P1Y", 0, true},
		{"missing number before unit", "PTH", 0, true},
		{"trailing number", "PT10", 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseISO8601Duration(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestParseDuration_RealWorldExamples(t *testing.T) {
	// Common durations users might enter
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{"10 minute task", "PT10M", 10 * time.Minute},
		{"30 minute meeting", "PT30M", 30 * time.Minute},
		{"1 hour meeting", "PT1H", 1 * time.Hour},
		{"1.5 hour session", "PT1H30M", 1*time.Hour + 30*time.Minute},
		{"2 hour workshop", "PT2H", 2 * time.Hour},
		{"quick 5 min task", "PT5M", 5 * time.Minute},
		{"full day", "PT8H", 8 * time.Hour},
		{"pomodoro", "PT25M", 25 * time.Minute},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseDuration(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
