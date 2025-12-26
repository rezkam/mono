package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDuration_Valid(t *testing.T) {
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
		{"zero seconds", "PT0S", 0},
		{"large values", "PT24H", 24 * time.Hour},

		// Date portion skipped (only time used)
		{"date and time", "P1DT2H", 2 * time.Hour},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := NewDuration(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, d.Value())
		})
	}
}

func TestNewDuration_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		// Empty/invalid
		{"empty string", ""},
		{"random text", "hello"},
		{"just P", "P"},

		// Go format NOT supported
		{"go format minutes", "10m"},
		{"go format hours", "2h"},
		{"go format complex", "1h30m"},
		{"go format milliseconds", "500ms"},

		// Invalid ISO 8601
		{"no P prefix", "T10M"},
		{"date only (no T)", "P1D"},
		{"date only year", "P1Y"},
		{"empty after PT", "PT"},
		{"missing number before unit", "PTM"},
		{"trailing number", "PT10"},
		{"unknown unit", "PT10X"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewDuration(tc.input)
			assert.Error(t, err)
		})
	}
}

func TestNewDuration_RealWorldExamples(t *testing.T) {
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
			d, err := NewDuration(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, d.Value())
		})
	}
}

func TestDuration_String(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"zero", "PT0S", "PT0S"},
		{"hours only", "PT2H", "PT2H"},
		{"minutes only", "PT30M", "PT30M"},
		{"seconds only", "PT45S", "PT45S"},
		{"hours and minutes", "PT1H30M", "PT1H30M"},
		{"all components", "PT2H30M15S", "PT2H30M15S"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := NewDuration(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, d.String())
		})
	}
}

func TestDuration_ValueReturnsUnderlyingDuration(t *testing.T) {
	d, err := NewDuration("PT1H30M")
	require.NoError(t, err)

	expected := 1*time.Hour + 30*time.Minute
	assert.Equal(t, expected, d.Value())

	// Verify it's a proper time.Duration
	assert.Equal(t, "1h30m0s", d.Value().String())
}
