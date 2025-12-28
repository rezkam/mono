package recurring

import (
	"testing"
	"time"
)

// TestInfiniteLoopReproduction attempts to reproduce the infinite loop bug
// when interval is <= 0.
func TestInfiniteLoopReproduction(t *testing.T) {
	// Set a timeout for this test to catch the infinite loop
	done := make(chan bool)

	go func() {
		calc := &DailyCalculator{}
		start := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 0, 5)

		// Case 1: Interval is 0 (simulating missing/invalid config treated as 0 if logic was flawed,
		// though currently it defaults to 1 if missing. But if we explicitly pass 0...)
		// The current code does:
		// interval := 1
		// if v, ok := config["interval"].(float64); ok {
		//     interval = int(v)
		// }
		// So if we pass 0.0, interval becomes 0.

		config := map[string]any{"interval": 0.0}

		// This should trigger the infinite loop in the buggy version
		_ = calc.OccurrencesBetween(start, end, config)

		done <- true
	}()

	select {
	case <-done:
		// Test finished, meaning no infinite loop (or it finished quickly)
		// If the bug exists, we shouldn't reach here if the loop is truly infinite
		// But since we want to verify the bug exists, we expect this to TIMEOUT.
		// However, for the purpose of "verifying the issue exists", if I run this and it times out,
		// that confirms the bug.
		// To make this test useful for *verification* (passing when fixed),
		// it should pass. To demonstrate failure, it should timeout.

		// I will write the test to EXPECT success (no infinite loop).
		// When I run it now, it should FAIL (timeout).
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out! Infinite loop detected in DailyCalculator.OccurrencesBetween with interval=0")
	}
}
