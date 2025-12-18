package recurring

import (
	"time"

	"github.com/rezkam/mono/internal/core"
)

// PatternCalculator calculates the next occurrence date for a given recurrence pattern.
type PatternCalculator interface {
	// NextOccurrence returns the next occurrence date after the given date.
	// Returns nil if there is no next occurrence.
	NextOccurrence(after time.Time, config map[string]interface{}) *time.Time

	// OccurrencesBetween returns all occurrence dates within the given range.
	OccurrencesBetween(start, end time.Time, config map[string]interface{}) []time.Time
}

// GetCalculator returns the appropriate calculator for the given pattern.
func GetCalculator(pattern core.RecurrencePattern) PatternCalculator {
	switch pattern {
	case core.RecurrenceDaily:
		return &DailyCalculator{}
	case core.RecurrenceWeekly:
		return &WeeklyCalculator{}
	case core.RecurrenceBiweekly:
		return &BiweeklyCalculator{}
	case core.RecurrenceMonthly:
		return &MonthlyCalculator{}
	case core.RecurrenceYearly:
		return &YearlyCalculator{}
	case core.RecurrenceQuarterly:
		return &QuarterlyCalculator{}
	case core.RecurrenceWeekdays:
		return &WeekdaysCalculator{}
	default:
		return nil
	}
}
