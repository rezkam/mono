package integration

import (
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/rezkam/mono/internal/config"
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
		db.Exec("TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
		db.Close()
	}

	return db, cleanup
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
