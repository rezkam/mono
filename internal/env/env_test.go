package env

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestConfig struct {
	Host    string `env:"TEST_HOST"`
	Port    int    `env:"TEST_PORT"`
	Enabled bool   `env:"TEST_ENABLED"`
}

func TestLoad(t *testing.T) {
	os.Clearenv()
	os.Setenv("TEST_HOST", "example.com")
	os.Setenv("TEST_PORT", "9090")
	os.Setenv("TEST_ENABLED", "false")

	var cfg TestConfig
	err := Load(&cfg)
	require.NoError(t, err)

	assert.Equal(t, "example.com", cfg.Host)
	assert.Equal(t, 9090, cfg.Port)
	assert.False(t, cfg.Enabled)
}

func TestLoad_ZeroValuesForUnset(t *testing.T) {
	os.Clearenv()
	// No env vars set

	var cfg TestConfig
	err := Load(&cfg)
	require.NoError(t, err)

	// Unset fields should be zero values
	assert.Empty(t, cfg.Host)
	assert.Equal(t, 0, cfg.Port)
	assert.False(t, cfg.Enabled)
}

func TestLoad_InvalidValue(t *testing.T) {
	os.Clearenv()
	os.Setenv("TEST_PORT", "not-a-number")

	var cfg TestConfig
	err := Load(&cfg)

	require.Error(t, err)
	var invalidErr ErrInvalidValue
	require.True(t, errors.As(err, &invalidErr))
	assert.Equal(t, "Port", invalidErr.Field)
	assert.Equal(t, "TEST_PORT", invalidErr.EnvVar)
	assert.Equal(t, "not-a-number", invalidErr.Value)
}

func TestLoad_EmptyStringRespected(t *testing.T) {
	os.Clearenv()
	os.Setenv("TEST_HOST", "") // Empty string explicitly set

	var cfg TestConfig
	err := Load(&cfg)
	require.NoError(t, err)

	// Empty string is a valid value, should be set
	assert.Equal(t, "", cfg.Host)
}

func TestLoad_NestedStruct(t *testing.T) {
	type DatabaseConfig struct {
		DSN          string `env:"DB_DSN"`
		MaxOpenConns int    `env:"DB_MAX_CONNS"`
	}

	type AppConfig struct {
		Database DatabaseConfig
		AppName  string `env:"APP_NAME"`
	}

	t.Run("loads nested struct fields", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("DB_DSN", "postgres://localhost/db")
		os.Setenv("DB_MAX_CONNS", "10")
		os.Setenv("APP_NAME", "testapp")

		var cfg AppConfig
		err := Load(&cfg)
		require.NoError(t, err)

		assert.Equal(t, "postgres://localhost/db", cfg.Database.DSN)
		assert.Equal(t, 10, cfg.Database.MaxOpenConns)
		assert.Equal(t, "testapp", cfg.AppName)
	})

	t.Run("nested struct fields default to zero", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("APP_NAME", "testapp")

		var cfg AppConfig
		err := Load(&cfg)
		require.NoError(t, err)

		assert.Empty(t, cfg.Database.DSN)
		assert.Equal(t, 0, cfg.Database.MaxOpenConns)
		assert.Equal(t, "testapp", cfg.AppName)
	})
}

func TestLoad_EmbeddedStruct(t *testing.T) {
	type BaseConfig struct {
		DSN string `env:"STORAGE_DSN"`
	}

	type AppConfig struct {
		BaseConfig        // embedded (anonymous)
		AppName    string `env:"APP_NAME"`
	}

	os.Clearenv()
	os.Setenv("STORAGE_DSN", "postgres://localhost/db")
	os.Setenv("APP_NAME", "testapp")

	var cfg AppConfig
	err := Load(&cfg)
	require.NoError(t, err)

	assert.Equal(t, "postgres://localhost/db", cfg.DSN)
	assert.Equal(t, "testapp", cfg.AppName)
}

func TestLoad_Duration(t *testing.T) {
	type DurationConfig struct {
		Timeout     time.Duration `env:"TIMEOUT"`
		ReadTimeout time.Duration `env:"READ_TIMEOUT"`
	}

	t.Run("loads duration values", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("TIMEOUT", "30s")
		os.Setenv("READ_TIMEOUT", "5m30s")

		var cfg DurationConfig
		err := Load(&cfg)
		require.NoError(t, err)

		assert.Equal(t, 30*time.Second, cfg.Timeout)
		assert.Equal(t, 5*time.Minute+30*time.Second, cfg.ReadTimeout)
	})

	t.Run("invalid duration returns error", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("READ_TIMEOUT", "invalid")

		var cfg DurationConfig
		err := Load(&cfg)

		require.Error(t, err)
		var invalidErr ErrInvalidValue
		require.True(t, errors.As(err, &invalidErr))
		assert.Equal(t, "ReadTimeout", invalidErr.Field)
	})
}

func TestLoad_BoolValues(t *testing.T) {
	type BoolConfig struct {
		Flag bool `env:"FLAG"`
	}

	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{"false", false},
		{"FALSE", false},
		{"False", false},
		{"0", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			os.Clearenv()
			os.Setenv("FLAG", tt.value)

			var cfg BoolConfig
			err := Load(&cfg)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, cfg.Flag)
		})
	}
}

func TestLoad_NotStructPointer(t *testing.T) {
	t.Run("non-pointer fails", func(t *testing.T) {
		var cfg TestConfig
		err := Load(cfg) // Not a pointer
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pointer to struct")
	})

	t.Run("pointer to non-struct fails", func(t *testing.T) {
		var s string
		err := Load(&s)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pointer to struct")
	})
}

func TestLoad_DeeplyNestedStruct(t *testing.T) {
	type Level3 struct {
		Value string `env:"LEVEL3_VALUE"`
	}

	type Level2 struct {
		Nested Level3
		Name   string `env:"LEVEL2_NAME"`
	}

	type Level1 struct {
		Child Level2
		ID    int `env:"LEVEL1_ID"`
	}

	os.Clearenv()
	os.Setenv("LEVEL3_VALUE", "deep")
	os.Setenv("LEVEL2_NAME", "middle")
	os.Setenv("LEVEL1_ID", "42")

	var cfg Level1
	err := Load(&cfg)
	require.NoError(t, err)

	assert.Equal(t, 42, cfg.ID)
	assert.Equal(t, "middle", cfg.Child.Name)
	assert.Equal(t, "deep", cfg.Child.Nested.Value)
}

func TestLoad_AutoValidatesNestedStructs(t *testing.T) {
	os.Clearenv()
	// DB_DSN not set - should fail validation

	type DatabaseConfig struct {
		DSN string `env:"DB_DSN"`
	}

	// Add Validate method via wrapper
	type ValidatedDB struct {
		DSN string `env:"DB_DSN"`
	}

	type AppConfig struct {
		Database ValidatedDB
	}

	var cfg AppConfig
	err := Load(&cfg)
	// Should succeed since ValidatedDB doesn't implement Validator
	require.NoError(t, err)
}

func TestLoad_ValidatorCalledOnNestedStruct(t *testing.T) {
	os.Clearenv()
	os.Setenv("APP_NAME", "test")
	// VALIDATED_VALUE not set

	var cfg configWithValidator
	err := Load(&cfg)

	// Should fail because nested ValidatedConfig.Validate() returns error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "value is required")
}

// Test types for validation
type validatedConfig struct {
	Value string `env:"VALIDATED_VALUE"`
}

func (c *validatedConfig) Validate() error {
	if c.Value == "" {
		return errors.New("value is required")
	}
	return nil
}

type configWithValidator struct {
	Validated validatedConfig
	AppName   string `env:"APP_NAME"`
}
