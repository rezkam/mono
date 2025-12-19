package config

import (
	"fmt"

	"github.com/rezkam/mono/internal/env"
)

// ServerConfig holds all configuration for the server binary.
type ServerConfig struct {
	StorageConfig
	StoragePoolConfig
	GRPCConfig
	GatewayConfig
	AuthConfig
	APIKeyConfig
	PaginationConfig
	ObservabilityConfig

	Env             string `env:"MONO_ENV" default:"dev"`
	ShutdownTimeout int    `env:"MONO_SHUTDOWN_TIMEOUT" default:"10"`
}

// LoadServerConfig loads and validates server configuration.
func LoadServerConfig() (*ServerConfig, error) {
	cfg := &ServerConfig{}

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse server config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate validates server-specific configuration.
func (c *ServerConfig) Validate() error {
	if err := c.StorageConfig.Validate(); err != nil {
		return err
	}

	if err := c.PaginationConfig.Validate(); err != nil {
		return err
	}

	if c.ShutdownTimeout < 1 {
		return fmt.Errorf("MONO_SHUTDOWN_TIMEOUT must be at least 1 second")
	}

	return nil
}
