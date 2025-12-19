package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/rezkam/mono/internal/service"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// listTasksTestEnv bundles the common wiring required for the ListTasks suites.
type listTasksTestEnv struct {
	ctx     context.Context
	store   *postgres.Store
	service *service.MonoService
	cleanup func()
}

func newListTasksTestEnv(t *testing.T) *listTasksTestEnv {
	t.Helper()

	_, dbCleanup := setupTestDB(t)

	ctx := context.Background()
	pgURL := getTestDSN(t)

	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)

	todoService := todo.NewService(store)
	svc := service.NewMonoService(todoService, 50, 100)

	env := &listTasksTestEnv{
		ctx:     ctx,
		store:   store,
		service: svc,
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

func (e *listTasksTestEnv) Service() *service.MonoService {
	return e.service
}

func (e *listTasksTestEnv) Store() *postgres.Store {
	return e.store
}

func ptrTaskPriority(p domain.TaskPriority) *domain.TaskPriority {
	return &p
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func timestampProto(t time.Time) *timestamppb.Timestamp {
	return timestamppb.New(t)
}
