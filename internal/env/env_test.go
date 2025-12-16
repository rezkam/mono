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
