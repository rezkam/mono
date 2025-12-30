package config

import (
	"fmt"
	"time"

	"github.com/rezkam/mono/internal/env"
)

// ServerConfig holds all configuration for the server binary.
type ServerConfig struct {
	Database        DatabaseConfig
	HTTP            HTTPConfig
	Auth            AuthConfig
	Todo            TodoConfig
	Observability   ObservabilityConfig
	ShutdownTimeout time.Duration `env:"MONO_SHUTDOWN_TIMEOUT"`
}

// HTTPConfig holds HTTP server configuration.
type HTTPConfig struct {
	Host              string        `env:"MONO_HTTP_HOST"`
	Port              string        `env:"MONO_HTTP_PORT"`
	ReadTimeout       time.Duration `env:"MONO_HTTP_READ_TIMEOUT"`
	WriteTimeout      time.Duration `env:"MONO_HTTP_WRITE_TIMEOUT"`
	IdleTimeout       time.Duration `env:"MONO_HTTP_IDLE_TIMEOUT"`
	ReadHeaderTimeout time.Duration `env:"MONO_HTTP_READ_HEADER_TIMEOUT"`
	MaxHeaderBytes    int           `env:"MONO_HTTP_MAX_HEADER_BYTES"`
	MaxBodyBytes      int64         `env:"MONO_HTTP_MAX_BODY_BYTES"`

	// TLS configuration for HTTPS
	TLSEnabled  bool   `env:"MONO_TLS_ENABLED"`
	TLSCertFile string `env:"MONO_TLS_CERT_FILE"`
	TLSKeyFile  string `env:"MONO_TLS_KEY_FILE"`
}

// AuthConfig holds authenticator configuration.
type AuthConfig struct {
	OperationTimeout time.Duration `env:"MONO_AUTH_OPERATION_TIMEOUT"`
	UpdateQueueSize  int           `env:"MONO_AUTH_UPDATE_QUEUE_SIZE"`
}

// TodoConfig holds todo service configuration.
type TodoConfig struct {
	DefaultPageSize int `env:"MONO_DEFAULT_PAGE_SIZE"`
	MaxPageSize     int `env:"MONO_MAX_PAGE_SIZE"`
}

// ObservabilityConfig holds observability configuration.
type ObservabilityConfig struct {
	OTelEnabled bool   `env:"MONO_OTEL_ENABLED"`
	ServiceName string `env:"OTEL_SERVICE_NAME"`
}

// LoadServerConfig loads and validates server configuration from environment.
func LoadServerConfig() (*ServerConfig, error) {
	cfg := &ServerConfig{}

	if err := env.Load(cfg); err != nil {
		return nil, fmt.Errorf("failed to load server config: %w", err)
	}

	return cfg, nil
}
