package config

import (
	"fmt"

	"github.com/rezkam/mono/internal/env"
)

// Config holds the application configuration.
type Config struct {
	// Server Configuration
	GRPCPort string `env:"MONO_GRPC_PORT" default:"8080"`
	RESTPort string `env:"MONO_REST_PORT" default:"8081"`
	GRPCHost string `env:"MONO_GRPC_HOST" default:"localhost"` // Host for gRPC server (used by REST gateway to connect)
	Env      string `env:"MONO_ENV" default:"dev"`             // dev, prod

	// Timeouts Configuration
	ShutdownTimeout  int `env:"MONO_SHUTDOWN_TIMEOUT" default:"10"`   // Graceful shutdown timeout in seconds
	RESTReadTimeout  int `env:"MONO_REST_READ_TIMEOUT" default:"5"`   // REST gateway ReadHeaderTimeout in seconds
	RESTWriteTimeout int `env:"MONO_REST_WRITE_TIMEOUT" default:"10"` // REST gateway WriteTimeout in seconds
	RESTIdleTimeout  int `env:"MONO_REST_IDLE_TIMEOUT" default:"120"` // REST gateway IdleTimeout in seconds

	// gRPC Keepalive Configuration
	GRPCKeepaliveTime                           int  `env:"MONO_GRPC_KEEPALIVE_TIME" default:"300"`                                // How long to wait before sending keepalive ping (seconds)
	GRPCKeepaliveTimeout                        int  `env:"MONO_GRPC_KEEPALIVE_TIMEOUT" default:"20"`                              // How long to wait for keepalive ping ack (seconds)
	GRPCMaxConnectionIdle                       int  `env:"MONO_GRPC_MAX_CONNECTION_IDLE" default:"900"`                           // Max idle time before closing connection (seconds, 15min)
	GRPCMaxConnectionAge                        int  `env:"MONO_GRPC_MAX_CONNECTION_AGE" default:"1800"`                           // Max connection lifetime (seconds, 30min)
	GRPCMaxConnectionAgeGrace                   int  `env:"MONO_GRPC_MAX_CONNECTION_AGE_GRACE" default:"5"`                        // Grace period for closing after max age (seconds)
	GRPCConnectionTimeout                       int  `env:"MONO_GRPC_CONNECTION_TIMEOUT" default:"120"`                            // Connection establishment timeout (seconds)
	GRPCKeepaliveEnforcementMinTime             int  `env:"MONO_GRPC_KEEPALIVE_ENFORCEMENT_MIN_TIME" default:"5"`                  // Min time between pings from client (seconds)
	GRPCKeepaliveEnforcementPermitWithoutStream bool `env:"MONO_GRPC_KEEPALIVE_ENFORCEMENT_PERMIT_WITHOUT_STREAM" default:"false"` // Allow pings without active streams

	// Storage Configuration
	StorageType string `env:"MONO_STORAGE_TYPE" default:"postgres"`
	PostgresURL string `env:"MONO_POSTGRES_URL"`

	// SQL Connection Pool Configuration
	DBMaxOpenConns    int `env:"MONO_DB_MAX_OPEN_CONNS" default:"25"`     // Maximum open connections
	DBMaxIdleConns    int `env:"MONO_DB_MAX_IDLE_CONNS" default:"5"`      // Maximum idle connections
	DBConnMaxLifetime int `env:"MONO_DB_CONN_MAX_LIFETIME" default:"300"` // Connection max lifetime in seconds (default: 5min)
	DBConnMaxIdleTime int `env:"MONO_DB_CONN_MAX_IDLE_TIME" default:"60"` // Connection max idle time in seconds (default: 1min)

	// Observability Configuration
	OTelEnabled   bool   `env:"MONO_OTEL_ENABLED" default:"true"`
	OTelCollector string `env:"MONO_OTEL_COLLECTOR" default:"localhost:4317"`

	// API Key Configuration
	// Full key format: {KeyType}-{ServiceName}-{APIVersion}-{short}-{long}
	// Example: sk-mono-v1-a7f3d8e2-8h3k2jf9s7d6f5g4h3j2k1
	APIKeyType     string `env:"MONO_API_KEY_TYPE" default:"sk"`       // Key type (sk=secret, pk=public)
	APIServiceName string `env:"MONO_API_SERVICE_NAME" default:"mono"` // Service name
	APIVersion     string `env:"MONO_API_VERSION" default:"v1"`        // API version

	// Pagination Configuration
	DefaultPageSize int `env:"MONO_DEFAULT_PAGE_SIZE" default:"50"` // Default page size for list operations
	MaxPageSize     int `env:"MONO_MAX_PAGE_SIZE" default:"100"`    // Maximum allowed page size
}

// Load parses environment variables into a Config struct.
// It enforces the MONO_ prefix and validated dependencies.
func Load() (*Config, error) {
	cfg := &Config{}

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.StorageType != "postgres" {
		return fmt.Errorf("unsupported MONO_STORAGE_TYPE: %s (only 'postgres' is supported)", c.StorageType)
	}

	if c.PostgresURL == "" {
		return fmt.Errorf("MONO_POSTGRES_URL is required")
	}

	return nil
}
