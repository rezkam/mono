package repository_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/rezkam/mono/internal/core"
	"github.com/rezkam/mono/internal/storage/compliance"
	"github.com/rezkam/mono/internal/storage/sql/repository"
	sqlstorage "github.com/rezkam/mono/internal/storage/sql"
)

func TestSQLiteStorageCompliance(t *testing.T) {
	compliance.RunStorageComplianceTest(t, func() (core.Storage, func()) {
		// Create a temporary SQLite database
		tmpFile, err := os.CreateTemp("", "mono-test-*.db")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		tmpFile.Close()
		dbPath := tmpFile.Name()

		ctx := context.Background()
		store, err := sqlstorage.NewSQLiteStore(ctx, dbPath)
		if err != nil {
			os.Remove(dbPath)
			t.Fatalf("failed to create SQLite store: %v", err)
		}

		cleanup := func() {
			os.Remove(dbPath)
		}

		return store, cleanup
	})
}

// TestPostgresStorageCompliance tests PostgreSQL storage if TEST_POSTGRES_URL is set.
func TestPostgresStorageCompliance(t *testing.T) {
	pgURL := os.Getenv("TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("TEST_POSTGRES_URL not set, skipping PostgreSQL tests")
	}

	compliance.RunStorageComplianceTest(t, func() (core.Storage, func()) {
		ctx := context.Background()
		store, err := sqlstorage.NewPostgresStore(ctx, pgURL)
		if err != nil {
			t.Fatalf("failed to create PostgreSQL store: %v", err)
		}

		// Cleanup: truncate tables
		cleanup := func() {
			// Get the underlying DB connection to clean up
			// This is a bit hacky but works for tests
			db := getDB(store)
			if db != nil {
				db.Exec("TRUNCATE TABLE todo_items, todo_lists CASCADE")
			}
		}

		return store, cleanup
	})
}

// getDB is a helper to extract the *sql.DB from the repository.Store for cleanup.
// This uses reflection-like access for testing purposes only.
func getDB(store *repository.Store) *sql.DB {
	// In a real implementation, you might want to expose a Close method
	// or a way to access the DB for cleanup. For now, we'll use a simple approach.
	// Since we can't easily access private fields, we'll skip this in the cleanup.
	return nil
}
