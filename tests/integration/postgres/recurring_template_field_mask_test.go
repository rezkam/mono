package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/rezkam/mono/internal/ptr"
	"github.com/rezkam/mono/internal/recurring"
	"github.com/stretchr/testify/require"
)

// TestUpdateRecurringTemplate_RecurrenceConfigNilInMask verifies that when
// recurrence_config is included in the update mask with a nil value, the
// system returns a validation error instead of attempting a database update
// that would violate the NOT NULL constraint.
//
// Expected behavior: Validation error when required field is nil in mask
func TestUpdateRecurringTemplate_RecurrenceConfigNilInMask(t *testing.T) {
	pgURL := GetTestStorageDSN(t)
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	generator := recurring.NewDomainGenerator()
	todoService := todo.NewService(store, generator, todo.Config{})

	// Create a list
	listID, err := uuid.NewV7()
	require.NoError(t, err)
	list := &domain.TodoList{
		ID:    listID.String(),
		Title: "Test List",
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a recurring template with valid recurrence_config
	template := &domain.RecurringTemplate{
		ListID:                list.ID,
		Title:                 "Daily Task",
		RecurrencePattern:     domain.RecurrenceDaily,
		RecurrenceConfig:      map[string]any{"interval": float64(1)},
		SyncHorizonDays:       14,
		GenerationHorizonDays: 365,
		IsActive:              true,
	}

	created, err := todoService.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)

	// Attempt to update with recurrence_config in mask but nil value
	updateParams := domain.UpdateRecurringTemplateParams{
		TemplateID: created.ID,
		ListID:     list.ID,
		UpdateMask: []string{"title", "recurrence_config"},
		Title:      ptr.To("Updated Title"),
		// RecurrenceConfig is nil (zero value for map)
		RecurrenceConfig: nil,
	}

	_, err = todoService.UpdateRecurringTemplate(ctx, updateParams)

	// Should return validation error, not database constraint violation
	require.ErrorIs(t, err, domain.ErrRecurrenceConfigRequired)
}
