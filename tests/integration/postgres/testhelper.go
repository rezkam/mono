package integration

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/rezkam/mono/internal/config"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres/migrations"
	"github.com/stretchr/testify/require"
)

// SetupTestDB initializes a PostgreSQL test database with migrations.
// Returns the database connection and a cleanup function.
func SetupTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	cfg, err := config.LoadTestConfig()
	if err != nil {
		t.Skipf("Failed to load test config: %v (set MONO_STORAGE_DSN to run integration tests)", err)
	}

	db, err := sql.Open("pgx", cfg.StorageDSN)
	require.NoError(t, err)

	require.NoError(t, db.Ping())

	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(db, "."))

	cleanup := func() {
		_, _ = db.Exec("TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
		_ = db.Close()
	}

	return db, cleanup
}

// SetupTestStore initializes a PostgreSQL store with automatic cleanup.
// Returns the store and context. Cleanup runs automatically via t.Cleanup().
func SetupTestStore(t *testing.T) (*postgres.Store, context.Context) {
	t.Helper()

	pgURL := GetTestStorageDSN(t)
	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)

	// Cleanup at END - truncate tables and close connection
	t.Cleanup(func() {
		db, err := sql.Open("pgx", pgURL)
		if err == nil {
			_, _ = db.Exec("TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
			_ = db.Close()
		}
		_ = store.Close()
	})

	return store, ctx
}

// GetTestStorageDSN returns the storage DSN for tests.
func GetTestStorageDSN(t *testing.T) string {
	t.Helper()

	cfg, err := config.LoadTestConfig()
	if err != nil {
		t.Skipf("Failed to load test config: %v (set MONO_STORAGE_DSN to run integration tests)", err)
	}

	return cfg.StorageDSN
}

// ItemToUpdateParams converts a TodoItem to UpdateItemParams for all fields.
// This is a test helper to simplify migration from old UpdateItem signature.
// Does NOT pass etag - tests that need optimistic locking should pass it explicitly.
func ItemToUpdateParams(listID string, item *domain.TodoItem) domain.UpdateItemParams {
	return domain.UpdateItemParams{
		ItemID:            item.ID,
		ListID:            listID,
		Etag:              nil, // Tests that need optimistic locking should pass etag explicitly
		UpdateMask:        []string{"title", "status", "priority", "due_time", "tags", "timezone", "estimated_duration", "actual_duration"},
		Title:             &item.Title,
		Status:            &item.Status,
		Priority:          item.Priority,
		DueTime:           item.DueTime,
		Tags:              &item.Tags,
		Timezone:          item.Timezone,
		EstimatedDuration: item.EstimatedDuration,
		ActualDuration:    item.ActualDuration,
	}
}

// ItemToUpdateParamsWithEtag is like ItemToUpdateParams but includes the etag for optimistic locking.
func ItemToUpdateParamsWithEtag(listID string, item *domain.TodoItem) domain.UpdateItemParams {
	etag := item.Etag()
	params := ItemToUpdateParams(listID, item)
	params.Etag = &etag
	return params
}

// ListToUpdateParams converts a TodoList to UpdateListParams.
// This is a test helper to simplify migration from old UpdateList signature.
func ListToUpdateParams(list *domain.TodoList) domain.UpdateListParams {
	return domain.UpdateListParams{
		ListID:     list.ID,
		UpdateMask: []string{"title"},
		Title:      &list.Title,
	}
}
