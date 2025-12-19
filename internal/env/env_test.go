package env

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestConfig struct {
	Host    string `env:"TEST_HOST" default:"localhost"`
	Port    int    `env:"TEST_PORT" default:"8080"`
	Enabled bool   `env:"TEST_ENABLED" default:"true"`
	NoDef   string `env:"TEST_NO_DEF"`
}

func TestParse(t *testing.T) {
	os.Clearenv()
	os.Setenv("TEST_HOST", "example.com")
	os.Setenv("TEST_PORT", "9090")
	os.Setenv("TEST_ENABLED", "false")
	os.Setenv("TEST_NO_DEF", "foo")

	var cfg TestConfig
	err := Parse(&cfg)
	require.NoError(t, err)

	assert.Equal(t, "example.com", cfg.Host)
	assert.Equal(t, 9090, cfg.Port)
	assert.False(t, cfg.Enabled)
	assert.Equal(t, "foo", cfg.NoDef)
}

func TestParse_Defaults(t *testing.T) {
	os.Clearenv()

	var cfg TestConfig
	err := Parse(&cfg)
	require.NoError(t, err)

	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, 8080, cfg.Port)
	assert.True(t, cfg.Enabled)
	assert.Empty(t, cfg.NoDef)
}

func TestParse_EmptyStringRespected(t *testing.T) {
	os.Clearenv()
	os.Setenv("TEST_HOST", "") // Empty string for string field

	var cfg TestConfig
	err := Parse(&cfg)
	require.NoError(t, err)

	// Empty strings should be respected for string fields (not use defaults)
	assert.Equal(t, "", cfg.Host)
	// Port not set, so uses default
	assert.Equal(t, 8080, cfg.Port)
}

func TestParse_EmptyStringIntError(t *testing.T) {
	os.Clearenv()
	os.Setenv("TEST_PORT", "") // Empty string for int field

	var cfg TestConfig
	err := Parse(&cfg)
	// Empty string for int field should error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing")
}

func TestParse_EmbeddedStruct(t *testing.T) {
	type BaseConfig struct {
		StorageDSN  string `env:"STORAGE_DSN"`
		StorageType string `env:"STORAGE_TYPE" default:"postgres"`
	}

	type AppConfig struct {
		BaseConfig
		AppName string `env:"APP_NAME" default:"myapp"`
	}

	t.Run("parses embedded struct fields", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("STORAGE_DSN", "postgres://localhost/db")
		os.Setenv("APP_NAME", "testapp")

		var cfg AppConfig
		err := Parse(&cfg)
		require.NoError(t, err)

		assert.Equal(t, "postgres://localhost/db", cfg.StorageDSN)
		assert.Equal(t, "postgres", cfg.StorageType) // Uses default
		assert.Equal(t, "testapp", cfg.AppName)
	})

	t.Run("empty string in embedded struct is respected", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("STORAGE_DSN", "postgres://localhost/db")
		os.Setenv("STORAGE_TYPE", "") // Empty string

		var cfg AppConfig
		err := Parse(&cfg)
		require.NoError(t, err)

		assert.Equal(t, "", cfg.StorageType) // Empty string is respected, not replaced with default
	})
}
