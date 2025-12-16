package gcs

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/core"
	"github.com/rezkam/mono/internal/storage/compliance"
	"github.com/stretchr/testify/require"
)

func TestGCSStore_Compliance(t *testing.T) {
	bucket := os.Getenv("TEST_GCS_BUCKET")
	if bucket == "" {
		t.Skip("TEST_GCS_BUCKET not set, skipping GCS tests")
	}

	compliance.RunStorageComplianceTest(t, func() (core.Storage, func()) {
		// Note: This assumes Application Default Credentials are set up
		// and point to a valid project with access to the bucket.
		ctx := context.Background()

		store, err := NewStore(ctx, bucket)
		require.NoError(t, err)

		// Cleanup function that deletes all .json objects in the bucket
		// This runs even if the test fails (via defer in compliance framework)
		cleanup := func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// List all objects in the bucket
			it := store.client.Bucket(bucket).Objects(cleanupCtx, nil)
			var objectsToDelete []string

			for {
				attrs, err := it.Next()
				if err != nil {
					if err.Error() == "iterator.Done" || err.Error() == "no more items in iterator" {
						break
					}
					t.Logf("Warning: failed to list objects during cleanup: %v", err)
					break
				}
				// Only delete .json files created by tests
				if len(attrs.Name) > 0 && attrs.Name[len(attrs.Name)-5:] == ".json" {
					objectsToDelete = append(objectsToDelete, attrs.Name)
				}
			}

			// Delete all test objects
			for _, name := range objectsToDelete {
				obj := store.client.Bucket(bucket).Object(name)
				if err := obj.Delete(cleanupCtx); err != nil {
					t.Logf("Warning: failed to delete object %s: %v", name, err)
				} else {
					t.Logf("Cleaned up test object: %s", name)
				}
			}
		}

		return store, cleanup
	})
}
