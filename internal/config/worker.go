package config

import (
	"fmt"
	"time"

	"github.com/rezkam/mono/internal/env"
)

// WorkerConfig holds all configuration for the worker binary.
type WorkerConfig struct {
	Database         DatabaseConfig
	OperationTimeout time.Duration `env:"MONO_WORKER_OPERATION_TIMEOUT"`
}

// LoadWorkerConfig loads and validates worker configuration from environment.
func LoadWorkerConfig() (*WorkerConfig, error) {
	cfg := &WorkerConfig{}

	if err := env.Load(cfg); err != nil {
		return nil, fmt.Errorf("failed to load worker config: %w", err)
	}

	return cfg, nil
}
