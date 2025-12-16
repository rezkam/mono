package fs_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/core"
	"github.com/rezkam/mono/internal/storage/fs"
)

func BenchmarkFS_ListLists_100Lists(b *testing.B) {
	// Setup: Create a temp directory with 100 lists, 1000 items each.
	tmpDir, err := os.MkdirTemp("", "mono-bench-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := fs.NewStore(tmpDir)
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	// Pre-populate data
	// We use parallel workers to setup faster, but the benchmark is for READs.
	// 100 lists * 1000 items ~ 20MB total.
	for i := 0; i < 100; i++ {
		items := make([]core.TodoItem, 1000)
		for j := 0; j < 1000; j++ {
			items[j] = core.TodoItem{
				ID:         fmt.Sprintf("item-%d-%d", i, j),
				Title:      "Benchmark Item Payload",
				Completed:  j%2 == 0,
				CreateTime: time.Now().UTC(),
			}
		}
		list := &core.TodoList{
			ID:         fmt.Sprintf("list-%d", i),
			Title:      fmt.Sprintf("List %d", i),
			Items:      items,
			CreateTime: time.Now().UTC(),
		}
		if err := store.CreateList(ctx, list); err != nil {
			b.Fatalf("setup failed: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lists, err := store.ListLists(ctx)
		if err != nil {
			b.Fatalf("ListLists failed: %v", err)
		}
		if len(lists) != 100 {
			b.Fatalf("expected 100 lists, got %d", len(lists))
		}
	}
}
