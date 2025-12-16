package fs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rezkam/mono/internal/core"
)

// Store is a filesystem-based implementation of core.Storage.
type Store struct {
	baseDir string
	mu      sync.RWMutex
}

// NewStore creates a new filesystem store.
func NewStore(baseDir string) (*Store, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}
	return &Store{baseDir: baseDir}, nil
}

func (s *Store) getFilePath(id string) string {
	return filepath.Join(s.baseDir, fmt.Sprintf("%s.json", id))
}

// CreateList creates a new TodoList as a JSON file.
func (s *Store) CreateList(ctx context.Context, list *core.TodoList) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.getFilePath(list.ID)
	// Check if file already exists to avoid overwrite (optional, depending on semantics)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("list with ID %s already exists", list.ID)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal list: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// GetList retrieves a TodoList by reading its JSON file.
func (s *Store) GetList(ctx context.Context, id string) (*core.TodoList, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.getFilePath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("list not found: %s", id)
		}
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var list core.TodoList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("failed to unmarshal list: %w", err)
	}

	return &list, nil
}

// UpdateList overwrites an existing TodoList JSON file.
func (s *Store) UpdateList(ctx context.Context, list *core.TodoList) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.getFilePath(list.ID)
	// Ensure it exists first
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("list not found: %s", list.ID)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal list: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// ListLists scans the directory for JSON files and loads them in parallel.
func (s *Store) ListLists(ctx context.Context) ([]*core.TodoList, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var mu sync.Mutex
	var lists []*core.TodoList
	var wg sync.WaitGroup

	// Limit concurrency to avoid "too many open files" on large directories
	// and to prevent overwhelming the CPU/Scheduler.
	const maxConcurrency = 20
	semaphore := make(chan struct{}, maxConcurrency)

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			wg.Add(1)
			semaphore <- struct{}{} // Acquire token

			go func(filename string) {
				defer wg.Done()
				defer func() { <-semaphore }() // Release token

				path := filepath.Join(s.baseDir, filename)
				data, err := os.ReadFile(path)
				if err != nil {
					// In a read-heavy system, one bad file shouldn't crash the whole list?
					// For now, we log/ignore or return error?
					// Let's ignore unreadable files to be robust.
					return
				}

				var list core.TodoList
				if err := json.Unmarshal(data, &list); err == nil {
					mu.Lock()
					lists = append(lists, &list)
					mu.Unlock()
				}
			}(entry.Name())
		}
	}

	wg.Wait()
	return lists, nil
}
