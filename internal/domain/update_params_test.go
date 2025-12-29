package domain

import (
	"errors"
	"testing"

	"github.com/rezkam/mono/internal/ptr"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// UpdateItemParams.Validate() Tests
// =============================================================================

func TestUpdateItemParams_Validate_UnknownField(t *testing.T) {
	tests := []struct {
		name    string
		mask    []string
		wantErr bool
	}{
		{
			name:    "valid field title",
			mask:    []string{"title"},
			wantErr: false,
		},
		{
			name:    "valid field status",
			mask:    []string{"status"},
			wantErr: false,
		},
		{
			name:    "valid multiple fields",
			mask:    []string{"title", "status", "priority", "due_time", "tags", "timezone", "estimated_duration", "actual_duration"},
			wantErr: false,
		},
		{
			name:    "unknown field typo",
			mask:    []string{"titl"},
			wantErr: true,
		},
		{
			name:    "unknown field stauts",
			mask:    []string{"stauts"},
			wantErr: true,
		},
		{
			name:    "unknown field mixed with valid",
			mask:    []string{"title", "unknown_field"},
			wantErr: true,
		},
		{
			name:    "empty mask is valid",
			mask:    []string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Provide valid values for required fields to isolate unknown field test
			params := UpdateItemParams{
				ItemID:     "item-123",
				ListID:     "list-456",
				UpdateMask: tt.mask,
				Title:      ptr.To("Valid Title"),
				Status:     ptr.To(TaskStatusTodo),
			}

			err := params.Validate()

			if tt.wantErr {
				assert.ErrorIs(t, err, ErrUnknownField, "should reject unknown field in mask")
			} else {
				// Should not error for unknown field (may error for required field)
				if err != nil {
					assert.False(t, errors.Is(err, ErrUnknownField), "should not error for known fields")
				}
			}
		})
	}
}

func TestUpdateItemParams_Validate_RequiredFieldNil(t *testing.T) {
	tests := []struct {
		name    string
		mask    []string
		title   *string
		status  *TaskStatus
		wantErr error
	}{
		{
			name:    "title in mask with nil value",
			mask:    []string{"title"},
			title:   nil,
			status:  nil,
			wantErr: ErrTitleRequired,
		},
		{
			name:    "status in mask with nil value",
			mask:    []string{"status"},
			title:   nil,
			status:  nil,
			wantErr: ErrStatusRequired,
		},
		{
			name:    "title in mask with valid value",
			mask:    []string{"title"},
			title:   ptr.To("Valid Title"),
			status:  nil,
			wantErr: nil,
		},
		{
			name:    "status in mask with valid value",
			mask:    []string{"status"},
			title:   nil,
			status:  ptr.To(TaskStatusTodo),
			wantErr: nil,
		},
		{
			name:    "both required fields in mask with nil values - title checked first",
			mask:    []string{"title", "status"},
			title:   nil,
			status:  nil,
			wantErr: ErrTitleRequired,
		},
		{
			name:    "nullable field priority in mask with nil is valid",
			mask:    []string{"priority"},
			title:   nil,
			status:  nil,
			wantErr: nil,
		},
		{
			name:    "nullable field tags in mask with nil is valid",
			mask:    []string{"tags"},
			title:   nil,
			status:  nil,
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := UpdateItemParams{
				ItemID:     "item-123",
				ListID:     "list-456",
				UpdateMask: tt.mask,
				Title:      tt.title,
				Status:     tt.status,
			}

			// This test should NOT panic - that's the whole point
			err := params.Validate()

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// UpdateListParams.Validate() Tests
// =============================================================================

func TestUpdateListParams_Validate_UnknownField(t *testing.T) {
	tests := []struct {
		name    string
		mask    []string
		wantErr bool
	}{
		{
			name:    "valid field title",
			mask:    []string{"title"},
			wantErr: false,
		},
		{
			name:    "unknown field",
			mask:    []string{"description"},
			wantErr: true,
		},
		{
			name:    "empty mask is valid",
			mask:    []string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := UpdateListParams{
				ListID:     "list-123",
				UpdateMask: tt.mask,
				Title:      ptr.To("Valid Title"),
			}

			err := params.Validate()

			if tt.wantErr {
				assert.ErrorIs(t, err, ErrUnknownField)
			} else {
				if err != nil {
					assert.False(t, errors.Is(err, ErrUnknownField))
				}
			}
		})
	}
}

func TestUpdateListParams_Validate_RequiredFieldNil(t *testing.T) {
	tests := []struct {
		name    string
		mask    []string
		title   *string
		wantErr error
	}{
		{
			name:    "title in mask with nil value",
			mask:    []string{"title"},
			title:   nil,
			wantErr: ErrTitleRequired,
		},
		{
			name:    "title in mask with valid value",
			mask:    []string{"title"},
			title:   ptr.To("Valid Title"),
			wantErr: nil,
		},
		{
			name:    "empty mask with nil title is valid",
			mask:    []string{},
			title:   nil,
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := UpdateListParams{
				ListID:     "list-123",
				UpdateMask: tt.mask,
				Title:      tt.title,
			}

			err := params.Validate()

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// UpdateRecurringTemplateParams.Validate() Tests
// =============================================================================

func TestUpdateRecurringTemplateParams_Validate_UnknownField(t *testing.T) {
	tests := []struct {
		name    string
		mask    []string
		wantErr bool
	}{
		{
			name:    "valid field title",
			mask:    []string{"title"},
			wantErr: false,
		},
		{
			name:    "valid multiple fields",
			mask:    []string{"title", "tags", "priority", "estimated_duration", "recurrence_pattern", "recurrence_config", "due_offset", "is_active", "generation_window_days"},
			wantErr: false,
		},
		{
			name:    "unknown field",
			mask:    []string{"unknown"},
			wantErr: true,
		},
		{
			name:    "typo in recurrence_pattern",
			mask:    []string{"recurrence_patern"},
			wantErr: true,
		},
		{
			name:    "empty mask is valid",
			mask:    []string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := UpdateRecurringTemplateParams{
				TemplateID:        "tmpl-123",
				ListID:            "list-456",
				UpdateMask:        tt.mask,
				Title:             ptr.To("Valid Title"),
				RecurrencePattern: ptr.To(RecurrenceDaily),
			}

			err := params.Validate()

			if tt.wantErr {
				assert.ErrorIs(t, err, ErrUnknownField)
			} else {
				if err != nil {
					assert.False(t, errors.Is(err, ErrUnknownField))
				}
			}
		})
	}
}

func TestUpdateRecurringTemplateParams_Validate_RequiredFieldNil(t *testing.T) {
	tests := []struct {
		name    string
		mask    []string
		title   *string
		pattern *RecurrencePattern
		wantErr error
	}{
		{
			name:    "title in mask with nil value",
			mask:    []string{"title"},
			title:   nil,
			pattern: nil,
			wantErr: ErrTitleRequired,
		},
		{
			name:    "recurrence_pattern in mask with nil value",
			mask:    []string{"recurrence_pattern"},
			title:   nil,
			pattern: nil,
			wantErr: ErrRecurrencePatternRequired,
		},
		{
			name:    "title in mask with valid value",
			mask:    []string{"title"},
			title:   ptr.To("Valid Title"),
			pattern: nil,
			wantErr: nil,
		},
		{
			name:    "recurrence_pattern in mask with valid value",
			mask:    []string{"recurrence_pattern"},
			title:   nil,
			pattern: ptr.To(RecurrenceDaily),
			wantErr: nil,
		},
		{
			name:    "nullable field tags in mask with nil is valid",
			mask:    []string{"tags"},
			title:   nil,
			pattern: nil,
			wantErr: nil,
		},
		{
			name:    "nullable field is_active in mask with nil is valid",
			mask:    []string{"is_active"},
			title:   nil,
			pattern: nil,
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := UpdateRecurringTemplateParams{
				TemplateID:        "tmpl-123",
				ListID:            "list-456",
				UpdateMask:        tt.mask,
				Title:             tt.title,
				RecurrencePattern: tt.pattern,
			}

			err := params.Validate()

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
