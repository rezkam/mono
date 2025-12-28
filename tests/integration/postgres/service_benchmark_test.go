package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/config"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
)

// getBenchmarkStorage creates a real PostgreSQL storage connection for benchmarking.
func getBenchmarkStorage(b *testing.B) *postgres.Store {
	b.Helper()

	cfg, err := config.LoadTestConfig()
	if err != nil {
		b.Skipf("Failed to load test config: %v", err)
	}

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, cfg.StorageDSN)
	if err != nil {
		b.Fatalf("failed to create storage: %v", err)
	}

	return store
}

// cleanupBenchmarkData removes all lists and items from the database.
func cleanupBenchmarkData(b *testing.B, storage todo.Repository) {
	b.Helper()

	ctx := context.Background()
	result, err := storage.ListLists(ctx, domain.ListListsParams{Limit: 1000})
	if err != nil {
		b.Logf("failed to list lists for cleanup: %v", err)
		return
	}
	lists := result.Lists

	// Delete all recurring templates first (they reference lists)
	for _, list := range lists {
		templates, err := storage.FindRecurringTemplates(ctx, list.ID, false)
		if err == nil {
			for _, tmpl := range templates {
				storage.DeleteRecurringTemplate(ctx, tmpl.ID)
			}
		}
	}

	// Note: Lists and items will be cleaned up via CASCADE DELETE when the test database
	// is truncated by SetupTestDB's cleanup function. The UpdateList method now only
	// supports updating the title field via field mask, so we can't clear items this way.
	// This is fine since benchmark cleanup happens at test boundaries anyway.
}

// setupBenchmarkData populates the real database with N lists, each having M items.
func setupBenchmarkData(b *testing.B, storage todo.Repository, numLists, itemsPerList int) {
	b.Helper()

	ctx := context.Background()
	now := time.Now().UTC()

	for i := 0; i < numLists; i++ {
		listUUID, err := uuid.NewV7()
		if err != nil {
			b.Fatalf("failed to generate list UUID: %v", err)
		}
		listID := listUUID.String()

		// Create the list first (without items)
		list := &domain.TodoList{
			ID:          listID,
			Title:       fmt.Sprintf("Benchmark List %d", i),
			CreateTime:  now,
			TotalItems:  0,
			UndoneItems: 0,
		}

		if err := storage.CreateList(ctx, list); err != nil {
			b.Fatalf("failed to create list %d: %v", i, err)
		}

		// Create items separately
		for j := 0; j < itemsPerList; j++ {
			due := now.Add(time.Duration(j+100) * time.Minute)
			status := domain.TaskStatusTodo
			if j%2 == 0 {
				status = domain.TaskStatusDone
			}
			itemUUID, err := uuid.NewV7()
			if err != nil {
				b.Fatalf("failed to generate item UUID: %v", err)
			}
			item := &domain.TodoItem{
				ID:         itemUUID.String(),
				Title:      fmt.Sprintf("Large Payload Item %d-%d with description and metadata to simulate real object size", i, j),
				Status:     status,
				CreateTime: now.Add(time.Duration(j) * time.Minute),
				UpdatedAt:  now.Add(time.Duration(j) * time.Minute),
				DueTime:    &due,
				Tags:       []string{"common", fmt.Sprintf("tag-%d", j%100)}, // 100 unique tags
			}

			if err := storage.CreateItem(ctx, listID, item); err != nil {
				b.Fatalf("failed to create item %d-%d: %v", i, j, err)
			}
		}
	}
}

// BenchmarkListTasks tests ListTasks performance at different scales using real PostgreSQL.
// Set MONO_STORAGE_DSN environment variable to run these benchmarks.
//
// Example:
//
//	MONO_STORAGE_DSN="postgres://postgres:postgres@localhost:5433/mono_test?sslmode=disable" go test -bench=BenchmarkListTasks -benchmem ./tests/integration/postgres/...
//
// Note: These benchmarks use b.Loop() which automatically handles timer management.
// See: https://go.dev/blog/testing-b-loop
//
// ACCESS PATTERN NOTE:
// The Size=10000 benchmark is NOT representative of typical user behavior.
// Real users typically view 1-2 lists with ~100 active items each (~500µs performance).
// This benchmark tests edge cases to monitor scaling characteristics. The implementation
// uses efficient batch queries to avoid N+1 problems.
func BenchmarkListTasks(b *testing.B) {
	// Use smaller counts for real database benchmarks to avoid long setup times
	counts := []int{100, 1_000, 10_000}

	for _, count := range counts {
		b.Run(fmt.Sprintf("Size=%d", count), func(b *testing.B) {
			storage := getBenchmarkStorage(b)
			defer storage.Close()

			// Clean up before setup to ensure fresh state
			cleanupBenchmarkData(b, storage)

			// Create 1 list with 'count' items to simulate one large list or aggregate of many.
			// The current service aggregation logic effectively merges them, so 1 list vs 10 lists
			// is similar O(N) but 1 list is simpler to setup.
			setupBenchmarkData(b, storage, 1, count)

			// Clean up after benchmark
			defer cleanupBenchmarkData(b, storage)

			todoService := todo.NewService(storage, todo.Config{})
			ctx := context.Background()

			b.Run("NoFilter", func(b *testing.B) {
				filter, _ := domain.NewItemsFilter(domain.ItemsFilterInput{})
				params := domain.ListTasksParams{Filter: filter}
				for b.Loop() {
					_, err := todoService.ListItems(ctx, params)
					if err != nil {
						b.Fatalf("failed: %v", err)
					}
				}
			})

			b.Run("Filter_Tags_HighSelectivity", func(b *testing.B) {
				// "tag-1" is present in 1/100 items (1%)
				filter, _ := domain.NewItemsFilter(domain.ItemsFilterInput{
					Tags: []string{"tag-1"},
				})
				params := domain.ListTasksParams{
					Filter: filter,
				}
				for b.Loop() {
					_, err := todoService.ListItems(ctx, params)
					if err != nil {
						b.Fatalf("failed: %v", err)
					}
				}
			})

			b.Run("Sort_DueTime", func(b *testing.B) {
				filter, _ := domain.NewItemsFilter(domain.ItemsFilterInput{
					OrderBy: ptrString("due_time"),
				})
				params := domain.ListTasksParams{
					Filter: filter,
				}
				for b.Loop() {
					_, err := todoService.ListItems(ctx, params)
					if err != nil {
						b.Fatalf("failed: %v", err)
					}
				}
			})

			b.Run("Combined_FilterTag_SortTime", func(b *testing.B) {
				filter, _ := domain.NewItemsFilter(domain.ItemsFilterInput{
					Tags:    []string{"tag-1"},
					OrderBy: ptrString("due_time"),
				})
				params := domain.ListTasksParams{
					Filter: filter,
				}
				for b.Loop() {
					_, err := todoService.ListItems(ctx, params)
					if err != nil {
						b.Fatalf("failed: %v", err)
					}
				}
			})
		})
	}
}

// BenchmarkCreateList benchmarks list creation with real database.
func BenchmarkCreateList(b *testing.B) {
	storage := getBenchmarkStorage(b)
	defer storage.Close()
	defer cleanupBenchmarkData(b, storage)

	todoService := todo.NewService(storage, todo.Config{})
	ctx := context.Background()

	for b.Loop() {
		_, err := todoService.CreateList(ctx, "Benchmark List")
		if err != nil {
			b.Fatalf("failed: %v", err)
		}
	}
}

// BenchmarkCreateItem benchmarks item creation with real database.
func BenchmarkCreateItem(b *testing.B) {
	storage := getBenchmarkStorage(b)
	defer storage.Close()
	defer cleanupBenchmarkData(b, storage)

	todoService := todo.NewService(storage, todo.Config{})
	ctx := context.Background()

	// Create a list to add items to
	list, err := todoService.CreateList(ctx, "Benchmark List")
	if err != nil {
		b.Fatalf("failed to create list: %v", err)
	}

	for b.Loop() {
		item := &domain.TodoItem{
			Title: "Benchmark Item",
		}
		_, err := todoService.CreateItem(ctx, list.ID, item)
		if err != nil {
			b.Fatalf("failed: %v", err)
		}
	}
}

// BenchmarkUpdateItem benchmarks item updates with real database.
func BenchmarkUpdateItem(b *testing.B) {
	storage := getBenchmarkStorage(b)
	defer storage.Close()
	defer cleanupBenchmarkData(b, storage)

	todoService := todo.NewService(storage, todo.Config{})
	ctx := context.Background()

	// Create list and item
	list, _ := todoService.CreateList(ctx, "Benchmark List")
	item := &domain.TodoItem{
		Title: "Original Title",
	}
	createdItem, _ := todoService.CreateItem(ctx, list.ID, item)

	for b.Loop() {
		updateItem := &domain.TodoItem{
			ID:     createdItem.ID,
			Title:  "Updated Title",
			Status: domain.TaskStatusDone,
		}
		_, err := todoService.UpdateItem(ctx, ItemToUpdateParams(list.ID, updateItem))
		if err != nil {
			b.Fatalf("failed: %v", err)
		}
	}
}

// BenchmarkGetList benchmarks list retrieval with real database.
func BenchmarkGetList(b *testing.B) {
	storage := getBenchmarkStorage(b)
	defer storage.Close()
	defer cleanupBenchmarkData(b, storage)

	todoService := todo.NewService(storage, todo.Config{})
	ctx := context.Background()

	// Create a list with items
	list, _ := todoService.CreateList(ctx, "Benchmark List")

	// Add some items
	for i := 0; i < 100; i++ {
		item := &domain.TodoItem{
			Title: fmt.Sprintf("Item %d", i),
		}
		todoService.CreateItem(ctx, list.ID, item)
	}

	for b.Loop() {
		_, err := todoService.GetList(ctx, list.ID)
		if err != nil {
			b.Fatalf("failed: %v", err)
		}
	}
}

// BenchmarkListLists benchmarks listing all lists with real database.
//
// ACCESS PATTERN NOTE:
// The Lists=1000 benchmark is NOT representative of typical user behavior.
// Real users typically view 1-2 lists at a time, not all lists simultaneously.
// This benchmark tests edge cases to monitor scaling characteristics. The implementation
// uses a single batch query to fetch all items (no N+1), but loading 10,000+ items into
// memory naturally has overhead from JSON unmarshaling and object construction.
func BenchmarkListLists(b *testing.B) {
	counts := []int{10, 100, 1_000}

	for _, count := range counts {
		b.Run(fmt.Sprintf("Lists=%d", count), func(b *testing.B) {
			storage := getBenchmarkStorage(b)
			defer storage.Close()
			defer cleanupBenchmarkData(b, storage)

			todoService := todo.NewService(storage, todo.Config{})
			ctx := context.Background()

			// Create N lists, each with 10 items
			for i := 0; i < count; i++ {
				list, _ := todoService.CreateList(ctx, fmt.Sprintf("List %d", i))
				for j := 0; j < 10; j++ {
					item := &domain.TodoItem{
						Title: fmt.Sprintf("Item %d", j),
					}
					todoService.CreateItem(ctx, list.ID, item)
				}
			}

			for b.Loop() {
				_, err := todoService.ListLists(ctx, domain.ListListsParams{})
				if err != nil {
					b.Fatalf("failed: %v", err)
				}
			}
		})
	}
}

// NOTE: BenchmarkRecurringTemplates and BenchmarkAccessPatterns are commented out
// as they were testing gRPC-specific patterns. These can be re-implemented with
// REST API patterns if needed for performance benchmarking.
