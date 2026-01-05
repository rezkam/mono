package postgres

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
)

func TestIntervalToDuration(t *testing.T) {
	t.Run("microseconds only", func(t *testing.T) {
		// 2 hours in microseconds
		interval := pgtype.Interval{
			Microseconds: 2 * 60 * 60 * 1_000_000, // 2 hours
			Valid:        true,
		}

		result := intervalToDuration(interval)

		assert.Equal(t, 2*time.Hour, result)
	})

	t.Run("days only", func(t *testing.T) {
		// 3 days stored in Days field (PostgreSQL: INTERVAL '3 days')
		interval := pgtype.Interval{
			Days:  3,
			Valid: true,
		}

		result := intervalToDuration(interval)

		// Should convert 3 days to 72 hours
		assert.Equal(t, 3*24*time.Hour, result)
	})

	t.Run("months only", func(t *testing.T) {
		// 2 months stored in Months field (PostgreSQL: INTERVAL '2 months')
		interval := pgtype.Interval{
			Months: 2,
			Valid:  true,
		}

		result := intervalToDuration(interval)

		// PostgreSQL uses 30 days/month for interval arithmetic
		// 2 months = 60 days = 1440 hours
		assert.Equal(t, 2*30*24*time.Hour, result)
	})

	t.Run("combined months days and microseconds", func(t *testing.T) {
		// 1 month, 2 days, 3 hours (PostgreSQL: INTERVAL '1 month 2 days 3 hours')
		interval := pgtype.Interval{
			Months:       1,
			Days:         2,
			Microseconds: 3 * 60 * 60 * 1_000_000, // 3 hours
			Valid:        true,
		}

		result := intervalToDuration(interval)

		// 1 month (30 days) + 2 days + 3 hours = 32 days + 3 hours = 771 hours
		expected := (30*24 + 2*24 + 3) * time.Hour
		assert.Equal(t, expected, result)
	})

	t.Run("zero interval", func(t *testing.T) {
		interval := pgtype.Interval{
			Months:       0,
			Days:         0,
			Microseconds: 0,
			Valid:        true,
		}

		result := intervalToDuration(interval)

		assert.Equal(t, time.Duration(0), result)
	})

	t.Run("one day in days field", func(t *testing.T) {
		// This is the most common failure case: INTERVAL '1 day'
		interval := pgtype.Interval{
			Days:  1,
			Valid: true,
		}

		result := intervalToDuration(interval)

		assert.Equal(t, 24*time.Hour, result)
	})

	t.Run("one month in months field", func(t *testing.T) {
		// INTERVAL '1 month'
		interval := pgtype.Interval{
			Months: 1,
			Valid:  true,
		}

		result := intervalToDuration(interval)

		// 30 days = 720 hours
		assert.Equal(t, 30*24*time.Hour, result)
	})

	t.Run("large interval one year", func(t *testing.T) {
		// INTERVAL '1 year' = 12 months
		interval := pgtype.Interval{
			Months: 12,
			Valid:  true,
		}

		result := intervalToDuration(interval)

		// 12 months * 30 days = 360 days
		assert.Equal(t, 12*30*24*time.Hour, result)
	})

	t.Run("fractional hours via microseconds", func(t *testing.T) {
		// 1.5 hours = 90 minutes
		interval := pgtype.Interval{
			Microseconds: 90 * 60 * 1_000_000,
			Valid:        true,
		}

		result := intervalToDuration(interval)

		assert.Equal(t, 90*time.Minute, result)
	})

	t.Run("days and hours combined", func(t *testing.T) {
		// 1 day 6 hours (common for due offsets)
		interval := pgtype.Interval{
			Days:         1,
			Microseconds: 6 * 60 * 60 * 1_000_000,
			Valid:        true,
		}

		result := intervalToDuration(interval)

		assert.Equal(t, 30*time.Hour, result) // 24 + 6 = 30 hours
	})
}

func TestDurationToInterval(t *testing.T) {
	t.Run("hours to interval", func(t *testing.T) {
		duration := 2 * time.Hour

		result := durationToInterval(duration)

		assert.True(t, result.Valid)
		assert.Equal(t, int64(2*60*60*1_000_000), result.Microseconds)
		// Go durations only use microseconds, not months/days
		assert.Equal(t, int32(0), result.Months)
		assert.Equal(t, int32(0), result.Days)
	})

	t.Run("days as duration to interval", func(t *testing.T) {
		// 3 days expressed as Go duration
		duration := 3 * 24 * time.Hour

		result := durationToInterval(duration)

		assert.True(t, result.Valid)
		// All stored in microseconds since Go duration has no day concept
		assert.Equal(t, int64(3*24*60*60*1_000_000), result.Microseconds)
		assert.Equal(t, int32(0), result.Days)
	})

	t.Run("zero duration", func(t *testing.T) {
		result := durationToInterval(0)

		assert.True(t, result.Valid)
		assert.Equal(t, int64(0), result.Microseconds)
	})
}

func TestIntervalRoundTrip(t *testing.T) {
	t.Run("roundtrip preserves duration", func(t *testing.T) {
		original := 48*time.Hour + 30*time.Minute + 15*time.Second

		interval := durationToInterval(original)
		result := intervalToDuration(interval)

		assert.Equal(t, original, result)
	})
}
