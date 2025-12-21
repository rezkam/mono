package integration

import (
	"context"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/require"
)

// listTasksTestEnv bundles the common wiring required for the ListTasks suites.
type listTasksTestEnv struct {
	ctx     context.Context
	store   *postgres.Store
	service *todo.Service
	cleanup func()
}

func newListTasksTestEnv(t *testing.T) *listTasksTestEnv {
	t.Helper()

	_, dbCleanup := SetupTestDB(t)

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)

	todoService := todo.NewService(store, todo.Config{})

	env := &listTasksTestEnv{
		ctx:     ctx,
		store:   store,
		service: todoService,
		cleanup: func() {
			store.Close()
			dbCleanup()
		},
	}

	t.Cleanup(env.Cleanup)
	return env
}

func (e *listTasksTestEnv) Cleanup() {
	if e.cleanup != nil {
		e.cleanup()
		e.cleanup = nil
	}
}

func (e *listTasksTestEnv) Context() context.Context {
	return e.ctx
}

func (e *listTasksTestEnv) Service() *todo.Service {
	return e.service
}

func (e *listTasksTestEnv) Store() *postgres.Store {
	return e.store
}

func getTestDSN(t *testing.T) string {
	return GetTestStorageDSN(t)
}

func ptrTaskPriority(p domain.TaskPriority) *domain.TaskPriority {
	return &p
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
