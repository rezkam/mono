package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rezkam/mono/internal/domain"
)

func TestMapItemToDTO_DurationFieldsUseISO8601Format(t *testing.T) {
	// Create a domain item with duration fields
	estimatedDuration := 1*time.Hour + 30*time.Minute
	actualDuration := 2*time.Hour + 15*time.Minute

	item := &domain.TodoItem{
		ID:                "test-id-123",
		Title:             "Test Task",
		Status:            domain.TaskStatusTodo,
		EstimatedDuration: &estimatedDuration,
		ActualDuration:    &actualDuration,
	}

	// Map to DTO
	dto := MapItemToDTO(item)

	// Verify EstimatedDuration is in ISO 8601 format (PT1H30M), not Go format (1h30m0s)
	require.NotNil(t, dto.EstimatedDuration)
	assert.Equal(t, "PT1H30M", *dto.EstimatedDuration, "EstimatedDuration should be ISO 8601 format")

	// Verify ActualDuration is in ISO 8601 format (PT2H15M), not Go format (2h15m0s)
	require.NotNil(t, dto.ActualDuration)
	assert.Equal(t, "PT2H15M", *dto.ActualDuration, "ActualDuration should be ISO 8601 format")
}

func TestMapTemplateToDTO_DurationFieldsUseISO8601Format(t *testing.T) {
	// Create a domain template with duration fields
	estimatedDuration := 45 * time.Minute
	dueOffset := 2 * time.Hour

	template := &domain.RecurringTemplate{
		ID:                "template-id-123",
		ListID:            "list-id-456",
		Title:             "Test Template",
		RecurrencePattern: domain.RecurrenceDaily,
		IsActive:          true,
		EstimatedDuration: &estimatedDuration,
		DueOffset:         &dueOffset,
	}

	// Map to DTO
	dto := MapTemplateToDTO(template)

	// Verify EstimatedDuration is in ISO 8601 format (PT45M), not Go format (45m0s)
	require.NotNil(t, dto.EstimatedDuration)
	assert.Equal(t, "PT45M", *dto.EstimatedDuration, "EstimatedDuration should be ISO 8601 format")

	// Verify DueOffset is in ISO 8601 format (PT2H), not Go format (2h0m0s)
	require.NotNil(t, dto.DueOffset)
	assert.Equal(t, "PT2H", *dto.DueOffset, "DueOffset should be ISO 8601 format")
}

func TestMapItemToDTO_NilDurationFields(t *testing.T) {
	// Create item with nil duration fields
	item := &domain.TodoItem{
		ID:     "test-id-123",
		Title:  "Test Task",
		Status: domain.TaskStatusTodo,
		// EstimatedDuration and ActualDuration are nil
	}

	// Map to DTO
	dto := MapItemToDTO(item)

	// Verify nil durations remain nil
	assert.Nil(t, dto.EstimatedDuration, "Nil EstimatedDuration should remain nil")
	assert.Nil(t, dto.ActualDuration, "Nil ActualDuration should remain nil")
}

func TestMapItemToDTO_ZeroDuration(t *testing.T) {
	// Create item with zero duration
	zeroDuration := time.Duration(0)

	item := &domain.TodoItem{
		ID:                "test-id-123",
		Title:             "Test Task",
		Status:            domain.TaskStatusTodo,
		EstimatedDuration: &zeroDuration,
	}

	// Map to DTO
	dto := MapItemToDTO(item)

	// Verify zero duration is formatted as PT0S (ISO 8601), not 0s (Go format)
	require.NotNil(t, dto.EstimatedDuration)
	assert.Equal(t, "PT0S", *dto.EstimatedDuration, "Zero duration should be ISO 8601 format")
}

func TestMapItemToDTO_StartsAtAndDueOffsetAreMapped(t *testing.T) {
	// Create item with StartsAt and DueOffset populated
	startsAt := time.Date(2025, 3, 15, 9, 0, 0, 0, time.UTC)
	dueOffset := 2 * time.Hour

	item := &domain.TodoItem{
		ID:        "test-id-123",
		Title:     "Scheduled Task",
		Status:    domain.TaskStatusTodo,
		StartsAt:  &startsAt,
		DueOffset: &dueOffset,
	}

	// Map to DTO
	dto := MapItemToDTO(item)

	// Verify StartsAt is mapped (not nil)
	require.NotNil(t, dto.StartsAt, "StartsAt should be mapped from domain to DTO")
	assert.Equal(t, startsAt.Format(time.DateOnly), dto.StartsAt.String(), "StartsAt date should match")

	// Verify DueOffset is mapped in ISO 8601 format
	require.NotNil(t, dto.DueOffset, "DueOffset should be mapped from domain to DTO")
	assert.Equal(t, "PT2H", *dto.DueOffset, "DueOffset should be ISO 8601 format")
}

func TestMapItemToDTO_NilStarsAtAndDueOffset(t *testing.T) {
	// Create item without StartsAt and DueOffset
	item := &domain.TodoItem{
		ID:     "test-id-123",
		Title:  "Regular Task",
		Status: domain.TaskStatusTodo,
		// StartsAt and DueOffset are nil
	}

	// Map to DTO
	dto := MapItemToDTO(item)

	// Verify nil values remain nil
	assert.Nil(t, dto.StartsAt, "Nil StartsAt should remain nil")
	assert.Nil(t, dto.DueOffset, "Nil DueOffset should remain nil")
}
