package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/rezkam/mono/internal/ptr"
	"github.com/rezkam/mono/internal/recurring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTimezone_FloatingTime verifies that when Timezone is nil, task times
// stay constant (9am stays 9am) and are NOT anchored to any specific timezone.
func TestTimezone_FloatingTime(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	generator := recurring.NewDomainGenerator()
	service := todo.NewService(store, generator, todo.Config{})

	// Create list
	list, err := service.CreateList(ctx, "Floating Time Test")
	require.NoError(t, err)

	// Create item with nil Timezone (floating time)
	dueTime := time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC) // 9:00 AM
	item := &domain.TodoItem{
		Title:    "Wake up at 9am",
		Status:   domain.TaskStatusTodo,
		DueAt:    &dueTime,
		Timezone: nil, // Floating time - 9am stays 9am everywhere
	}

	created, err := service.CreateItem(ctx, list.ID, item)
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := service.GetItem(ctx, created.ID)
	require.NoError(t, err)

	// Timezone field should be nil (floating time mode)
	assert.Nil(t, retrieved.Timezone, "Timezone should be nil for floating time")

	// DueAt should be stored as provided (9am)
	require.NotNil(t, retrieved.DueAt)
	assert.Equal(t, 9, retrieved.DueAt.Hour(), "Hour should be 9 for floating time")
	assert.Equal(t, 0, retrieved.DueAt.Minute(), "Minute should be 0")

	// IMPORTANT: Floating time means the time value is location-independent
	// 9am UTC in storage represents "9am local time wherever you are"
	// The application layer should interpret this as floating when Timezone is nil
}

// TestTimezone_FixedTimezone verifies that when Timezone is set, task times
// are anchored to a specific timezone and represent absolute UTC moments.
func TestTimezone_FixedTimezone(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	generator := recurring.NewDomainGenerator()
	service := todo.NewService(store, generator, todo.Config{})

	// Create list
	list, err := service.CreateList(ctx, "Fixed Timezone Test")
	require.NoError(t, err)

	testCases := []struct {
		name     string
		timezone string
	}{
		{"Stockholm (UTC+1)", "Europe/Stockholm"},
		{"New York (UTC-4 in summer)", "America/New_York"},
		{"Tokyo (UTC+9)", "Asia/Tokyo"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create item with fixed timezone
			dueTime := time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC)
			item := &domain.TodoItem{
				Title:    "Meeting at 9am " + tc.name,
				Status:   domain.TaskStatusTodo,
				DueAt:    &dueTime,
				Timezone: &tc.timezone, // Fixed timezone - anchored to specific location
			}

			created, err := service.CreateItem(ctx, list.ID, item)
			require.NoError(t, err)

			// Retrieve and verify
			retrieved, err := service.GetItem(ctx, created.ID)
			require.NoError(t, err)

			// Timezone field should be preserved
			require.NotNil(t, retrieved.Timezone, "Timezone should be set for fixed timezone mode")
			assert.Equal(t, tc.timezone, *retrieved.Timezone, "Timezone should match")

			// DueAt should be stored as provided
			// Note: The application interprets DueAt based on the Timezone field
			// Storage layer always uses UTC, but the semantic meaning changes based on Timezone field
			require.NotNil(t, retrieved.DueAt)
		})
	}
}

// TestTimezone_ValidationRejectsInvalidTimezone verifies that invalid timezone
// names are rejected during item creation.
func TestTimezone_ValidationRejectsInvalidTimezone(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	generator := recurring.NewDomainGenerator()
	service := todo.NewService(store, generator, todo.Config{})

	// Create list
	list, err := service.CreateList(ctx, "Timezone Validation Test")
	require.NoError(t, err)

	invalidTimezones := []struct {
		name     string
		timezone string
	}{
		{"invalid format", "GMT+1"},
		{"non-existent", "Invalid/Timezone"},
		{"random text", "not a timezone"},
	}

	for _, tc := range invalidTimezones {
		t.Run(tc.name, func(t *testing.T) {
			dueTime := time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC)
			item := &domain.TodoItem{
				Title:    "Test Invalid Timezone",
				Status:   domain.TaskStatusTodo,
				DueAt:    &dueTime,
				Timezone: &tc.timezone,
			}

			_, err := service.CreateItem(ctx, list.ID, item)

			// Invalid timezone should be rejected
			require.Error(t, err, "Invalid timezone should be rejected: %s", tc.timezone)
			assert.Contains(t, err.Error(), "invalid timezone", "Error should mention invalid timezone")
		})
	}
}

// TestTimezone_ValidIANATimezones verifies that all common IANA timezone names
// are accepted and preserved correctly.
func TestTimezone_ValidIANATimezones(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	generator := recurring.NewDomainGenerator()
	service := todo.NewService(store, generator, todo.Config{})

	// Create list
	list, err := service.CreateList(ctx, "Valid Timezone Test")
	require.NoError(t, err)

	validTimezones := []string{
		"UTC",
		"Europe/London",
		"Europe/Stockholm",
		"Europe/Berlin",
		"America/New_York",
		"America/Los_Angeles",
		"America/Chicago",
		"Asia/Tokyo",
		"Asia/Shanghai",
		"Australia/Sydney",
		"Pacific/Auckland",
	}

	for _, tz := range validTimezones {
		t.Run(tz, func(t *testing.T) {
			dueTime := time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC)
			item := &domain.TodoItem{
				Title:    "Test " + tz,
				Status:   domain.TaskStatusTodo,
				DueAt:    &dueTime,
				Timezone: &tz,
			}

			created, err := service.CreateItem(ctx, list.ID, item)
			require.NoError(t, err, "Valid IANA timezone should be accepted: %s", tz)

			// Retrieve and verify timezone is preserved
			retrieved, err := service.GetItem(ctx, created.ID)
			require.NoError(t, err)
			require.NotNil(t, retrieved.Timezone)
			assert.Equal(t, tz, *retrieved.Timezone, "Timezone should be preserved exactly")
		})
	}
}

// TestTimezone_UpdateBetweenModes verifies that items can be updated to switch
// between floating time and fixed timezone modes.
func TestTimezone_UpdateBetweenModes(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	generator := recurring.NewDomainGenerator()
	service := todo.NewService(store, generator, todo.Config{})

	// Create list
	list, err := service.CreateList(ctx, "Timezone Update Test")
	require.NoError(t, err)

	t.Run("FloatingToFixed", func(t *testing.T) {
		// Create item with floating time (nil timezone)
		dueTime := time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC)
		item := &domain.TodoItem{
			Title:    "Convert to fixed timezone",
			Status:   domain.TaskStatusTodo,
			DueAt:    &dueTime,
			Timezone: nil, // Start with floating time
		}

		created, err := service.CreateItem(ctx, list.ID, item)
		require.NoError(t, err)
		assert.Nil(t, created.Timezone, "Should start with floating time")

		// Update to fixed timezone
		stockholm := "Europe/Stockholm"
		updated, err := service.UpdateItem(ctx, domain.UpdateItemParams{
			ItemID:     created.ID,
			ListID:     list.ID,
			UpdateMask: []string{"timezone"},
			Timezone:   &stockholm,
		})
		require.NoError(t, err)
		require.NotNil(t, updated.Timezone)
		assert.Equal(t, stockholm, *updated.Timezone, "Should now have fixed timezone")
	})

	t.Run("FixedToFloating", func(t *testing.T) {
		// Create item with fixed timezone
		stockholm := "Europe/Stockholm"
		dueTime := time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC)
		item := &domain.TodoItem{
			Title:    "Convert to floating time",
			Status:   domain.TaskStatusTodo,
			DueAt:    &dueTime,
			Timezone: &stockholm, // Start with fixed timezone
		}

		created, err := service.CreateItem(ctx, list.ID, item)
		require.NoError(t, err)
		require.NotNil(t, created.Timezone)

		// Update to floating time (nil)
		updated, err := service.UpdateItem(ctx, domain.UpdateItemParams{
			ItemID:     created.ID,
			ListID:     list.ID,
			UpdateMask: []string{"timezone"},
			Timezone:   nil, // Convert to floating time
		})
		require.NoError(t, err)
		assert.Nil(t, updated.Timezone, "Should now have floating time (nil)")
	})
}

// TestTimezone_RecurringItemsInheritTimezone verifies that recurring task
// instances inherit the timezone setting from their template.
func TestTimezone_RecurringItemsInheritTimezone(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	generator := recurring.NewDomainGenerator()
	service := todo.NewService(store, generator, todo.Config{})

	// Create list
	list, err := service.CreateList(ctx, "Recurring Timezone Test")
	require.NoError(t, err)

	// Create daily recurring template without timezone (floating time)
	template := &domain.RecurringTemplate{
		ID:                    uuid.Must(uuid.NewV7()).String(),
		ListID:                list.ID,
		Title:                 "Daily wake up",
		RecurrencePattern:     domain.RecurrenceDaily,
		RecurrenceConfig:      map[string]any{"hour": 9, "minute": 0},
		SyncHorizonDays:       7,
		GenerationHorizonDays: 30,
		IsActive:              true,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
		GeneratedThrough:      time.Now().UTC().AddDate(0, 0, -1),
	}

	err = store.AtomicRecurring(ctx, func(ops todo.RecurringOperations) error {
		_, err := ops.CreateRecurringTemplate(ctx, template)
		if err != nil {
			return err
		}

		// Generate tasks
		from := time.Now().UTC()
		until := from.AddDate(0, 0, 7)
		tasks, err := generator.GenerateTasksForTemplateWithExceptions(ctx, template, from, until, nil)
		if err != nil {
			return err
		}

		// Batch insert generated tasks
		_, err = ops.BatchInsertItemsIgnoreConflict(ctx, tasks)
		return err
	})
	require.NoError(t, err)

	// Verify generated tasks have nil timezone (inherited floating time)
	params := domain.ListTasksParams{
		ListID: ptr.To(list.ID),
		Filter: domain.ItemsFilter{},
		Limit:  10,
		Offset: 0,
	}
	result, err := service.ListItems(ctx, params)
	require.NoError(t, err)
	require.Greater(t, len(result.Items), 0, "Should have generated tasks")

	for _, task := range result.Items {
		if task.RecurringTemplateID != nil && *task.RecurringTemplateID == template.ID {
			assert.Nil(t, task.Timezone, "Recurring task should inherit nil timezone (floating time)")
		}
	}
}

// TestTimezone_DoesNotAffectOperationalTimes verifies that the Timezone field
// only affects task-related times, NOT operational times like CreatedAt/UpdatedAt.
func TestTimezone_DoesNotAffectOperationalTimes(t *testing.T) {
	_, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	generator := recurring.NewDomainGenerator()
	service := todo.NewService(store, generator, todo.Config{})

	// Create list
	list, err := service.CreateList(ctx, "Operational Time Test")
	require.NoError(t, err)

	// Create item with fixed timezone
	stockholm := "Europe/Stockholm"
	dueTime := time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC)
	item := &domain.TodoItem{
		Title:    "Stockholm meeting",
		Status:   domain.TaskStatusTodo,
		DueAt:    &dueTime,
		Timezone: &stockholm, // Fixed timezone for task times
	}

	created, err := service.CreateItem(ctx, list.ID, item)
	require.NoError(t, err)

	// CRITICAL: CreatedAt and UpdatedAt should ALWAYS be UTC regardless of Timezone field
	assert.Equal(t, time.UTC, created.CreatedAt.Location(),
		"CreatedAt must be UTC regardless of Timezone field")
	assert.Equal(t, time.UTC, created.UpdatedAt.Location(),
		"UpdatedAt must be UTC regardless of Timezone field")

	// Update the item
	newTitle := "Updated meeting"
	updated, err := service.UpdateItem(ctx, domain.UpdateItemParams{
		ItemID:     created.ID,
		ListID:     list.ID,
		UpdateMask: []string{"title"},
		Title:      &newTitle,
	})
	require.NoError(t, err)

	// Verify operational times are still UTC
	assert.Equal(t, time.UTC, updated.CreatedAt.Location(),
		"CreatedAt must remain UTC after update")
	assert.Equal(t, time.UTC, updated.UpdatedAt.Location(),
		"UpdatedAt must be UTC after update")

	// The Timezone field should not have changed
	require.NotNil(t, updated.Timezone)
	assert.Equal(t, stockholm, *updated.Timezone,
		"Timezone field should be preserved")
}
