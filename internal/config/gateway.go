package config

// GatewayConfig holds REST gateway configuration.
type GatewayConfig struct {
	RESTPort string `env:"MONO_REST_PORT" default:"8081"`

	RESTReadTimeout  int `env:"MONO_REST_READ_TIMEOUT" default:"5"`
	RESTWriteTimeout int `env:"MONO_REST_WRITE_TIMEOUT" default:"10"`
	RESTIdleTimeout  int `env:"MONO_REST_IDLE_TIMEOUT" default:"120"`
}
