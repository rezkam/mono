package config

// APIKeyConfig holds API key format configuration.
type APIKeyConfig struct {
	APIKeyType     string `env:"MONO_API_KEY_TYPE" default:"sk"`
	APIServiceName string `env:"MONO_API_SERVICE_NAME" default:"mono"`
	APIVersion     string `env:"MONO_API_VERSION" default:"v1"`
}

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	AuthOperationTimeout int `env:"MONO_AUTH_OPERATION_TIMEOUT" default:"5"`
}
