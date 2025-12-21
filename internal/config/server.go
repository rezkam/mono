package config

import (
	"fmt"

	"github.com/rezkam/mono/internal/env"
)

// ServerConfig holds all configuration for the server binary.
type ServerConfig struct {
	StorageConfig

	Env string `env:"MONO_ENV" default:"dev"`
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
	return c.StorageConfig.Validate()
}
