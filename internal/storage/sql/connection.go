package sql

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
	"github.com/pressly/goose/v3"

	"github.com/rezkam/mono/internal/storage/sql/repository"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// DBConfig holds PostgreSQL database connection configuration.
type DBConfig struct {
	DSN             string        // PostgreSQL connection string
	MaxOpenConns    int           // Maximum open connections (default: 25)
	MaxIdleConns    int           // Maximum idle connections (default: 5)
	ConnMaxLifetime time.Duration // Connection max lifetime (default: 5min)
	ConnMaxIdleTime time.Duration // Connection max idle time (default: 1min)
}

// NewStore creates a new PostgreSQL store with the given configuration.
// It also runs migrations automatically.
func NewStore(ctx context.Context, cfg DBConfig) (*repository.Store, error) {
	// Open PostgreSQL connection
	db, err := sql.Open("pgx", cfg.DSN)
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
	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return repository.NewStore(db), nil
}

// runMigrations runs PostgreSQL database migrations using goose with embedded files.
func runMigrations(db *sql.DB) error {
	if err := goose.SetDialect("postgres"); err != nil {
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

// NewPostgresStore creates a PostgreSQL store with default connection pool settings.
func NewPostgresStore(ctx context.Context, connString string) (*repository.Store, error) {
	return NewStore(ctx, DBConfig{
		DSN: connString,
	})
}

// NewPostgresStoreWithConfig creates a PostgreSQL store with custom connection pool settings.
func NewPostgresStoreWithConfig(ctx context.Context, connString string, poolConfig DBConfig) (*repository.Store, error) {
	poolConfig.DSN = connString
	return NewStore(ctx, poolConfig)
}
