package recurring

import (
	"testing"
	"time"

	"github.com/rezkam/mono/internal/core"
)

// TestDailyCalculator tests daily recurrence pattern
func TestDailyCalculator(t *testing.T) {
	calc := &DailyCalculator{}
	start := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("default interval (1 day)", func(t *testing.T) {
		config := map[string]interface{}{}
		next := calc.NextOccurrence(start, config)
		if next == nil {
			t.Fatal("expected next occurrence, got nil")
		}
		expected := start.AddDate(0, 0, 1)
		if !next.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, *next)
		}
	})

	t.Run("custom interval (3 days)", func(t *testing.T) {
		config := map[string]interface{}{"interval": 3.0}
		next := calc.NextOccurrence(start, config)
		expected := start.AddDate(0, 0, 3)
		if !next.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, *next)
		}
	})

	t.Run("occurrences between", func(t *testing.T) {
		config := map[string]interface{}{"interval": 2.0}
		end := start.AddDate(0, 0, 7) // 7 days later
		occurrences := calc.OccurrencesBetween(start, end, config)
		// Should get: day 0, 2, 4, 6 = 4 occurrences
		if len(occurrences) != 4 {
			t.Errorf("expected 4 occurrences, got %d", len(occurrences))
		}
	})
}

// TestWeeklyCalculator tests weekly recurrence pattern
func TestWeeklyCalculator(t *testing.T) {
	calc := &WeeklyCalculator{}
	start := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC) // Wednesday

	t.Run("default interval (1 week)", func(t *testing.T) {
		config := map[string]interface{}{}
		next := calc.NextOccurrence(start, config)
		expected := start.AddDate(0, 0, 7)
		if !next.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, *next)
		}
	})

	t.Run("custom interval (2 weeks)", func(t *testing.T) {
		config := map[string]interface{}{"interval": 2.0}
		next := calc.NextOccurrence(start, config)
		expected := start.AddDate(0, 0, 14)
		if !next.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, *next)
		}
	})
}

// TestMonthlyCalculator tests monthly recurrence pattern
func TestMonthlyCalculator(t *testing.T) {
	calc := &MonthlyCalculator{}
	start := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	t.Run("default interval (1 month)", func(t *testing.T) {
		config := map[string]interface{}{}
		next := calc.NextOccurrence(start, config)
		expected := start.AddDate(0, 1, 0)
		if !next.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, *next)
		}
	})

	t.Run("custom interval (3 months)", func(t *testing.T) {
		config := map[string]interface{}{"interval": 3.0}
		next := calc.NextOccurrence(start, config)
		expected := start.AddDate(0, 3, 0)
		if !next.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, *next)
		}
	})

	t.Run("year boundary", func(t *testing.T) {
		dec := time.Date(2024, 12, 15, 12, 0, 0, 0, time.UTC)
		config := map[string]interface{}{}
		next := calc.NextOccurrence(dec, config)
		expected := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
		if !next.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, *next)
		}
	})
}

// TestWeekdaysCalculator tests weekdays-only recurrence
func TestWeekdaysCalculator(t *testing.T) {
	calc := &WeekdaysCalculator{}

	t.Run("skip weekend", func(t *testing.T) {
		friday := time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC) // Friday
		config := map[string]interface{}{}
		next := calc.NextOccurrence(friday, config)
		// Should skip Saturday and Sunday, land on Monday
		monday := time.Date(2025, 1, 6, 12, 0, 0, 0, time.UTC)
		if !next.Equal(monday) {
			t.Errorf("expected %v (Monday), got %v", monday, *next)
		}
	})

	t.Run("weekday to weekday", func(t *testing.T) {
		monday := time.Date(2025, 1, 6, 12, 0, 0, 0, time.UTC) // Monday
		config := map[string]interface{}{}
		next := calc.NextOccurrence(monday, config)
		tuesday := time.Date(2025, 1, 7, 12, 0, 0, 0, time.UTC)
		if !next.Equal(tuesday) {
			t.Errorf("expected %v (Tuesday), got %v", tuesday, *next)
		}
	})

	t.Run("occurrences between includes only weekdays", func(t *testing.T) {
		// Start Monday, end next Monday (7 days)
		start := time.Date(2025, 1, 6, 12, 0, 0, 0, time.UTC)
		end := time.Date(2025, 1, 13, 12, 0, 0, 0, time.UTC)
		config := map[string]interface{}{}
		occurrences := calc.OccurrencesBetween(start, end, config)
		// Mon, Tue, Wed, Thu, Fri (5) + Mon, Tue, Wed, Thu, Fri (5) = 10 weekdays (excluding weekends)
		// Actually: Jan 6-10 (5 days) + Jan 13 (1 day) = 6 weekdays
		// Let me recalculate: Mon 6, Tue 7, Wed 8, Thu 9, Fri 10, Mon 13 = 6
		if len(occurrences) != 6 {
			t.Errorf("expected 6 weekday occurrences, got %d", len(occurrences))
		}
	})
}

// TestYearlyCalculator tests yearly recurrence
func TestYearlyCalculator(t *testing.T) {
	calc := &YearlyCalculator{}
	start := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)

	t.Run("next year", func(t *testing.T) {
		config := map[string]interface{}{}
		next := calc.NextOccurrence(start, config)
		expected := time.Date(2025, 3, 15, 12, 0, 0, 0, time.UTC)
		if !next.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, *next)
		}
	})
}

// TestQuarterlyCalculator tests quarterly recurrence
func TestQuarterlyCalculator(t *testing.T) {
	calc := &QuarterlyCalculator{}
	start := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("next quarter", func(t *testing.T) {
		config := map[string]interface{}{}
		next := calc.NextOccurrence(start, config)
		expected := time.Date(2025, 4, 1, 12, 0, 0, 0, time.UTC) // 3 months later
		if !next.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, *next)
		}
	})
}

// TestGetCalculator tests the calculator factory function
func TestGetCalculator(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		wantNil bool
	}{
		{"DAILY", "DAILY", false},
		{"WEEKLY", "WEEKLY", false},
		{"MONTHLY", "MONTHLY", false},
		{"WEEKDAYS", "WEEKDAYS", false},
		{"INVALID", "INVALID", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc := GetCalculator(core.RecurrencePattern(tt.pattern))
			if tt.wantNil && calc != nil {
				t.Errorf("expected nil for invalid pattern, got %T", calc)
			}
			if !tt.wantNil && calc == nil {
				t.Errorf("expected calculator for pattern %s, got nil", tt.pattern)
			}
		})
	}
}
