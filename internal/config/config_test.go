package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	os.Clearenv()
	// Set valid defaults for validation pass
	os.Setenv("MONO_STORAGE_TYPE", "fs")
	os.Setenv("MONO_FS_DIR", "./data")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "8080", cfg.GRPCPort) // Default
	assert.Equal(t, "fs", cfg.StorageType)
}

func TestLoad_WithEnv(t *testing.T) {
	os.Clearenv()
	os.Setenv("MONO_GRPC_PORT", "9090")
	os.Setenv("MONO_STORAGE_TYPE", "fs")
	os.Setenv("MONO_FS_DIR", "./data")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "9090", cfg.GRPCPort)
}

func TestLoad_Validation_GCS(t *testing.T) {
	os.Clearenv()
	os.Setenv("MONO_STORAGE_TYPE", "gcs")
	// Missing BUCKET

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MONO_GCS_BUCKET is required")
}

func TestLoad_Validation_FS(t *testing.T) {
	os.Clearenv()
	os.Setenv("MONO_STORAGE_TYPE", "fs")
	os.Setenv("MONO_FS_DIR", "") // Explicit empty, overriding default if parsed correctly?
	// Note: custom parser prioritizes Env over Default. Empty env string = empty value.

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MONO_FS_DIR is required")
}
