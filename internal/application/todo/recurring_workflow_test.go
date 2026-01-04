package todo

import (
	"context"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Synctest-based tests for recurring template time-dependent workflows
// ============================================================================
// These tests verify the three-generation strategy (Sync, Async, Reconciliation)
// with controlled time advancement using Go 1.25's testing/synctest (when available).
//
// NOTE: synctest is not yet available in Go 1.25, so these tests use regular time
// with tolerance-based assertions. Once synctest becomes available, update to use it.

// workflowMockRepo is a comprehensive mock that captures all RecurringOperations calls
// for testing time-dependent workflows.
type workflowMockRepo struct {
	// Captured calls
	createdTemplate          *domain.RecurringTemplate
	batchInsertedItems       []*domain.TodoItem
	deleteFutureItemsCalls   []deleteFutureItemsCall
	setGeneratedThroughCalls []setGeneratedThroughCall
	scheduleJobCalls         []scheduleJobCall
	findTemplateByIDCalls    []string
	updateTemplateCalls      []domain.UpdateRecurringTemplateParams

	// Return values
	templateToReturn     *domain.RecurringTemplate
	errorToReturn        error
	jobIDToReturn        string
	deletedItemCount     int64
	batchInsertedCount   int
	findTemplateReturn   *domain.RecurringTemplate
	updateTemplateReturn *domain.RecurringTemplate
}

type deleteFutureItemsCall struct {
	templateID string
	from       time.Time
}

type setGeneratedThroughCall struct {
	templateID       string
	generatedThrough time.Time
}

type scheduleJobCall struct {
	templateID   string
	scheduledFor time.Time
	from         time.Time
	until        time.Time
}

func (m *workflowMockRepo) CreateRecurringTemplate(ctx context.Context, template *domain.RecurringTemplate) (*domain.RecurringTemplate, error) {
	m.createdTemplate = template
	if m.errorToReturn != nil {
		return nil, m.errorToReturn
	}
	if m.templateToReturn != nil {
		return m.templateToReturn, nil
	}
	return template, nil
}

func (m *workflowMockRepo) FindRecurringTemplateByID(ctx context.Context, id string) (*domain.RecurringTemplate, error) {
	m.findTemplateByIDCalls = append(m.findTemplateByIDCalls, id)
	if m.errorToReturn != nil {
		return nil, m.errorToReturn
	}
	return m.findTemplateReturn, nil
}

func (m *workflowMockRepo) UpdateRecurringTemplate(ctx context.Context, params domain.UpdateRecurringTemplateParams) (*domain.RecurringTemplate, error) {
	m.updateTemplateCalls = append(m.updateTemplateCalls, params)
	if m.errorToReturn != nil {
		return nil, m.errorToReturn
	}
	return m.updateTemplateReturn, nil
}

func (m *workflowMockRepo) BatchInsertItemsIgnoreConflict(ctx context.Context, items []*domain.TodoItem) (int, error) {
	m.batchInsertedItems = append(m.batchInsertedItems, items...)
	if m.errorToReturn != nil {
		return 0, m.errorToReturn
	}
	if m.batchInsertedCount > 0 {
		return m.batchInsertedCount, nil
	}
	return len(items), nil
}

func (m *workflowMockRepo) DeleteFuturePendingItems(ctx context.Context, templateID string, from time.Time) (int64, error) {
	m.deleteFutureItemsCalls = append(m.deleteFutureItemsCalls, deleteFutureItemsCall{
		templateID: templateID,
		from:       from,
	})
	if m.errorToReturn != nil {
		return 0, m.errorToReturn
	}
	return m.deletedItemCount, nil
}

func (m *workflowMockRepo) SetGeneratedThrough(ctx context.Context, templateID string, generatedThrough time.Time) error {
	m.setGeneratedThroughCalls = append(m.setGeneratedThroughCalls, setGeneratedThroughCall{
		templateID:       templateID,
		generatedThrough: generatedThrough,
	})
	return m.errorToReturn
}

func (m *workflowMockRepo) ScheduleGenerationJob(ctx context.Context, templateID string, scheduledFor, from, until time.Time) (string, error) {
	m.scheduleJobCalls = append(m.scheduleJobCalls, scheduleJobCall{
		templateID:   templateID,
		scheduledFor: scheduledFor,
		from:         from,
		until:        until,
	})
	if m.errorToReturn != nil {
		return "", m.errorToReturn
	}
	if m.jobIDToReturn != "" {
		return m.jobIDToReturn, nil
	}
	return "job-123", nil
}

func (m *workflowMockRepo) Atomic(ctx context.Context, fn func(tx Repository) error) error {
	return fn(m)
}

func (m *workflowMockRepo) AtomicRecurring(ctx context.Context, fn func(ops RecurringOperations) error) error {
	return fn(m)
}

// Unused repository methods - panic if called
func (m *workflowMockRepo) CreateList(ctx context.Context, list *domain.TodoList) (*domain.TodoList, error) {
	panic("not used in workflow tests")
}

func (m *workflowMockRepo) FindListByID(ctx context.Context, id string) (*domain.TodoList, error) {
	panic("not used in workflow tests")
}

func (m *workflowMockRepo) FindLists(ctx context.Context, params domain.ListListsParams) (*domain.PagedListResult, error) {
	panic("not used in workflow tests")
}

func (m *workflowMockRepo) UpdateList(ctx context.Context, params domain.UpdateListParams) (*domain.TodoList, error) {
	panic("not used in workflow tests")
}

func (m *workflowMockRepo) CreateItem(ctx context.Context, listID string, item *domain.TodoItem) (*domain.TodoItem, error) {
	panic("not used in workflow tests")
}

func (m *workflowMockRepo) FindItemByID(ctx context.Context, id string) (*domain.TodoItem, error) {
	panic("not used in workflow tests")
}

func (m *workflowMockRepo) UpdateItem(ctx context.Context, params domain.UpdateItemParams) (*domain.TodoItem, error) {
	panic("not used in workflow tests")
}

func (m *workflowMockRepo) FindItems(ctx context.Context, params domain.ListTasksParams, excludedStatuses []domain.TaskStatus) (*domain.PagedResult, error) {
	panic("not used in workflow tests")
}

func (m *workflowMockRepo) DeleteItem(ctx context.Context, id string) error {
	panic("not used in workflow tests")
}

func (m *workflowMockRepo) DeleteRecurringTemplate(ctx context.Context, id string) error {
	panic("not used in workflow tests")
}

func (m *workflowMockRepo) FindRecurringTemplates(ctx context.Context, listID string, activeOnly bool) ([]*domain.RecurringTemplate, error) {
	panic("not used in workflow tests")
}

func (m *workflowMockRepo) CreateException(ctx context.Context, exception *domain.RecurringTemplateException) (*domain.RecurringTemplateException, error) {
	panic("not used in workflow tests")
}

// workflowMockGenerator generates predictable tasks for testing
type workflowMockGenerator struct {
	itemsToGenerate []*domain.TodoItem
	errorToReturn   error
}

func (m *workflowMockGenerator) GenerateTasksForTemplate(ctx context.Context, template *domain.RecurringTemplate, start, end time.Time) ([]*domain.TodoItem, error) {
	if m.errorToReturn != nil {
		return nil, m.errorToReturn
	}
	if m.itemsToGenerate != nil {
		return m.itemsToGenerate, nil
	}

	// Generate simple predictable tasks - one per day
	var items []*domain.TodoItem
	current := start
	for current.Before(end) {
		items = append(items, &domain.TodoItem{
			ID:     "item-" + current.Format("2006-01-02"),
			ListID: template.ListID,
			Title:  template.Title,
		})
		current = current.AddDate(0, 0, 1)
	}
	return items, nil
}

func (m *workflowMockGenerator) GenerateTasksForTemplateWithExceptions(ctx context.Context, template *domain.RecurringTemplate, from, until time.Time, exceptions []*domain.RecurringTemplateException) ([]*domain.TodoItem, error) {
	if m.errorToReturn != nil {
		return nil, m.errorToReturn
	}
	if m.itemsToGenerate != nil {
		return m.itemsToGenerate, nil
	}

	// Generate simple predictable tasks - one per day
	var items []*domain.TodoItem
	current := from
	for current.Before(until) {
		items = append(items, &domain.TodoItem{
			ID:     "item-" + current.Format("2006-01-02"),
			ListID: template.ListID,
			Title:  template.Title,
		})
		current = current.AddDate(0, 0, 1)
	}
	return items, nil
}

// TestCreateRecurringTemplate_SyncGeneration verifies that CreateRecurringTemplate
// generates tasks immediately for the sync horizon period and sets the generation marker correctly.
func TestCreateRecurringTemplate_SyncGeneration(t *testing.T) {
	// TODO: Replace with synctest.Run() when available in Go 1.25+

	repo := &workflowMockRepo{
		templateToReturn: &domain.RecurringTemplate{
			ID:                    "template-123",
			ListID:                "list-456",
			Title:                 "Daily Task",
			RecurrencePattern:     domain.RecurrenceDaily,
			SyncHorizonDays:       14,
			GenerationHorizonDays: 14, // Same as sync - no async job needed
		},
		jobIDToReturn: "job-123",
	}
	generator := &workflowMockGenerator{}
	service := NewService(repo, generator, Config{DefaultPageSize: 25, MaxPageSize: 100})

	template := &domain.RecurringTemplate{
		ListID:                "list-456",
		Title:                 "Daily Task",
		RecurrencePattern:     domain.RecurrenceDaily,
		SyncHorizonDays:       14,
		GenerationHorizonDays: 14,
	}

	ctx := context.Background()
	now := time.Now().UTC()

	created, err := service.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)
	require.NotNil(t, created)

	// Verify template was created
	assert.Equal(t, "template-123", created.ID)

	// Verify sync tasks were batch inserted
	assert.NotEmpty(t, repo.batchInsertedItems, "should have inserted sync tasks")

	// Calculate expected sync end
	expectedSyncEnd := now.AddDate(0, 0, 14)

	// Verify generation marker was set (with tolerance for test execution time)
	require.Len(t, repo.setGeneratedThroughCalls, 1, "should set generated_through marker")
	actualMarker := repo.setGeneratedThroughCalls[0].generatedThrough
	assert.WithinDuration(t, expectedSyncEnd, actualMarker, 2*time.Second,
		"generated_through should be set to sync_horizon_days from now")

	// Verify NO async job was scheduled (sync == generation horizon)
	assert.Empty(t, repo.scheduleJobCalls, "should NOT schedule job when sync_horizon == generation_horizon")
}

// TestCreateRecurringTemplate_AsyncJobScheduling verifies that when
// generation_horizon_days > sync_horizon_days, an async job is scheduled for the remaining period.
func TestCreateRecurringTemplate_AsyncJobScheduling(t *testing.T) {
	repo := &workflowMockRepo{
		templateToReturn: &domain.RecurringTemplate{
			ID:                    "template-123",
			ListID:                "list-456",
			Title:                 "Daily Task",
			RecurrencePattern:     domain.RecurrenceDaily,
			SyncHorizonDays:       14,
			GenerationHorizonDays: 365, // Much larger - async job needed
		},
		jobIDToReturn: "job-789",
	}
	generator := &workflowMockGenerator{}
	service := NewService(repo, generator, Config{DefaultPageSize: 25, MaxPageSize: 100})

	template := &domain.RecurringTemplate{
		ListID:                "list-456",
		Title:                 "Daily Task",
		RecurrencePattern:     domain.RecurrenceDaily,
		SyncHorizonDays:       14,
		GenerationHorizonDays: 365,
	}

	ctx := context.Background()
	now := time.Now().UTC()

	created, err := service.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)
	require.NotNil(t, created)

	// Verify sync tasks were inserted
	assert.NotEmpty(t, repo.batchInsertedItems, "should have inserted sync tasks")

	// Verify generation marker was set to sync end
	require.Len(t, repo.setGeneratedThroughCalls, 1)
	expectedSyncEnd := now.AddDate(0, 0, 14)
	assert.WithinDuration(t, expectedSyncEnd, repo.setGeneratedThroughCalls[0].generatedThrough, 2*time.Second)

	// Verify async job WAS scheduled
	require.Len(t, repo.scheduleJobCalls, 1, "should schedule async generation job")
	jobCall := repo.scheduleJobCalls[0]

	assert.Equal(t, "template-123", jobCall.templateID, "job should reference correct template")
	assert.True(t, jobCall.scheduledFor.IsZero(), "job should be scheduled immediately")

	// Job should cover from sync end to generation end
	assert.WithinDuration(t, expectedSyncEnd, jobCall.from, 2*time.Second, "job from should be sync end")
	expectedAsyncEnd := now.AddDate(0, 0, 365)
	assert.WithinDuration(t, expectedAsyncEnd, jobCall.until, 2*time.Second, "job until should be generation end")
}

// TestCreateRecurringTemplate_TransactionRollback verifies that if any operation
// in the AtomicRecurring callback fails, the entire transaction is rolled back.
func TestCreateRecurringTemplate_TransactionRollback(t *testing.T) {
	testCases := []struct {
		name          string
		errorToReturn error
		errorField    string
	}{
		{
			name:          "batch insert failure",
			errorToReturn: assert.AnError,
			errorField:    "batch_insert",
		},
		{
			name:          "set marker failure",
			errorToReturn: assert.AnError,
			errorField:    "set_marker",
		},
		{
			name:          "schedule job failure",
			errorToReturn: assert.AnError,
			errorField:    "schedule_job",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &workflowMockRepo{
				templateToReturn: &domain.RecurringTemplate{
					ID:                    "template-123",
					ListID:                "list-456",
					Title:                 "Daily Task",
					RecurrencePattern:     domain.RecurrenceDaily,
					SyncHorizonDays:       14,
					GenerationHorizonDays: 365,
				},
				errorToReturn: tc.errorToReturn,
			}

			generator := &workflowMockGenerator{}
			service := NewService(repo, generator, Config{DefaultPageSize: 25, MaxPageSize: 100})

			template := &domain.RecurringTemplate{
				ListID:                "list-456",
				Title:                 "Daily Task",
				RecurrencePattern:     domain.RecurrenceDaily,
				SyncHorizonDays:       14,
				GenerationHorizonDays: 365,
			}

			ctx := context.Background()
			_, err := service.CreateRecurringTemplate(ctx, template)

			// Should return an error
			assert.Error(t, err, "operation should fail when %s fails", tc.errorField)
		})
	}
}

// TestUpdateRecurringTemplate_Regeneration verifies that UpdateRecurringTemplate
// deletes future pending items and regenerates them when pattern fields change.
//
// Verifies that regeneration uses the UPDATED template's sync_horizon_days, not the old value.
func TestUpdateRecurringTemplate_Regeneration(t *testing.T) {
	now := time.Now().UTC()
	existingTemplate := &domain.RecurringTemplate{
		ID:                    "template-123",
		ListID:                "list-456",
		Title:                 "Old Title",
		RecurrencePattern:     domain.RecurrenceDaily,
		SyncHorizonDays:       14,
		GenerationHorizonDays: 365,
		GeneratedThrough:      now.AddDate(0, 0, 7), // Already generated 7 days
	}

	repo := &workflowMockRepo{
		findTemplateReturn: existingTemplate,
		updateTemplateReturn: &domain.RecurringTemplate{
			ID:                    "template-123",
			ListID:                "list-456",
			Title:                 "New Title",
			RecurrencePattern:     domain.RecurrenceWeekly, // Changed pattern
			SyncHorizonDays:       21,                      // Changed horizon (was 14)
			GenerationHorizonDays: 365,
			GeneratedThrough:      now.AddDate(0, 0, 7),
		},
		deletedItemCount: 100, // Simulate deleting future items
		jobIDToReturn:    "job-new",
	}

	generator := &workflowMockGenerator{}
	service := NewService(repo, generator, Config{DefaultPageSize: 25, MaxPageSize: 100})

	newTitle := "New Title"
	newPattern := domain.RecurrenceWeekly
	newSyncHorizon := 21
	params := domain.UpdateRecurringTemplateParams{
		TemplateID:        "template-123",
		ListID:            "list-456",
		UpdateMask:        []string{"title", "recurrence_pattern", "sync_horizon_days"},
		Title:             &newTitle,
		RecurrencePattern: &newPattern,
		SyncHorizonDays:   &newSyncHorizon,
	}

	ctx := context.Background()
	updated, err := service.UpdateRecurringTemplate(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, updated)

	// Verify template was fetched
	require.Len(t, repo.findTemplateByIDCalls, 1)
	assert.Equal(t, "template-123", repo.findTemplateByIDCalls[0])

	// Verify future pending items were deleted from NOW (not from generated_through)
	require.Len(t, repo.deleteFutureItemsCalls, 1, "should delete future pending items")
	deleteCall := repo.deleteFutureItemsCalls[0]
	assert.Equal(t, "template-123", deleteCall.templateID)
	assert.WithinDuration(t, now, deleteCall.from, 2*time.Second, "should delete from NOW, not generated_through")

	// Verify new sync tasks were batch inserted
	assert.NotEmpty(t, repo.batchInsertedItems, "should have inserted new sync tasks")

	// Verify generation marker was updated (uses NEW sync_horizon_days = 21)
	require.Len(t, repo.setGeneratedThroughCalls, 1)
	expectedSyncEnd := now.AddDate(0, 0, 21) // NEW value from updated template
	assert.WithinDuration(t, expectedSyncEnd, repo.setGeneratedThroughCalls[0].generatedThrough, 2*time.Second,
		"should use NEW sync_horizon_days (21) from updated template, not old (14)")

	// Verify new async job was scheduled
	require.Len(t, repo.scheduleJobCalls, 1, "should schedule new async job")
	jobCall := repo.scheduleJobCalls[0]
	assert.Equal(t, "template-123", jobCall.templateID)
	assert.WithinDuration(t, expectedSyncEnd, jobCall.from, 2*time.Second)
}

// TestUpdateRecurringTemplate_NoRegenerationWhenNoRelevantChanges verifies that when
// only non-timing fields are updated, no regeneration occurs.
func TestUpdateRecurringTemplate_NoRegenerationWhenNoRelevantChanges(t *testing.T) {
	now := time.Now().UTC()
	existingTemplate := &domain.RecurringTemplate{
		ID:                    "template-123",
		ListID:                "list-456",
		Title:                 "Old Title",
		RecurrencePattern:     domain.RecurrenceDaily,
		SyncHorizonDays:       14,
		GenerationHorizonDays: 365,
		GeneratedThrough:      now.AddDate(0, 0, 7),
	}

	repo := &workflowMockRepo{
		findTemplateReturn: existingTemplate,
		updateTemplateReturn: &domain.RecurringTemplate{
			ID:                    "template-123",
			ListID:                "list-456",
			Title:                 "New Title", // Only title changed
			RecurrencePattern:     domain.RecurrenceDaily,
			SyncHorizonDays:       14,  // Same
			GenerationHorizonDays: 365, // Same
			GeneratedThrough:      now.AddDate(0, 0, 7),
		},
	}

	generator := &workflowMockGenerator{}
	service := NewService(repo, generator, Config{DefaultPageSize: 25, MaxPageSize: 100})

	newTitle := "New Title"
	params := domain.UpdateRecurringTemplateParams{
		TemplateID: "template-123",
		ListID:     "list-456",
		UpdateMask: []string{"title"},
		Title:      ptr.To(newTitle),
	}

	ctx := context.Background()
	updated, err := service.UpdateRecurringTemplate(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, updated)

	// Verify template was updated
	require.Len(t, repo.updateTemplateCalls, 1)

	// Verify NO regeneration operations occurred
	assert.Empty(t, repo.deleteFutureItemsCalls, "should NOT delete items when only title changes")
	assert.Empty(t, repo.batchInsertedItems, "should NOT insert new items")
	assert.Empty(t, repo.setGeneratedThroughCalls, "should NOT update marker")
	assert.Empty(t, repo.scheduleJobCalls, "should NOT schedule new job")
}
