package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetPageSize_PassesRawValue verifies that getPageSize returns the raw value
// without applying compile-time limits. The service layer handles limits using
// its runtime configuration.
func TestGetPageSize_PassesRawValue(t *testing.T) {
	tests := []struct {
		name     string
		input    *int
		expected int
	}{
		{"nil returns 0", nil, 0},
		{"zero returns 0", intPtr(0), 0},
		{"valid value passed through", intPtr(42), 42},
		{"large value passed through", intPtr(1000), 1000}, // Service will clamp
		{"negative value passed through", intPtr(-5), -5},  // Service will handle
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getPageSize(tc.input)
			assert.Equal(t, tc.expected, result,
				"getPageSize should pass raw value to service layer")
		})
	}
}

func intPtr(i int) *int {
	return &i
}
