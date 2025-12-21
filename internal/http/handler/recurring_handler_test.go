package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/oapi-codegen/runtime/types"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
)

// stubRepository implements todo.Repository and panics on calls we don't expect.
type stubRepository struct{}

func (s *stubRepository) CreateList(ctx context.Context, list *domain.TodoList) error {
	panic("not implemented")
}
func (s *stubRepository) FindListByID(ctx context.Context, id string) (*domain.TodoList, error) {
	panic("not implemented")
}
func (s *stubRepository) FindAllLists(ctx context.Context) ([]*domain.TodoList, error) {
	panic("not implemented")
}
func (s *stubRepository) UpdateList(ctx context.Context, list *domain.TodoList) error {
	panic("not implemented")
}
func (s *stubRepository) CreateItem(ctx context.Context, listID string, item *domain.TodoItem) error {
	panic("not implemented")
}
func (s *stubRepository) FindItemByID(ctx context.Context, id string) (*domain.TodoItem, error) {
	panic("not implemented")
}
func (s *stubRepository) UpdateItem(ctx context.Context, listID string, item *domain.TodoItem) error {
	panic("not implemented")
}
func (s *stubRepository) FindItems(ctx context.Context, params domain.ListTasksParams) (*domain.PagedResult, error) {
	panic("not implemented")
}
func (s *stubRepository) CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) error {
	panic("not implemented")
}
func (s *stubRepository) FindRecurringTemplate(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	panic("should not be called")
}
func (s *stubRepository) UpdateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) error {
	panic("not implemented")
}
func (s *stubRepository) DeleteRecurringTemplate(ctx context.Context, id string) error {
	panic("not implemented")
}
func (s *stubRepository) FindRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*domain.RecurringTemplate, error) {
	panic("not implemented")
}

func TestUpdateRecurringTemplate_MissingTemplateReturnsBadRequest(t *testing.T) {
	repo := &stubRepository{}
	service := todo.NewService(repo, todo.Config{})
	srv := NewServer(service)

	id := types.UUID(uuid.New())
	req := httptest.NewRequest(http.MethodPatch, "/v1/recurring-templates/"+id.String(), bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	// Expect handler to validate payload before calling service; otherwise stub panics
	srv.UpdateRecurringTemplate(w, req, id)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
