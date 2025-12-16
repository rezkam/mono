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
	StorageType string `env:"MONO_STORAGE_TYPE" default:"fs"` // fs, gcs
	GCSBucket   string `env:"MONO_GCS_BUCKET"`
	FSDir       string `env:"MONO_FS_DIR" default:"./mono-data"`

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
	default:
		return fmt.Errorf("unknown MONO_STORAGE_TYPE: %s", c.StorageType)
	}
	return nil
}
