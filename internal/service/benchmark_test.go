package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	monov1 "github.com/rezkam/mono/api/proto/monov1"
	"github.com/rezkam/mono/internal/core"
	"github.com/rezkam/mono/internal/service"
)

// MockStorage is a simple in-memory storage for benchmarking.
type MockStorage struct {
	lists map[string]*core.TodoList
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		lists: make(map[string]*core.TodoList),
	}
}

func (m *MockStorage) CreateList(ctx context.Context, list *core.TodoList) error {
	m.lists[list.ID] = list
	return nil
}

func (m *MockStorage) GetList(ctx context.Context, id string) (*core.TodoList, error) {
	if l, ok := m.lists[id]; ok {
		return l, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *MockStorage) UpdateList(ctx context.Context, list *core.TodoList) error {
	m.lists[list.ID] = list
	return nil
}

func (m *MockStorage) ListLists(ctx context.Context) ([]*core.TodoList, error) {
	var results []*core.TodoList
	for _, l := range m.lists {
		results = append(results, l)
	}
	return results, nil
}

// setupBenchmarkData populates the mock storage with N lists, each having M items.
func setupBenchmarkData(storage *MockStorage, numLists, itemsPerList int) {
	ctx := context.Background()
	// Pre-allocate to reduce setup noise
	for i := 0; i < numLists; i++ {
		listID := fmt.Sprintf("list-%d", i)
		items := make([]core.TodoItem, itemsPerList)
		now := time.Now().UTC()
		for j := 0; j < itemsPerList; j++ {
			items[j] = core.TodoItem{
				ID:         fmt.Sprintf("item-%d-%d", i, j),
				Title:      fmt.Sprintf("Large Payload Item %d-%d with description and metadata to simulate real object size", i, j),
				Completed:  j%2 == 0,
				CreateTime: now.Add(time.Duration(j) * time.Minute), // Ascending time
				DueTime:    now.Add(time.Duration(j+100) * time.Minute),
				Tags:       []string{"common", fmt.Sprintf("tag-%d", j%100)}, // 100 unique tags
			}
		}
		list := &core.TodoList{
			ID:         listID,
			Title:      fmt.Sprintf("List %d", i),
			Items:      items,
			CreateTime: now,
		}
		storage.CreateList(ctx, list)
	}
}

// BenchmarkListTasks tests ListTasks performance at different scales using b.Loop().
// Note: b.Loop() automatically handles timer management (resets on first call,
// stops on final iteration), so manual b.ResetTimer() calls are not needed.
// See: https://go.dev/blog/testing-b-loop
func BenchmarkListTasks(b *testing.B) {
	counts := []int{10_000, 100_000, 1_000_000}

	for _, count := range counts {
		b.Run(fmt.Sprintf("Size=%d", count), func(b *testing.B) {
			storage := NewMockStorage()
			// Create 1 list with 'count' items to simulate one large list or aggregate of many.
			// The current service aggregation logic effectively merges them, so 1 list vs 10 lists
			// is similar O(N) but 1 list is simpler to setup.
			setupBenchmarkData(storage, 1, count)
			svc := service.NewMonoService(storage)
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

			// Note: Our current implementation doesn't support complex sorting in ListTasks fully yet
			// (it was a TODO in the code), but we benchmark the overhead of the request logic that usually implies it.
			// If we add sorting logic later, these baselines will be useful.
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
