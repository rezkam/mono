package config

import (
	"fmt"

	"github.com/rezkam/mono/internal/env"
)

// WorkerConfig holds all configuration for the worker binary.
type WorkerConfig struct {
	StorageConfig

	WorkerOperationTimeout int `env:"MONO_WORKER_OPERATION_TIMEOUT" default:"30"`
}

// LoadWorkerConfig loads and validates worker configuration.
func LoadWorkerConfig() (*WorkerConfig, error) {
	cfg := &WorkerConfig{}

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse worker config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate validates worker-specific configuration.
func (c *WorkerConfig) Validate() error {
	if err := c.StorageConfig.Validate(); err != nil {
		return err
	}

	if c.WorkerOperationTimeout < 1 {
		return fmt.Errorf("MONO_WORKER_OPERATION_TIMEOUT must be at least 1 second")
	}

	return nil
}
