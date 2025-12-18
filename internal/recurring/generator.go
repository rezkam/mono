package recurring

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/core"
)

// Generator handles the generation of task instances from recurring templates.
type Generator struct {
	storage core.Storage
}

// NewGenerator creates a new task generator.
func NewGenerator(storage core.Storage) *Generator {
	return &Generator{
		storage: storage,
	}
}

// GenerateTasksForTemplate generates task instances for a given template within the specified date range.
func (g *Generator) GenerateTasksForTemplate(ctx context.Context, template *core.RecurringTaskTemplate, start, end time.Time) ([]core.TodoItem, error) {
	calculator := GetCalculator(template.RecurrencePattern)
	if calculator == nil {
		return nil, fmt.Errorf("unsupported recurrence pattern: %s", template.RecurrencePattern)
	}

	// Parse recurrence config - it's already a map[string]interface{}
	config := template.RecurrenceConfig
	if config == nil {
		config = make(map[string]interface{})
	}

	// Calculate all occurrences in the range
	occurrences := calculator.OccurrencesBetween(start, end, config)

	// Generate task instances
	var tasks []core.TodoItem
	for _, occurrence := range occurrences {
		task := g.createTaskInstance(template, occurrence)
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// createTaskInstance creates a single task instance from a template for a specific date.
func (g *Generator) createTaskInstance(template *core.RecurringTaskTemplate, instanceDate time.Time) core.TodoItem {
	taskIDObj, err := uuid.NewV7()
	if err != nil {
		// Fallback to zero-value TodoItem in case of UUID generation failure
		// This should be extremely rare, but we handle it gracefully
		return core.TodoItem{}
	}
	taskID := taskIDObj.String()

	// Calculate due time if offset is specified
	var dueTime *time.Time
	if template.DueOffset != nil {
		due := instanceDate.Add(*template.DueOffset)
		dueTime = &due
	}

	task := core.TodoItem{
		ID:                  taskID,
		Title:               template.Title,
		Status:              core.TaskStatusTodo,
		Priority:            template.Priority,
		EstimatedDuration:   template.EstimatedDuration,
		CreateTime:          time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
		DueTime:             dueTime,
		Tags:                template.Tags,
		RecurringTemplateID: &template.ID,
		InstanceDate:        &instanceDate,
	}

	return task
}

// GenerateUpcomingTasks generates tasks for all active templates that need generation.
// This should be called periodically (e.g., by a background worker).
func (g *Generator) GenerateUpcomingTasks(ctx context.Context, listID string) error {
	// Note: We need to extend the storage interface to support recurring templates.
	// This will be implemented when we add template storage methods.
	_ = listID // Mark as intentionally unused for now

	return nil
}
