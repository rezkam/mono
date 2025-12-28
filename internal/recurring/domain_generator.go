package recurring

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
)

// DomainGenerator handles the generation of task instances from recurring templates using domain types.
type DomainGenerator struct {
	// Repository interface for future use (e.g., checking existing tasks)
	// Currently unused but kept for extensibility
	repo any
}

// NewDomainGenerator creates a new task generator that works with domain types.
func NewDomainGenerator(repo any) *DomainGenerator {
	return &DomainGenerator{
		repo: repo,
	}
}

// GenerateTasksForTemplate generates task instances for a given template within the specified date range.
func (g *DomainGenerator) GenerateTasksForTemplate(ctx context.Context, template *domain.RecurringTemplate, start, end time.Time) ([]domain.TodoItem, error) {
	calculator := GetCalculator(template.RecurrencePattern)
	if calculator == nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrInvalidRecurrencePattern, template.RecurrencePattern)
	}

	// Parse recurrence config - it's already a map[string]interface{}
	config := template.RecurrenceConfig
	if config == nil {
		config = make(map[string]any)
	}

	// Calculate all occurrences in the range
	occurrences := calculator.OccurrencesBetween(start, end, config)

	// Generate task instances
	var tasks []domain.TodoItem
	for _, occurrence := range occurrences {
		task, err := g.createTaskInstance(template, occurrence)
		if err != nil {
			return nil, fmt.Errorf("failed to create task instance for %s: %w", occurrence.Format(time.RFC3339), err)
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// createTaskInstance creates a single task instance from a template for a specific date.
func (g *DomainGenerator) createTaskInstance(template *domain.RecurringTemplate, instanceDate time.Time) (domain.TodoItem, error) {
	taskIDObj, err := uuid.NewV7()
	if err != nil {
		return domain.TodoItem{}, fmt.Errorf("failed to generate task ID: %w", err)
	}
	taskID := taskIDObj.String()

	// Calculate due time if offset is specified
	var dueTime *time.Time
	if template.DueOffset != nil {
		due := instanceDate.Add(*template.DueOffset)
		dueTime = &due
	}

	templateID := template.ID
	task := domain.TodoItem{
		ID:                  taskID,
		ListID:              template.ListID,
		Title:               template.Title,
		Status:              domain.TaskStatusTodo,
		Priority:            template.Priority,
		EstimatedDuration:   template.EstimatedDuration,
		CreateTime:          time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
		DueTime:             dueTime,
		Tags:                template.Tags,
		RecurringTemplateID: &templateID,
		InstanceDate:        &instanceDate,
	}

	return task, nil
}
