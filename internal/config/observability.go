package config

// ObservabilityConfig holds observability configuration.
type ObservabilityConfig struct {
	OTelEnabled bool `env:"MONO_OTEL_ENABLED" default:"true"`
}
