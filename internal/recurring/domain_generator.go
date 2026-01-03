package recurring

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
)

// DomainGenerator handles the generation of task instances from recurring templates using domain types.
type DomainGenerator struct{}

// NewDomainGenerator creates a new task generator that works with domain types.
func NewDomainGenerator() *DomainGenerator {
	return &DomainGenerator{}
}

// GenerateTasksForTemplateWithExceptions generates tasks while filtering exception dates.
func (g *DomainGenerator) GenerateTasksForTemplateWithExceptions(
	ctx context.Context,
	template *domain.RecurringTemplate,
	start, end time.Time,
	exceptions []*domain.RecurringTemplateException,
) ([]*domain.TodoItem, error) {
	// Build exception map for O(1) lookup
	exceptionTimes := make(map[time.Time]bool)
	for _, exc := range exceptions {
		// Normalize to UTC for comparison
		normalized := exc.OccursAt.UTC()
		exceptionTimes[normalized] = true
	}

	calculator := GetCalculator(template.RecurrencePattern)
	if calculator == nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrInvalidRecurrencePattern, template.RecurrencePattern)
	}

	// Parse recurrence config
	config := template.RecurrenceConfig
	if config == nil {
		config = make(map[string]any)
	}

	// Calculate all occurrences in the range
	occurrences := calculator.OccurrencesBetween(start, end, config)

	// Filter out exceptions and create tasks
	tasks := make([]*domain.TodoItem, 0, len(occurrences))
	for _, occurrence := range occurrences {
		normalizedOccurrence := occurrence.UTC()

		// Skip if this date is an exception
		if exceptionTimes[normalizedOccurrence] {
			continue
		}

		task, err := g.createTaskInstance(template, occurrence)
		if err != nil {
			return nil, fmt.Errorf("failed to create task instance for %s: %w", occurrence.Format(time.RFC3339), err)
		}
		tasks = append(tasks, &task)
	}

	return tasks, nil
}

// createTaskInstance creates a single task instance from a template for a specific occurrence.
func (g *DomainGenerator) createTaskInstance(template *domain.RecurringTemplate, occursAt time.Time) (domain.TodoItem, error) {
	taskIDObj, err := uuid.NewV7()
	if err != nil {
		return domain.TodoItem{}, fmt.Errorf("failed to generate task ID: %w", err)
	}
	taskID := taskIDObj.String()

	// StartsAt is the date portion (when task becomes visible)
	startsAt := time.Date(occursAt.Year(), occursAt.Month(), occursAt.Day(), 0, 0, 0, 0, occursAt.Location())

	// Calculate DueAt if offset is specified
	var dueAt *time.Time
	if template.DueOffset != nil {
		due := startsAt.Add(*template.DueOffset)
		dueAt = &due
	}

	templateID := template.ID
	task := domain.TodoItem{
		ID:                  taskID,
		ListID:              template.ListID,
		Title:               template.Title,
		Status:              domain.TaskStatusTodo,
		Priority:            template.Priority,
		EstimatedDuration:   template.EstimatedDuration,
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
		DueAt:               dueAt,
		Tags:                template.Tags,
		RecurringTemplateID: &templateID,
		StartsAt:            &startsAt, // Date when task becomes visible
		OccursAt:            &occursAt, // Exact timestamp for this occurrence
		DueOffset:           template.DueOffset,
	}

	return task, nil
}
