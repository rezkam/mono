package handler

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/oapi-codegen/runtime/types"

	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/http/openapi"
)

// Helper functions for pointer conversion

func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ptrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func ptrInt(i int) *int {
	return &i
}

func ptrUUID(s string) *types.UUID {
	if s == "" {
		return nil
	}
	u, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	typesUUID := types.UUID(u)
	return &typesUUID
}

func ptrDuration(d *time.Duration) *string {
	if d == nil {
		return nil
	}
	s := d.String()
	return &s
}

// Domain → DTO mappers

// MapListToDTO converts domain.TodoList to openapi.TodoList.
func MapListToDTO(list *domain.TodoList) openapi.TodoList {
	dto := openapi.TodoList{
		Id:          ptrUUID(list.ID),
		Title:       ptrString(list.Title),
		CreateTime:  ptrTime(list.CreateTime),
		TotalItems:  ptrInt(list.TotalItems),
		UndoneItems: ptrInt(list.UndoneItems),
	}

	if len(list.Items) > 0 {
		items := make([]openapi.TodoItem, len(list.Items))
		for i, item := range list.Items {
			items[i] = MapItemToDTO(&item)
		}
		dto.Items = &items
	}

	return dto
}

// MapItemToDTO converts domain.TodoItem to openapi.TodoItem.
func MapItemToDTO(item *domain.TodoItem) openapi.TodoItem {
	dto := openapi.TodoItem{
		Id:                  ptrUUID(item.ID),
		Title:               ptrString(item.Title),
		CreateTime:          ptrTime(item.CreateTime),
		UpdatedAt:           ptrTime(item.UpdatedAt),
		DueTime:             item.DueTime,
		Tags:                &item.Tags,
		EstimatedDuration:   ptrDuration(item.EstimatedDuration),
		ActualDuration:      ptrDuration(item.ActualDuration),
		RecurringTemplateId: ptrUUID(stringValue(item.RecurringTemplateID)),
		InstanceDate:        item.InstanceDate,
		Timezone:            item.Timezone,
	}

	// Map status
	if item.Status != "" {
		status := openapi.TaskStatus(item.Status)
		dto.Status = &status
	}

	// Map priority
	if item.Priority != nil {
		priority := openapi.TaskPriority(*item.Priority)
		dto.Priority = &priority
	}

	return dto
}

// MapTemplateToDTO converts domain.RecurringTemplate to openapi.RecurringTaskTemplate.
func MapTemplateToDTO(template *domain.RecurringTemplate) openapi.RecurringTaskTemplate {
	dto := openapi.RecurringTaskTemplate{
		Id:                   ptrUUID(template.ID),
		ListId:               ptrUUID(template.ListID),
		Title:                ptrString(template.Title),
		Tags:                 &template.Tags,
		EstimatedDuration:    ptrDuration(template.EstimatedDuration),
		DueOffset:            ptrDuration(template.DueOffset),
		IsActive:             &template.IsActive,
		CreatedAt:            ptrTime(template.CreatedAt),
		UpdatedAt:            ptrTime(template.UpdatedAt),
		LastGeneratedUntil:   ptrTime(template.LastGeneratedUntil),
		GenerationWindowDays: &template.GenerationWindowDays,
	}

	// Map priority
	if template.Priority != nil {
		priority := openapi.TaskPriority(*template.Priority)
		dto.Priority = &priority
	}

	// Map recurrence pattern
	pattern := openapi.RecurrencePattern(template.RecurrencePattern)
	dto.RecurrencePattern = &pattern

	// Map recurrence_config (domain map[string]interface{} to JSON string)
	if template.RecurrenceConfig != nil {
		configJSON, err := json.Marshal(template.RecurrenceConfig)
		if err == nil {
			configStr := string(configJSON)
			dto.RecurrenceConfig = &configStr
		}
	}

	return dto
}

// Helper to dereference *string safely
func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseDuration parses a duration string (e.g., "1h30m") into time.Duration.
// Returns an error for invalid duration formats.
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("duration cannot be empty")
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration format: %w", err)
	}
	return d, nil
}
