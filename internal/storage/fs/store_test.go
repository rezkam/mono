package fs

import (
	"os"
	"testing"

	"github.com/rezkam/mono/internal/core"
	"github.com/rezkam/mono/internal/storage/compliance"
	"github.com/stretchr/testify/require"
)

func TestFSStore_Compliance(t *testing.T) {
	compliance.RunStorageComplianceTest(t, func() (core.Storage, func()) {
		tmpDir, err := os.MkdirTemp("", "fs-store-test-*")
		require.NoError(t, err)

		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		cleanup := func() {
			os.RemoveAll(tmpDir)
		}

		return store, cleanup
	})
}
