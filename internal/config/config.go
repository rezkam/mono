package config

import (
	"fmt"

	"github.com/rezkam/mono/internal/env"
)

// Config holds the application configuration.
type Config struct {
	// Server Configuration
	GRPCPort string `env:"MONO_GRPC_PORT" default:"8080"`
	HTTPPort string `env:"MONO_HTTP_PORT" default:"8081"`
	Env      string `env:"MONO_ENV" default:"dev"` // dev, prod

	// Storage Configuration
	StorageType string `env:"MONO_STORAGE_TYPE" default:"fs"` // fs, gcs, postgres, sqlite
	GCSBucket   string `env:"MONO_GCS_BUCKET"`
	FSDir       string `env:"MONO_FS_DIR" default:"./mono-data"`

	// SQL Storage Configuration
	PostgresURL string `env:"MONO_POSTGRES_URL"` // PostgreSQL connection string
	SQLitePath  string `env:"MONO_SQLITE_PATH" default:"./mono-data/mono.db"`

	// SQL Connection Pool Configuration
	DBMaxOpenConns    int `env:"MONO_DB_MAX_OPEN_CONNS" default:"25"`     // Maximum open connections
	DBMaxIdleConns    int `env:"MONO_DB_MAX_IDLE_CONNS" default:"5"`      // Maximum idle connections
	DBConnMaxLifetime int `env:"MONO_DB_CONN_MAX_LIFETIME" default:"300"` // Connection max lifetime in seconds (default: 5min)
	DBConnMaxIdleTime int `env:"MONO_DB_CONN_MAX_IDLE_TIME" default:"60"` // Connection max idle time in seconds (default: 1min)

	// Observability Configuration
	OTelEnabled   bool   `env:"MONO_OTEL_ENABLED" default:"true"`
	OTelCollector string `env:"MONO_OTEL_COLLECTOR" default:"localhost:4317"`
}

// Load parses environment variables into a Config struct.
// It enforces the MONO_ prefix and validated dependencies.
func Load() (*Config, error) {
	cfg := &Config{}

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	switch c.StorageType {
	case "fs":
		if c.FSDir == "" {
			return fmt.Errorf("MONO_FS_DIR is required when MONO_STORAGE_TYPE is 'fs'")
		}
	case "gcs":
		if c.GCSBucket == "" {
			return fmt.Errorf("MONO_GCS_BUCKET is required when MONO_STORAGE_TYPE is 'gcs'")
		}
	case "postgres":
		if c.PostgresURL == "" {
			return fmt.Errorf("MONO_POSTGRES_URL is required when MONO_STORAGE_TYPE is 'postgres'")
		}
	case "sqlite":
		if c.SQLitePath == "" {
			return fmt.Errorf("MONO_SQLITE_PATH is required when MONO_STORAGE_TYPE is 'sqlite'")
		}
	default:
		return fmt.Errorf("unknown MONO_STORAGE_TYPE: %s (supported: fs, gcs, postgres, sqlite)", c.StorageType)
	}
	return nil
}
