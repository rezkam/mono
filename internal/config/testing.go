package config

import (
	"fmt"

	"github.com/rezkam/mono/internal/env"
)

// TestConfig holds configuration for integration and benchmark tests.
type TestConfig struct {
	StorageConfig
}

// LoadTestConfig loads and validates test configuration.
func LoadTestConfig() (*TestConfig, error) {
	cfg := &TestConfig{}

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse test config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate validates test configuration.
func (c *TestConfig) Validate() error {
	return c.StorageConfig.Validate()
}
