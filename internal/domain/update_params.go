package domain

import "fmt"

// Valid fields for UpdateItemParams.
var updateItemValidFields = map[string]struct{}{
	"title":              {},
	"status":             {},
	"priority":           {},
	"due_at":             {},
	"tags":               {},
	"timezone":           {},
	"estimated_duration": {},
	"actual_duration":    {},
}

// Validate checks that UpdateMask contains only known fields and that
// required fields have non-nil values when included in the mask.
func (p UpdateItemParams) Validate() error {
	if len(p.UpdateMask) == 0 {
		return ErrEmptyUpdateMask
	}

	maskSet := make(map[string]bool, len(p.UpdateMask))

	// Check for unknown fields
	for _, field := range p.UpdateMask {
		if _, ok := updateItemValidFields[field]; !ok {
			return fmt.Errorf("%w: %s", ErrUnknownField, field)
		}
		maskSet[field] = true
	}

	// Required field checks (cannot be nil when in mask)
	if maskSet["title"] && p.Title == nil {
		return ErrTitleRequired
	}
	if maskSet["status"] && p.Status == nil {
		return ErrStatusRequired
	}

	return nil
}

// Valid fields for UpdateListParams.
var updateListValidFields = map[string]struct{}{
	"title": {},
}

// Validate checks that UpdateMask contains only known fields and that
// required fields have non-nil values when included in the mask.
func (p UpdateListParams) Validate() error {
	if len(p.UpdateMask) == 0 {
		return ErrEmptyUpdateMask
	}

	maskSet := make(map[string]bool, len(p.UpdateMask))

	// Check for unknown fields
	for _, field := range p.UpdateMask {
		if _, ok := updateListValidFields[field]; !ok {
			return fmt.Errorf("%w: %s", ErrUnknownField, field)
		}
		maskSet[field] = true
	}

	// Required field checks (cannot be nil when in mask)
	if maskSet["title"] && p.Title == nil {
		return ErrTitleRequired
	}

	return nil
}

// Valid fields for UpdateRecurringTemplateParams.
var updateRecurringTemplateValidFields = map[string]struct{}{
	"title":                   {},
	"tags":                    {},
	"priority":                {},
	"estimated_duration":      {},
	"recurrence_pattern":      {},
	"recurrence_config":       {},
	"due_offset":              {},
	"is_active":               {},
	"sync_horizon_days":       {},
	"generation_horizon_days": {},
}

// Validate checks that UpdateMask contains only known fields and that
// required fields have non-nil values when included in the mask.
func (p UpdateRecurringTemplateParams) Validate() error {
	if len(p.UpdateMask) == 0 {
		return ErrEmptyUpdateMask
	}

	maskSet := make(map[string]bool, len(p.UpdateMask))

	// Check for unknown fields
	for _, field := range p.UpdateMask {
		if _, ok := updateRecurringTemplateValidFields[field]; !ok {
			return fmt.Errorf("%w: %s", ErrUnknownField, field)
		}
		maskSet[field] = true
	}

	// Required field checks (cannot be nil when in mask)
	if maskSet["title"] && p.Title == nil {
		return ErrTitleRequired
	}
	if maskSet["recurrence_pattern"] && p.RecurrencePattern == nil {
		return ErrRecurrencePatternRequired
	}
	if maskSet["recurrence_config"] && p.RecurrenceConfig == nil {
		return ErrRecurrenceConfigRequired
	}

	return nil
}
