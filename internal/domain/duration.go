package domain

import (
	"fmt"
	"strings"
	"time"
)

// Duration is a validated duration value object.
// Only accepts ISO 8601 duration format (e.g., "PT1H30M").
type Duration struct {
	value time.Duration
}

// NewDuration creates a new Duration from an ISO 8601 duration string.
// Only supports time components (hours, minutes, seconds), not date components.
// Examples: "PT1H", "PT30M", "PT1H30M", "PT1H30M15S"
func NewDuration(s string) (Duration, error) {
	if s == "" {
		return Duration{}, ErrDurationEmpty
	}

	d, err := parseISO8601Duration(s)
	if err != nil {
		return Duration{}, err
	}

	return Duration{value: d}, nil
}

// Value returns the underlying time.Duration.
func (d Duration) Value() time.Duration {
	return d.value
}

// String returns the ISO 8601 representation of the duration.
func (d Duration) String() string {
	return FormatDurationISO8601(d.value)
}

// FormatDurationISO8601 converts a time.Duration to ISO 8601 format (e.g., "PT1H30M").
func FormatDurationISO8601(d time.Duration) string {
	if d == 0 {
		return "PT0S"
	}

	var b strings.Builder
	b.WriteString("PT")

	remaining := d

	if hours := remaining / time.Hour; hours > 0 {
		fmt.Fprintf(&b, "%dH", hours)
		remaining %= time.Hour
	}

	if minutes := remaining / time.Minute; minutes > 0 {
		fmt.Fprintf(&b, "%dM", minutes)
		remaining %= time.Minute
	}

	if seconds := remaining / time.Second; seconds > 0 {
		fmt.Fprintf(&b, "%dS", seconds)
	}

	return b.String()
}

// parseISO8601Duration parses ISO 8601 duration format (e.g., "PT1H30M10S").
// Only supports time components (hours, minutes, seconds), not date components.
func parseISO8601Duration(s string) (time.Duration, error) {
	if len(s) < 2 || s[0] != 'P' {
		return 0, fmt.Errorf("%w: must start with 'P' (e.g., 'PT1H30M')", ErrInvalidDurationFormat)
	}

	// Remove 'P' prefix
	s = s[1:]

	// Skip date portion if present (before 'T')
	if idx := strings.Index(s, "T"); idx >= 0 {
		s = s[idx+1:]
	} else if len(s) > 0 && (s[0] == 'T') {
		s = s[1:]
	} else {
		// No 'T' means only date components, which we don't support
		return 0, fmt.Errorf("%w: must include time component 'T' (e.g., 'PT1H30M')", ErrInvalidDurationFormat)
	}

	if len(s) == 0 {
		return 0, fmt.Errorf("%w: empty duration after 'PT'", ErrInvalidDurationFormat)
	}

	var duration time.Duration
	var numBuf strings.Builder

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' || c == '.' {
			numBuf.WriteByte(c)
		} else {
			if numBuf.Len() == 0 {
				return 0, fmt.Errorf("%w: missing number before '%c'", ErrInvalidDurationFormat, c)
			}
			numStr := numBuf.String()
			numBuf.Reset()

			// Parse as float to support decimals
			var num float64
			if _, err := fmt.Sscanf(numStr, "%f", &num); err != nil {
				return 0, fmt.Errorf("%w: invalid number: %s", ErrInvalidDurationFormat, numStr)
			}

			switch c {
			case 'H':
				duration += time.Duration(num * float64(time.Hour))
			case 'M':
				duration += time.Duration(num * float64(time.Minute))
			case 'S':
				duration += time.Duration(num * float64(time.Second))
			default:
				return 0, fmt.Errorf("%w: unknown unit '%c' (valid: H, M, S)", ErrInvalidDurationFormat, c)
			}
		}
	}

	if numBuf.Len() > 0 {
		return 0, fmt.Errorf("%w: trailing number without unit", ErrInvalidDurationFormat)
	}

	return duration, nil
}
