package postgres_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestErrorWrappingPattern verifies that error wrapping preserves the full error chain.
// This tests the PATTERN of error wrapping, not specific repository methods.
//
// In Go 1.20+, fmt.Errorf supports multiple %w verbs, allowing us to wrap multiple errors.
// This is critical because we want BOTH:
// - Domain errors (for business logic: errors.Is(err, domain.ErrInvalidID))
// - Original errors (for debugging: seeing the actual UUID parse error details)
func TestErrorWrappingPattern(t *testing.T) {
	t.Run("INCORRECT pattern - %w with %v breaks error chain", func(t *testing.T) {
		// Simulate what currently happens in the codebase
		_, parseErr := uuid.Parse("invalid-uuid")
		require.Error(t, parseErr)

		// INCORRECT: Using %v for the second error
		wrappedErr := fmt.Errorf("%w: %v", domain.ErrInvalidID, parseErr)

		// Domain error is accessible ✓
		assert.True(t, errors.Is(wrappedErr, domain.ErrInvalidID))

		// But the original error is NOT in the chain ✗
		// It's just converted to a string
		unwrapped := errors.Unwrap(wrappedErr)
		assert.Equal(t, domain.ErrInvalidID, unwrapped,
			"only domain error is wrapped, original error is lost")
	})

	t.Run("CORRECT pattern - %w with %w preserves error chain", func(t *testing.T) {
		// Simulate what SHOULD happen
		_, parseErr := uuid.Parse("invalid-uuid")
		require.Error(t, parseErr)

		// CORRECT: Using %w for both errors (Go 1.20+)
		wrappedErr := fmt.Errorf("%w: %w", domain.ErrInvalidID, parseErr)

		// Domain error is accessible ✓
		assert.True(t, errors.Is(wrappedErr, domain.ErrInvalidID))

		// AND the original error is also in the chain ✓
		assert.True(t, errors.Is(wrappedErr, parseErr),
			"both errors should be in the chain for debugging")
	})

	t.Run("demonstrates the debugging benefit", func(t *testing.T) {
		_, parseErr := uuid.Parse("totally-invalid")
		require.Error(t, parseErr)

		// With %v (current/broken pattern)
		brokenErr := fmt.Errorf("%w: %v", domain.ErrInvalidID, parseErr)

		// With %w (correct pattern)
		correctErr := fmt.Errorf("%w: %w", domain.ErrInvalidID, parseErr)

		t.Logf("Broken error message: %v", brokenErr)
		t.Logf("Correct error message: %v", correctErr)

		// Both show the error message, BUT:
		// Only the correct pattern lets you unwrap to the original error
		// This matters for structured logging, error type checking, etc.
	})
}
