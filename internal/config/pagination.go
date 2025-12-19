package config

import "fmt"

// PaginationConfig holds pagination configuration.
type PaginationConfig struct {
	DefaultPageSize int `env:"MONO_DEFAULT_PAGE_SIZE" default:"50"`
	MaxPageSize     int `env:"MONO_MAX_PAGE_SIZE" default:"100"`
}

// Validate validates pagination configuration.
func (c *PaginationConfig) Validate() error {
	if c.MaxPageSize < c.DefaultPageSize {
		return fmt.Errorf("MONO_MAX_PAGE_SIZE (%d) must be >= MONO_DEFAULT_PAGE_SIZE (%d)", c.MaxPageSize, c.DefaultPageSize)
	}
	return nil
}
