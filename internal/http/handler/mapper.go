package handler

import (
	"encoding/json"
	"fmt"
	"strings"
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
	etag := item.Etag()
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
		Etag:                &etag,
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

// parseDuration parses a duration string into time.Duration.
// Supports both ISO 8601 format (e.g., "PT1H30M") and Go format (e.g., "1h30m").
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, domain.ErrDurationEmpty
	}

	// Try Go format first (more common in code)
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Try ISO 8601 format (PT prefix)
	if d, err := parseISO8601Duration(s); err == nil {
		return d, nil
	}

	return 0, fmt.Errorf("%w (expected ISO 8601 like 'PT1H30M' or Go format like '1h30m')", domain.ErrInvalidDurationFormat)
}

// parseISO8601Duration parses ISO 8601 duration format (e.g., "PT1H30M10S").
// Only supports time components (hours, minutes, seconds), not date components.
func parseISO8601Duration(s string) (time.Duration, error) {
	if len(s) < 2 || s[0] != 'P' {
		return 0, fmt.Errorf("%w: not ISO 8601 format", domain.ErrInvalidDurationFormat)
	}

	// Remove 'P' prefix
	s = s[1:]

	// Skip date portion if present (before 'T')
	if idx := strings.Index(s, "T"); idx >= 0 {
		s = s[idx+1:]
	} else if len(s) > 0 && (s[0] == 'T') {
		s = s[1:]
	} else {
		// No 'T' means only date components, which we don't support
		return 0, fmt.Errorf("%w: ISO 8601 date durations not supported", domain.ErrInvalidDurationFormat)
	}

	var duration time.Duration
	var numBuf strings.Builder

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' || c == '.' {
			numBuf.WriteByte(c)
		} else {
			if numBuf.Len() == 0 {
				return 0, fmt.Errorf("%w: missing number before %c", domain.ErrInvalidDurationFormat, c)
			}
			numStr := numBuf.String()
			numBuf.Reset()

			// Parse as float to support decimals
			var num float64
			if _, err := fmt.Sscanf(numStr, "%f", &num); err != nil {
				return 0, fmt.Errorf("%w: invalid number: %s", domain.ErrInvalidDurationFormat, numStr)
			}

			switch c {
			case 'H':
				duration += time.Duration(num * float64(time.Hour))
			case 'M':
				duration += time.Duration(num * float64(time.Minute))
			case 'S':
				duration += time.Duration(num * float64(time.Second))
			default:
				return 0, fmt.Errorf("%w: unknown unit: %c", domain.ErrInvalidDurationFormat, c)
			}
		}
	}

	if numBuf.Len() > 0 {
		return 0, fmt.Errorf("%w: trailing number without unit", domain.ErrInvalidDurationFormat)
	}

	return duration, nil
}
