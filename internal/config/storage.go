package config

import "fmt"

// StorageConfig holds storage connection configuration.
type StorageConfig struct {
	// StorageDSN is the Data Source Name (connection string) for the storage backend.
	// For PostgreSQL: postgres://username:password@hostname:port/database?options
	StorageDSN  string `env:"MONO_STORAGE_DSN"`
	StorageType string `env:"MONO_STORAGE_TYPE" default:"postgres"`
}

// Validate validates the storage configuration.
func (c *StorageConfig) Validate() error {
	if c.StorageType != "postgres" {
		return fmt.Errorf("unsupported MONO_STORAGE_TYPE: %s (only 'postgres' is supported)", c.StorageType)
	}
	if c.StorageDSN == "" {
		return fmt.Errorf("MONO_STORAGE_DSN is required")
	}
	return nil
}
