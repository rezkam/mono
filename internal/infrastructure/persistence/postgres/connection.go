package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver for migrations
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// DBConfig holds PostgreSQL database connection configuration.
type DBConfig struct {
	DSN             string        // PostgreSQL connection string
	MaxOpenConns    int           // Maximum open connections (0 = auto-scale based on available CPUs)
	MaxIdleConns    int           // Maximum idle connections (0 = auto-scale based on available CPUs)
	ConnMaxLifetime time.Duration // Connection max lifetime (0 = default: 5min)
	ConnMaxIdleTime time.Duration // Connection max idle time (0 = default: 1min)
}

// NewStoreWithConfig creates a new PostgreSQL store with the given configuration.
// It also runs migrations automatically.
func NewStoreWithConfig(ctx context.Context, cfg DBConfig) (*Store, error) {
	// Run migrations first using database/sql (goose requires it)
	if err := runMigrationsWithDSN(ctx, cfg.DSN); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Parse connection string and configure pool
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	maxConns := int32(cfg.MaxOpenConns)
	if maxConns <= 0 {
		cpus := runtime.GOMAXPROCS(0)
		maxConns = int32(cpus * 4)
	}
	minConns := int32(cfg.MaxIdleConns)
	if minConns <= 0 {
		cpus := runtime.GOMAXPROCS(0)
		minConns = int32(cpus)
	}
	connMaxLifetime := cfg.ConnMaxLifetime
	if connMaxLifetime <= 0 {
		connMaxLifetime = 5 * time.Minute
	}
	connMaxIdleTime := cfg.ConnMaxIdleTime
	if connMaxIdleTime <= 0 {
		connMaxIdleTime = 1 * time.Minute
	}

	poolConfig.MaxConns = maxConns
	poolConfig.MinConns = minConns
	poolConfig.MaxConnLifetime = connMaxLifetime
	poolConfig.MaxConnIdleTime = connMaxIdleTime

	// Set timezone to UTC for all connections to ensure consistent timestamp handling
	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET TIMEZONE='UTC'")
		return err
	}

	// Create connection pool
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return NewStore(pool), nil
}

// NewPostgresStore creates a PostgreSQL store with auto-scaled connection pool.
// Pool size adapts to available CPU cores (respects container limits in Go 1.25+).
func NewPostgresStore(ctx context.Context, connString string) (*Store, error) {
	return NewStoreWithConfig(ctx, DBConfig{
		DSN: connString,
	})
}

// NewPostgresStoreWithPoolConfig creates a PostgreSQL store with custom connection pool settings.
// Set pool size fields to 0 to enable auto-scaling based on available CPUs.
func NewPostgresStoreWithPoolConfig(ctx context.Context, connString string, poolConfig DBConfig) (*Store, error) {
	poolConfig.DSN = connString
	return NewStoreWithConfig(ctx, poolConfig)
}

// runMigrationsWithDSN runs PostgreSQL database migrations using goose with embedded files.
// Uses a temporary database/sql connection since goose requires it.
func runMigrationsWithDSN(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database for migrations: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.ErrorContext(ctx, "Failed to close migration database connection", "error", err)
		}
	}()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database for migrations: %w", err)
	}

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
