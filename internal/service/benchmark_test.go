package service_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/rezkam/mono/internal/service"
)

// getBenchmarkStorage creates a real PostgreSQL storage connection for benchmarking.
// Skips the benchmark if BENCHMARK_POSTGRES_URL is not set.
func getBenchmarkStorage(b *testing.B) *postgres.Store {
	b.Helper()

	dbURL := os.Getenv("BENCHMARK_POSTGRES_URL")
	if dbURL == "" {
		b.Skip("BENCHMARK_POSTGRES_URL not set, skipping benchmark")
	}

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, dbURL)
	if err != nil {
		b.Fatalf("failed to create storage: %v", err)
	}

	return store
}

// cleanupBenchmarkData removes all lists and items from the database.
func cleanupBenchmarkData(b *testing.B, storage todo.Repository) {
	b.Helper()

	ctx := context.Background()
	lists, err := storage.FindAllLists(ctx)
	if err != nil {
		b.Logf("failed to list lists for cleanup: %v", err)
		return
	}

	// Delete all recurring templates first (they reference lists)
	for _, list := range lists {
		templates, err := storage.FindRecurringTemplates(ctx, list.ID, false)
		if err == nil {
			for _, tmpl := range templates {
				storage.DeleteRecurringTemplate(ctx, tmpl.ID)
			}
		}
	}

	// Delete all lists (this will cascade to items)
	for _, list := range lists {
		// Set items to empty and update to effectively delete them
		// NOTE: UpdateList deletes all items (and their status history) before recreating.
		// This is acceptable for benchmark cleanup where history loss is expected.
		list.Items = []domain.TodoItem{}
		if err := storage.UpdateList(ctx, list); err != nil {
			b.Logf("failed to clear list %s: %v", list.ID, err)
		}
	}
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
		items := make([]domain.TodoItem, itemsPerList)

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
			items[j] = domain.TodoItem{
				ID:         itemUUID.String(),
				Title:      fmt.Sprintf("Large Payload Item %d-%d with description and metadata to simulate real object size", i, j),
				Status:     status,
				CreateTime: now.Add(time.Duration(j) * time.Minute),
				UpdatedAt:  now.Add(time.Duration(j) * time.Minute),
				DueTime:    &due,
				Tags:       []string{"common", fmt.Sprintf("tag-%d", j%100)}, // 100 unique tags
			}
		}

		list := &domain.TodoList{
			ID:         listID,
			Title:      fmt.Sprintf("Benchmark List %d", i),
			Items:      items,
			CreateTime: now,
		}

		if err := storage.CreateList(ctx, list); err != nil {
			b.Fatalf("failed to create list %d: %v", i, err)
		}
	}
}

// BenchmarkListTasks tests ListTasks performance at different scales using real PostgreSQL.
// Set BENCHMARK_POSTGRES_URL environment variable to run these benchmarks.
//
// Example:
//
//	BENCHMARK_POSTGRES_URL="postgres://postgres:postgres@localhost:5433/mono_test?sslmode=disable" go test -bench=BenchmarkListTasks -benchmem ./internal/service/...
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

			todoService := todo.NewService(storage)
			svc := service.NewMonoService(todoService, 50, 100)
			ctx := context.Background()

			b.Run("NoFilter", func(b *testing.B) {
				req := &monov1.ListTasksRequest{}
				for b.Loop() {
					_, err := svc.ListTasks(ctx, req)
					if err != nil {
						b.Fatalf("failed: %v", err)
					}
				}
			})

			b.Run("Filter_Tags_HighSelectivity", func(b *testing.B) {
				// "tag-1" is present in 1/100 items (1%)
				req := &monov1.ListTasksRequest{
					Filter: "tags:tag-1",
				}
				for b.Loop() {
					_, err := svc.ListTasks(ctx, req)
					if err != nil {
						b.Fatalf("failed: %v", err)
					}
				}
			})

			b.Run("Sort_DueTime", func(b *testing.B) {
				req := &monov1.ListTasksRequest{
					OrderBy: "due_time",
				}
				for b.Loop() {
					_, err := svc.ListTasks(ctx, req)
					if err != nil {
						b.Fatalf("failed: %v", err)
					}
				}
			})

			b.Run("Combined_FilterTag_SortTime", func(b *testing.B) {
				req := &monov1.ListTasksRequest{
					Filter:  "tags:tag-1",
					OrderBy: "due_time",
				}
				for b.Loop() {
					_, err := svc.ListTasks(ctx, req)
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

	todoService := todo.NewService(storage)
	svc := service.NewMonoService(todoService, 50, 100)
	ctx := context.Background()

	for b.Loop() {
		_, err := svc.CreateList(ctx, &monov1.CreateListRequest{
			Title: "Benchmark List",
		})
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

	todoService := todo.NewService(storage)
	svc := service.NewMonoService(todoService, 50, 100)
	ctx := context.Background()

	// Create a list to add items to
	listResp, err := svc.CreateList(ctx, &monov1.CreateListRequest{
		Title: "Benchmark List",
	})
	if err != nil {
		b.Fatalf("failed to create list: %v", err)
	}

	for b.Loop() {
		_, err := svc.CreateItem(ctx, &monov1.CreateItemRequest{
			ListId: listResp.List.Id,
			Title:  "Benchmark Item",
		})
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

	todoService := todo.NewService(storage)
	svc := service.NewMonoService(todoService, 50, 100)
	ctx := context.Background()

	// Create list and item
	listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{
		Title: "Benchmark List",
	})
	itemResp, _ := svc.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId: listResp.List.Id,
		Title:  "Original Title",
	})

	for b.Loop() {
		_, err := svc.UpdateItem(ctx, &monov1.UpdateItemRequest{
			ListId: listResp.List.Id,
			Item: &monov1.TodoItem{
				Id:     itemResp.Item.Id,
				Title:  "Updated Title",
				Status: monov1.TaskStatus_TASK_STATUS_DONE,
			},
		})
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

	todoService := todo.NewService(storage)
	svc := service.NewMonoService(todoService, 50, 100)
	ctx := context.Background()

	// Create a list with items
	listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{
		Title: "Benchmark List",
	})

	// Add some items
	for i := 0; i < 100; i++ {
		svc.CreateItem(ctx, &monov1.CreateItemRequest{
			ListId: listResp.List.Id,
			Title:  fmt.Sprintf("Item %d", i),
		})
	}

	for b.Loop() {
		_, err := svc.GetList(ctx, &monov1.GetListRequest{
			Id: listResp.List.Id,
		})
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

			todoService := todo.NewService(storage)
			svc := service.NewMonoService(todoService, 50, 100)
			ctx := context.Background()

			// Create N lists, each with 10 items
			for i := 0; i < count; i++ {
				listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{
					Title: fmt.Sprintf("List %d", i),
				})
				for j := 0; j < 10; j++ {
					svc.CreateItem(ctx, &monov1.CreateItemRequest{
						ListId: listResp.List.Id,
						Title:  fmt.Sprintf("Item %d", j),
					})
				}
			}

			for b.Loop() {
				_, err := svc.ListLists(ctx, &monov1.ListListsRequest{})
				if err != nil {
					b.Fatalf("failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkRecurringTemplates benchmarks recurring template operations.
func BenchmarkRecurringTemplates(b *testing.B) {
	storage := getBenchmarkStorage(b)
	defer storage.Close()
	defer cleanupBenchmarkData(b, storage)

	todoService := todo.NewService(storage)
	svc := service.NewMonoService(todoService, 50, 100)
	ctx := context.Background()

	// Create a list for templates
	listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{
		Title: "Template List",
	})

	b.Run("CreateTemplate", func(b *testing.B) {
		for b.Loop() {
			_, err := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
				ListId:            listResp.List.Id,
				Title:             "Daily Task",
				RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
			})
			if err != nil {
				b.Fatalf("failed: %v", err)
			}
		}
	})

	// Create a template for get/update benchmarks
	tmplResp, _ := svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
		ListId:            listResp.List.Id,
		Title:             "Benchmark Template",
		RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKLY,
	})

	b.Run("GetTemplate", func(b *testing.B) {
		for b.Loop() {
			_, err := svc.GetRecurringTemplate(ctx, &monov1.GetRecurringTemplateRequest{
				Id: tmplResp.Template.Id,
			})
			if err != nil {
				b.Fatalf("failed: %v", err)
			}
		}
	})

	b.Run("UpdateTemplate", func(b *testing.B) {
		for b.Loop() {
			_, err := svc.UpdateRecurringTemplate(ctx, &monov1.UpdateRecurringTemplateRequest{
				Template: &monov1.RecurringTaskTemplate{
					Id:                tmplResp.Template.Id,
					ListId:            listResp.List.Id,
					Title:             "Updated Template",
					RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
				},
			})
			if err != nil {
				b.Fatalf("failed: %v", err)
			}
		}
	})

	b.Run("ListTemplates", func(b *testing.B) {
		// Create some templates
		for i := 0; i < 10; i++ {
			svc.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
				ListId:            listResp.List.Id,
				Title:             fmt.Sprintf("Template %d", i),
				RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
			})
		}

		for b.Loop() {
			_, err := svc.ListRecurringTemplates(ctx, &monov1.ListRecurringTemplatesRequest{
				ListId: listResp.List.Id,
			})
			if err != nil {
				b.Fatalf("failed: %v", err)
			}
		}
	})
}

// BenchmarkAccessPatterns benchmarks real-world access patterns to verify optimizations.
//
// ACCESS PATTERN DOCUMENTATION:
// This benchmark suite validates the performance characteristics of common user operations:
//
// 1. LIST VIEW (Dashboard):
//   - User opens app, sees all lists with counts
//   - Expected: O(lists) with single SQL aggregation query
//   - Scales: Independent of total item count
//
// 2. SEARCH/FILTER (Find Tasks):
//   - User searches "show me overdue high-priority tasks"
//   - Expected: O(matching_results) using database indexes
//   - Scales: Pagination keeps memory constant
//
// 3. DETAIL VIEW (Single List):
//   - User clicks a list to see all items
//   - Expected: O(items_in_list) loading only one list's items
//   - Scales: Per-list, not affected by other lists
//
// Run with real database:
//
//	BENCHMARK_POSTGRES_URL="postgres://..." go test -bench=BenchmarkAccessPatterns -benchmem
func BenchmarkAccessPatterns(b *testing.B) {
	storage := getBenchmarkStorage(b)
	defer storage.Close()
	defer cleanupBenchmarkData(b, storage)

	todoService := todo.NewService(storage)
	svc := service.NewMonoService(todoService, 50, 100)
	ctx := context.Background()

	// Setup: Create realistic dataset
	// 10 lists with varying item counts (10-100 items each)
	// Total: ~550 items across all lists
	listIDs := make([]string, 10)
	for i := 0; i < 10; i++ {
		listResp, _ := svc.CreateList(ctx, &monov1.CreateListRequest{
			Title: fmt.Sprintf("List %d", i),
		})
		listIDs[i] = listResp.List.Id

		// Create items (10 * i items per list, so list 0 has 10, list 9 has 90)
		itemCount := 10 + (i * 10)
		for j := 0; j < itemCount; j++ {
			priority := monov1.TaskPriority_TASK_PRIORITY_MEDIUM

			// Make some items high priority
			if j%3 == 0 {
				priority = monov1.TaskPriority_TASK_PRIORITY_HIGH
			}

			svc.CreateItem(ctx, &monov1.CreateItemRequest{
				ListId:   listResp.List.Id,
				Title:    fmt.Sprintf("Item %d-%d", i, j),
				Priority: priority,
			})

			// Update some items to done status
			if j%4 == 0 {
				items, _ := svc.GetList(ctx, &monov1.GetListRequest{Id: listResp.List.Id})
				if len(items.List.Items) > 0 {
					lastItem := items.List.Items[len(items.List.Items)-1]
					svc.UpdateItem(ctx, &monov1.UpdateItemRequest{
						Item: &monov1.TodoItem{
							Id:     lastItem.Id,
							Status: monov1.TaskStatus_TASK_STATUS_DONE,
						},
					})
				}
			}
		}
	}

	b.ResetTimer()

	// ACCESS PATTERN 1: LIST VIEW (Dashboard showing all lists with counts)
	// Expected: Fast regardless of total item count (uses SQL aggregation)
	b.Run("AccessPattern_ListView_DashboardWithCounts", func(b *testing.B) {
		b.ReportMetric(float64(len(listIDs)), "lists")
		b.ReportMetric(550, "total_items")

		for b.Loop() {
			resp, err := svc.ListLists(ctx, &monov1.ListListsRequest{})
			if err != nil {
				b.Fatalf("failed: %v", err)
			}

			// Verify counts are populated (not zero)
			if len(resp.Lists) > 0 && resp.Lists[0].TotalItems == 0 {
				b.Fatal("counts not populated - optimization not working")
			}
		}
	})

	// ACCESS PATTERN 2: SEARCH/FILTER (Find specific tasks across lists)
	b.Run("AccessPattern_Search_HighPriorityTasks", func(b *testing.B) {
		b.ReportMetric(550, "total_items_searched")

		for b.Loop() {
			resp, err := svc.ListTasks(ctx, &monov1.ListTasksRequest{
				Filter:   "priority:HIGH",
				PageSize: 20,
			})
			if err != nil {
				b.Fatalf("failed: %v", err)
			}

			// Should return filtered results with pagination
			if len(resp.Items) > 20 {
				b.Fatal("pagination not working - returned too many items")
			}
		}
	})

	// ACCESS PATTERN 3: SEARCH/FILTER (Paginated results)
	b.Run("AccessPattern_Search_PaginatedResults", func(b *testing.B) {
		b.ReportMetric(550, "total_items")
		b.ReportMetric(50, "page_size")

		for b.Loop() {
			// First page
			resp, err := svc.ListTasks(ctx, &monov1.ListTasksRequest{
				PageSize: 50,
			})
			if err != nil {
				b.Fatalf("failed: %v", err)
			}

			// Should have next page token
			if resp.NextPageToken == "" && len(resp.Items) >= 50 {
				b.Fatal("pagination token not generated")
			}
		}
	})

	// ACCESS PATTERN 4: DETAIL VIEW (Single list with all items)
	b.Run("AccessPattern_DetailView_SingleListFullLoad", func(b *testing.B) {
		targetList := listIDs[5] // List with ~60 items
		b.ReportMetric(60, "items_in_list")

		for b.Loop() {
			resp, err := svc.GetList(ctx, &monov1.GetListRequest{
				Id: targetList,
			})
			if err != nil {
				b.Fatalf("failed: %v", err)
			}

			// Should load all items for this specific list
			if len(resp.List.Items) == 0 {
				b.Fatal("items not loaded in detail view")
			}
		}
	})

	// ACCESS PATTERN 5: FILTER BY LIST (Tasks in specific list)
	b.Run("AccessPattern_FilterByList_SingleListTasks", func(b *testing.B) {
		targetList := listIDs[8] // List with ~90 items
		b.ReportMetric(90, "items_in_list")
		b.ReportMetric(25, "page_size")

		for b.Loop() {
			resp, err := svc.ListTasks(ctx, &monov1.ListTasksRequest{
				Parent:   targetList,
				PageSize: 25,
			})
			if err != nil {
				b.Fatalf("failed: %v", err)
			}

			// Should return paginated results from single list
			if len(resp.Items) > 25 {
				b.Fatal("pagination not working for list filter")
			}
		}
	})
}
