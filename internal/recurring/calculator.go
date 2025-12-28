package recurring

import (
	"time"

	"github.com/rezkam/mono/internal/domain"
)

// PatternCalculator calculates the next occurrence date for a given recurrence pattern.
type PatternCalculator interface {
	// NextOccurrence returns the next occurrence date after the given date.
	// Returns nil if there is no next occurrence.
	NextOccurrence(after time.Time, config map[string]any) *time.Time

	// OccurrencesBetween returns all occurrence dates within the given range.
	OccurrencesBetween(start, end time.Time, config map[string]any) []time.Time
}

// GetCalculator returns the appropriate calculator for the given pattern.
func GetCalculator(pattern domain.RecurrencePattern) PatternCalculator {
	switch pattern {
	case domain.RecurrenceDaily:
		return &DailyCalculator{}
	case domain.RecurrenceWeekly:
		return &WeeklyCalculator{}
	case domain.RecurrenceBiweekly:
		return &BiweeklyCalculator{}
	case domain.RecurrenceMonthly:
		return &MonthlyCalculator{}
	case domain.RecurrenceYearly:
		return &YearlyCalculator{}
	case domain.RecurrenceQuarterly:
		return &QuarterlyCalculator{}
	case domain.RecurrenceWeekdays:
		return &WeekdaysCalculator{}
	default:
		return nil
	}
}
