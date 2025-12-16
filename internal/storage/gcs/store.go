package gcs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"cloud.google.com/go/storage"
	"github.com/rezkam/mono/internal/core"
	"google.golang.org/api/iterator"
)

// Store is a GCS-based implementation of core.Storage.
type Store struct {
	client *storage.Client
	bucket string
}

// NewStore creates a new GCS store.
// It assumes the client is authenticated (e.g. via GOOGLE_APPLICATION_CREDENTIALS).
func NewStore(ctx context.Context, bucketName string) (*Store, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}
	return &Store{
		client: client,
		bucket: bucketName,
	}, nil
}

func (s *Store) objectName(id string) string {
	return fmt.Sprintf("%s.json", id)
}

// CreateList creates a new TodoList as a JSON object in GCS.
func (s *Store) CreateList(ctx context.Context, list *core.TodoList) error {
	name := s.objectName(list.ID)
	obj := s.client.Bucket(s.bucket).Object(name)

	// Check if exists (optional - we could remove this check for performance)
	_, err := obj.Attrs(ctx)
	if err == nil {
		return fmt.Errorf("list with ID %s already exists", list.ID)
	}
	// Use errors.Is to handle wrapped errors from GCS client
	if !errors.Is(err, storage.ErrObjectNotExist) {
		// Some other error occurred (permissions, network, etc)
		return fmt.Errorf("failed to check object existence: %w", err)
	}
	// If err is ErrObjectNotExist, that's expected - continue creating

	data, err := json.Marshal(list)
	if err != nil {
		return fmt.Errorf("failed to marshal list: %w", err)
	}

	w := obj.NewWriter(ctx)
	if _, err := w.Write(data); err != nil {
		w.Close()
		return fmt.Errorf("failed to write object: %w", err)
	}
	return w.Close()
}

// GetList retrieves a TodoList from GCS.
func (s *Store) GetList(ctx context.Context, id string) (*core.TodoList, error) {
	name := s.objectName(id)
	obj := s.client.Bucket(s.bucket).Object(name)

	r, err := obj.NewReader(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return nil, fmt.Errorf("list not found: %s", id)
		}
		return nil, fmt.Errorf("failed to read object: %w", err)
	}
	defer r.Close()

	var list core.TodoList
	if err := json.NewDecoder(r).Decode(&list); err != nil {
		return nil, fmt.Errorf("failed to decode list: %w", err)
	}
	return &list, nil
}

// UpdateList overwrites an existing TodoList in GCS.
func (s *Store) UpdateList(ctx context.Context, list *core.TodoList) error {
	name := s.objectName(list.ID)
	obj := s.client.Bucket(s.bucket).Object(name)

	// Ensure exists first (optional but consistent with interface)
	_, err := obj.Attrs(ctx)
	if err == storage.ErrObjectNotExist {
		return fmt.Errorf("list not found: %s", list.ID)
	}

	data, err := json.Marshal(list)
	if err != nil {
		return fmt.Errorf("failed to marshal list: %w", err)
	}

	w := obj.NewWriter(ctx)
	if _, err := w.Write(data); err != nil {
		w.Close()
		return fmt.Errorf("failed to write object: %w", err)
	}
	return w.Close()
}

// ListLists scans the bucket for JSON objects and loads them in parallel.
func (s *Store) ListLists(ctx context.Context) ([]*core.TodoList, error) {
	it := s.client.Bucket(s.bucket).Objects(ctx, nil)

	// First, collect all object names
	var objectNames []string
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}
		if strings.HasSuffix(attrs.Name, ".json") {
			objectNames = append(objectNames, attrs.Name)
		}
	}

	// Then, fetch objects in parallel
	var mu sync.Mutex
	var lists []*core.TodoList
	var wg sync.WaitGroup

	// Limit concurrency to avoid overwhelming GCS and local resources.
	// GCS handles 20+ concurrent requests well, but we stay conservative.
	const maxConcurrency = 20
	semaphore := make(chan struct{}, maxConcurrency)

	for _, name := range objectNames {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire token

		go func(objectName string) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release token

			obj := s.client.Bucket(s.bucket).Object(objectName)
			r, err := obj.NewReader(ctx)
			if err != nil {
				// Skip unreadable objects
				return
			}
			defer r.Close()

			data, err := io.ReadAll(r)
			if err != nil {
				return
			}

			var list core.TodoList
			if err := json.Unmarshal(data, &list); err == nil {
				mu.Lock()
				lists = append(lists, &list)
				mu.Unlock()
			}
		}(name)
	}

	wg.Wait()
	return lists, nil
}
