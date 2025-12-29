package config

import (
	"fmt"

	"github.com/rezkam/mono/internal/env"
)

// TestConfig holds configuration for integration and benchmark tests.
type TestConfig struct {
	Database DatabaseConfig
}

// LoadTestConfig loads and validates test configuration from environment.
func LoadTestConfig() (*TestConfig, error) {
	cfg := &TestConfig{}

	if err := env.Load(cfg); err != nil {
		return nil, fmt.Errorf("failed to load test config: %w", err)
	}

	return cfg, nil
}
