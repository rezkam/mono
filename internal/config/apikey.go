package config

import (
	"fmt"

	"github.com/rezkam/mono/internal/env"
)

// APIKeyConfig holds API key format configuration.
type APIKeyConfig struct {
	APIKeyType     string `env:"MONO_API_KEY_TYPE" default:"sk"`
	APIServiceName string `env:"MONO_API_SERVICE_NAME" default:"mono"`
	APIVersion     string `env:"MONO_API_VERSION" default:"v1"`
}

// APIKeyGenConfig holds all configuration for the apikey binary.
type APIKeyGenConfig struct {
	StorageConfig
	APIKeyConfig

	Name      string
	DaysValid int
}

// LoadAPIKeyGenConfig loads and validates apikey generation configuration.
func LoadAPIKeyGenConfig(name string, daysValid int) (*APIKeyGenConfig, error) {
	cfg := &APIKeyGenConfig{
		Name:      name,
		DaysValid: daysValid,
	}

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse apikey config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate validates apikey generation configuration.
func (c *APIKeyGenConfig) Validate() error {
	if err := c.StorageConfig.Validate(); err != nil {
		return err
	}

	if c.Name == "" {
		return fmt.Errorf("name is required (use -name flag)")
	}

	if c.DaysValid < 0 {
		return fmt.Errorf("days must be >= 0 (0 = never expires)")
	}

	return nil
}
