package sql

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // SQLite driver

	"github.com/rezkam/mono/internal/storage/sql/repository"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// DBConfig holds database connection configuration.
type DBConfig struct {
	Driver            string        // "pgx" for PostgreSQL, "sqlite" for SQLite
	DSN               string        // Data Source Name / connection string
	MaxOpenConns      int           // Maximum open connections (default: 25)
	MaxIdleConns      int           // Maximum idle connections (default: 5)
	ConnMaxLifetime   time.Duration // Connection max lifetime (default: 5min)
	ConnMaxIdleTime   time.Duration // Connection max idle time (default: 1min)
}

// NewStore creates a new SQL store with the given configuration.
// It also runs migrations automatically.
func NewStore(ctx context.Context, cfg DBConfig) (*repository.Store, error) {
	// Open database connection
	db, err := sql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool with defaults if not set
	maxOpenConns := cfg.MaxOpenConns
	if maxOpenConns <= 0 {
		maxOpenConns = 25
	}
	maxIdleConns := cfg.MaxIdleConns
	if maxIdleConns <= 0 {
		maxIdleConns = 5
	}
	connMaxLifetime := cfg.ConnMaxLifetime
	if connMaxLifetime <= 0 {
		connMaxLifetime = 5 * time.Minute
	}
	connMaxIdleTime := cfg.ConnMaxIdleTime
	if connMaxIdleTime <= 0 {
		connMaxIdleTime = 1 * time.Minute
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)
	db.SetConnMaxIdleTime(connMaxIdleTime)

	// Verify connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Run migrations
	if err := runMigrations(db, cfg.Driver); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return repository.NewStore(db), nil
}

// runMigrations runs database migrations using goose with embedded files.
func runMigrations(db *sql.DB, driver string) error {
	// Set the dialect based on the driver
	dialect := "sqlite3"
	if driver == "pgx" {
		dialect = "postgres"
	}

	if err := goose.SetDialect(dialect); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	// Set the base FS for migrations
	goose.SetBaseFS(embedMigrations)

	// Run migrations from embedded directory
	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}

// NewPostgresStore creates a PostgreSQL-backed store with default connection pool settings.
func NewPostgresStore(ctx context.Context, connString string) (*repository.Store, error) {
	return NewStore(ctx, DBConfig{
		Driver: "pgx",
		DSN:    connString,
	})
}

// NewPostgresStoreWithConfig creates a PostgreSQL-backed store with custom connection pool settings.
func NewPostgresStoreWithConfig(ctx context.Context, connString string, poolConfig DBConfig) (*repository.Store, error) {
	poolConfig.Driver = "pgx"
	poolConfig.DSN = connString
	return NewStore(ctx, poolConfig)
}

// NewSQLiteStore creates a SQLite-backed store with default connection pool settings.
func NewSQLiteStore(ctx context.Context, dbPath string) (*repository.Store, error) {
	// SQLite DSN with recommended pragmas for better performance and reliability
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on", dbPath)
	return NewStore(ctx, DBConfig{
		Driver: "sqlite",
		DSN:    dsn,
	})
}

// NewSQLiteStoreWithConfig creates a SQLite-backed store with custom connection pool settings.
func NewSQLiteStoreWithConfig(ctx context.Context, dbPath string, poolConfig DBConfig) (*repository.Store, error) {
	// SQLite DSN with recommended pragmas for better performance and reliability
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on", dbPath)
	poolConfig.Driver = "sqlite"
	poolConfig.DSN = dsn
	return NewStore(ctx, poolConfig)
}
