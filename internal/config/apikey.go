package config

import (
	"errors"
	"fmt"

	"github.com/rezkam/mono/internal/env"
)

// Validation errors for API key generation config.
var (
	ErrNameRequired = errors.New("name is required (use -name flag)")
	ErrInvalidDays  = errors.New("days must be >= 0 (0 = never expires)")
)

// APIKeyConfig holds API key format configuration.
type APIKeyConfig struct {
	KeyType     string `env:"MONO_API_KEY_TYPE"`
	ServiceName string `env:"MONO_API_SERVICE_NAME"`
	Version     string `env:"MONO_API_VERSION"`
}

// APIKeyGenConfig holds all configuration for the apikey generator binary.
type APIKeyGenConfig struct {
	Database  DatabaseConfig
	APIKey    APIKeyConfig
	Name      string // from command-line flag
	DaysValid int    // from command-line flag
}

// LoadAPIKeyGenConfig loads apikey generation configuration from environment.
// name and daysValid come from command-line flags.
func LoadAPIKeyGenConfig(name string, daysValid int) (*APIKeyGenConfig, error) {
	cfg := &APIKeyGenConfig{
		Name:      name,
		DaysValid: daysValid,
	}

	if err := env.Load(cfg); err != nil {
		return nil, fmt.Errorf("failed to load apikey config: %w", err)
	}

	return cfg, nil
}

// Validate validates apikey generation configuration.
func (c *APIKeyGenConfig) Validate() error {
	if c.Name == "" {
		return ErrNameRequired
	}

	if c.DaysValid < 0 {
		return ErrInvalidDays
	}

	return nil
}
