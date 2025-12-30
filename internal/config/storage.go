package config

import "errors"

// ErrDSNRequired is returned when the database DSN is not configured.
var ErrDSNRequired = errors.New("MONO_DB_DSN is required")

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	// DSN is the Data Source Name (connection string) for the database.
	// For PostgreSQL: postgres://username:password@hostname:port/database?options
	DSN string `env:"MONO_DB_DSN"`

	// Connection pool settings (zero = use infrastructure defaults)
	MaxOpenConns    int `env:"MONO_DB_MAX_OPEN_CONNS"`
	MaxIdleConns    int `env:"MONO_DB_MAX_IDLE_CONNS"`
	ConnMaxLifetime int `env:"MONO_DB_CONN_MAX_LIFETIME_SEC"`  // seconds
	ConnMaxIdleTime int `env:"MONO_DB_CONN_MAX_IDLE_TIME_SEC"` // seconds

	// AutoMigrate enables automatic migrations on startup.
	// Disabled by default; set to true for development or when not using external migration tools.
	AutoMigrate bool `env:"MONO_DB_AUTO_MIGRATE"`
}

// Validate validates the database configuration.
func (c *DatabaseConfig) Validate() error {
	if c.DSN == "" {
		return ErrDSNRequired
	}
	return nil
}
