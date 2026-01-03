package recurring

import (
	"context"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateTasksForTemplateWithExceptions_FiltersExceptions(t *testing.T) {
	template := &domain.RecurringTemplate{
		ID:                "template-123",
		ListID:            "list-123",
		Title:             "Daily Task",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  map[string]any{"interval": 1},
	}

	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)

	// Create exceptions for Jan 2 and Jan 4
	exceptions := []*domain.RecurringTemplateException{
		{
			TemplateID:    template.ID,
			OccursAt:      time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC),
			ExceptionType: domain.ExceptionTypeDeleted,
		},
		{
			TemplateID:    template.ID,
			OccursAt:      time.Date(2026, 1, 4, 10, 0, 0, 0, time.UTC),
			ExceptionType: domain.ExceptionTypeEdited,
		},
	}

	generator := NewDomainGenerator()
	tasks, err := generator.GenerateTasksForTemplateWithExceptions(
		context.Background(),
		template,
		start,
		end,
		exceptions,
	)

	require.NoError(t, err)

	// Should generate Jan 1, 3, 5 (skip Jan 2, 4)
	assert.Equal(t, 3, len(tasks))

	expectedDates := []time.Time{
		time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 3, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC),
	}

	for i, task := range tasks {
		assert.Equal(t, expectedDates[i], *task.OccursAt, "Task %d should occur at %s", i, expectedDates[i])
	}
}

func TestGenerateTasksForTemplateWithExceptions_NoExceptions(t *testing.T) {
	template := &domain.RecurringTemplate{
		ID:                "template-123",
		ListID:            "list-123",
		Title:             "Daily Task",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  map[string]any{"interval": 1},
	}

	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 3, 10, 0, 0, 0, time.UTC)

	generator := NewDomainGenerator()
	tasks, err := generator.GenerateTasksForTemplateWithExceptions(
		context.Background(),
		template,
		start,
		end,
		nil, // No exceptions
	)

	require.NoError(t, err)

	// Should generate all 3 days
	assert.Equal(t, 3, len(tasks))
}
