package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	os.Clearenv()
	// Set required fields for validation
	os.Setenv("MONO_POSTGRES_URL", "postgres://user:pass@localhost:5432/dbname")

	cfg, err := Load()
	require.NoError(t, err)

	// Verify defaults
	assert.Equal(t, "8080", cfg.GRPCPort)
	assert.Equal(t, "8081", cfg.RESTPort)
	assert.Equal(t, "localhost", cfg.GRPCHost)
	assert.Equal(t, "dev", cfg.Env)
	assert.Equal(t, "postgres", cfg.StorageType)
	assert.Equal(t, 10, cfg.ShutdownTimeout)
	assert.Equal(t, 5, cfg.RESTReadTimeout)
	assert.Equal(t, 10, cfg.RESTWriteTimeout)
	assert.Equal(t, 120, cfg.RESTIdleTimeout)

	// DB pool defaults
	assert.Equal(t, 25, cfg.DBMaxOpenConns)
	assert.Equal(t, 5, cfg.DBMaxIdleConns)
	assert.Equal(t, 300, cfg.DBConnMaxLifetime)
	assert.Equal(t, 60, cfg.DBConnMaxIdleTime)

	// Observability defaults
	assert.Equal(t, true, cfg.OTelEnabled)
}

func TestLoad_WithEnv(t *testing.T) {
	os.Clearenv()
	os.Setenv("MONO_GRPC_PORT", "9090")
	os.Setenv("MONO_REST_PORT", "9091")
	os.Setenv("MONO_ENV", "prod")
	os.Setenv("MONO_POSTGRES_URL", "postgres://prod:secret@prod-db:5432/prod")
	os.Setenv("MONO_DB_MAX_OPEN_CONNS", "50")
	os.Setenv("MONO_DB_MAX_IDLE_CONNS", "10")
	os.Setenv("MONO_OTEL_ENABLED", "false")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "9090", cfg.GRPCPort)
	assert.Equal(t, "9091", cfg.RESTPort)
	assert.Equal(t, "prod", cfg.Env)
	assert.Equal(t, "postgres://prod:secret@prod-db:5432/prod", cfg.PostgresURL)
	assert.Equal(t, 50, cfg.DBMaxOpenConns)
	assert.Equal(t, 10, cfg.DBMaxIdleConns)
	assert.Equal(t, false, cfg.OTelEnabled)
}

func TestLoad_Validation_MissingPostgresURL(t *testing.T) {
	os.Clearenv()
	os.Setenv("MONO_STORAGE_TYPE", "postgres")
	// Missing POSTGRES_URL

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MONO_POSTGRES_URL is required")
}

func TestLoad_Validation_InvalidStorageType(t *testing.T) {
	os.Clearenv()
	os.Setenv("MONO_STORAGE_TYPE", "mysql")
	os.Setenv("MONO_POSTGRES_URL", "postgres://localhost/db")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported MONO_STORAGE_TYPE")
	assert.Contains(t, err.Error(), "only 'postgres' is supported")
}

func TestLoad_Validation_EmptyPostgresURL(t *testing.T) {
	os.Clearenv()
	os.Setenv("MONO_STORAGE_TYPE", "postgres")
	os.Setenv("MONO_POSTGRES_URL", "")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MONO_POSTGRES_URL is required")
}

func TestLoad_DBPoolConfig(t *testing.T) {
	os.Clearenv()
	os.Setenv("MONO_POSTGRES_URL", "postgres://localhost/db")
	os.Setenv("MONO_DB_MAX_OPEN_CONNS", "100")
	os.Setenv("MONO_DB_MAX_IDLE_CONNS", "20")
	os.Setenv("MONO_DB_CONN_MAX_LIFETIME", "600")
	os.Setenv("MONO_DB_CONN_MAX_IDLE_TIME", "120")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, 100, cfg.DBMaxOpenConns)
	assert.Equal(t, 20, cfg.DBMaxIdleConns)
	assert.Equal(t, 600, cfg.DBConnMaxLifetime)
	assert.Equal(t, 120, cfg.DBConnMaxIdleTime)
}

func TestLoad_TimeoutConfig(t *testing.T) {
	os.Clearenv()
	os.Setenv("MONO_POSTGRES_URL", "postgres://localhost/db")
	os.Setenv("MONO_SHUTDOWN_TIMEOUT", "30")
	os.Setenv("MONO_REST_READ_TIMEOUT", "10")
	os.Setenv("MONO_REST_WRITE_TIMEOUT", "20")
	os.Setenv("MONO_REST_IDLE_TIMEOUT", "300")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, 30, cfg.ShutdownTimeout)
	assert.Equal(t, 10, cfg.RESTReadTimeout)
	assert.Equal(t, 20, cfg.RESTWriteTimeout)
	assert.Equal(t, 300, cfg.RESTIdleTimeout)
}
