package recurring

import (
	"time"
)

// DailyCalculator generates daily recurrences.
type DailyCalculator struct{}

func (c *DailyCalculator) NextOccurrence(after time.Time, config map[string]interface{}) *time.Time {
	interval := 1
	if v, ok := config["interval"].(float64); ok {
		interval = int(v)
	}

	next := after.AddDate(0, 0, interval)
	return &next
}

func (c *DailyCalculator) OccurrencesBetween(start, end time.Time, config map[string]interface{}) []time.Time {
	interval := 1
	if v, ok := config["interval"].(float64); ok {
		interval = int(v)
	}

	var occurrences []time.Time
	current := start

	for !current.After(end) {
		occurrences = append(occurrences, current)
		current = current.AddDate(0, 0, interval)
	}

	return occurrences
}

// WeeklyCalculator generates weekly recurrences.
type WeeklyCalculator struct{}

func (c *WeeklyCalculator) NextOccurrence(after time.Time, config map[string]interface{}) *time.Time {
	interval := 1
	if v, ok := config["interval"].(float64); ok {
		interval = int(v)
	}

	next := after.AddDate(0, 0, 7*interval)
	return &next
}

func (c *WeeklyCalculator) OccurrencesBetween(start, end time.Time, config map[string]interface{}) []time.Time {
	interval := 1
	if v, ok := config["interval"].(float64); ok {
		interval = int(v)
	}

	var occurrences []time.Time
	current := start

	for !current.After(end) {
		occurrences = append(occurrences, current)
		current = current.AddDate(0, 0, 7*interval)
	}

	return occurrences
}

// BiweeklyCalculator generates biweekly (every 2 weeks) recurrences.
type BiweeklyCalculator struct{}

func (c *BiweeklyCalculator) NextOccurrence(after time.Time, config map[string]interface{}) *time.Time {
	next := after.AddDate(0, 0, 14)
	return &next
}

func (c *BiweeklyCalculator) OccurrencesBetween(start, end time.Time, config map[string]interface{}) []time.Time {
	var occurrences []time.Time
	current := start

	for !current.After(end) {
		occurrences = append(occurrences, current)
		current = current.AddDate(0, 0, 14)
	}

	return occurrences
}

// MonthlyCalculator generates monthly recurrences.
type MonthlyCalculator struct{}

func (c *MonthlyCalculator) NextOccurrence(after time.Time, config map[string]interface{}) *time.Time {
	interval := 1
	if v, ok := config["interval"].(float64); ok {
		interval = int(v)
	}

	next := after.AddDate(0, interval, 0)
	return &next
}

func (c *MonthlyCalculator) OccurrencesBetween(start, end time.Time, config map[string]interface{}) []time.Time {
	interval := 1
	if v, ok := config["interval"].(float64); ok {
		interval = int(v)
	}

	var occurrences []time.Time
	current := start

	for !current.After(end) {
		occurrences = append(occurrences, current)
		current = current.AddDate(0, interval, 0)
	}

	return occurrences
}

// YearlyCalculator generates yearly recurrences.
type YearlyCalculator struct{}

func (c *YearlyCalculator) NextOccurrence(after time.Time, config map[string]interface{}) *time.Time {
	next := after.AddDate(1, 0, 0)
	return &next
}

func (c *YearlyCalculator) OccurrencesBetween(start, end time.Time, config map[string]interface{}) []time.Time {
	var occurrences []time.Time
	current := start

	for !current.After(end) {
		occurrences = append(occurrences, current)
		current = current.AddDate(1, 0, 0)
	}

	return occurrences
}

// QuarterlyCalculator generates quarterly recurrences.
type QuarterlyCalculator struct{}

func (c *QuarterlyCalculator) NextOccurrence(after time.Time, config map[string]interface{}) *time.Time {
	next := after.AddDate(0, 3, 0)
	return &next
}

func (c *QuarterlyCalculator) OccurrencesBetween(start, end time.Time, config map[string]interface{}) []time.Time {
	var occurrences []time.Time
	current := start

	for !current.After(end) {
		occurrences = append(occurrences, current)
		current = current.AddDate(0, 3, 0)
	}

	return occurrences
}

// WeekdaysCalculator generates recurrences on weekdays only (Mon-Fri).
type WeekdaysCalculator struct{}

func (c *WeekdaysCalculator) NextOccurrence(after time.Time, config map[string]interface{}) *time.Time {
	next := after.AddDate(0, 0, 1)

	// Skip weekends
	for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
		next = next.AddDate(0, 0, 1)
	}

	return &next
}

func (c *WeekdaysCalculator) OccurrencesBetween(start, end time.Time, config map[string]interface{}) []time.Time {
	var occurrences []time.Time
	current := start

	for !current.After(end) {
		if current.Weekday() != time.Saturday && current.Weekday() != time.Sunday {
			occurrences = append(occurrences, current)
		}
		current = current.AddDate(0, 0, 1)
	}

	return occurrences
}
