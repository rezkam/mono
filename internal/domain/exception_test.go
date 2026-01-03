package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExceptionType_Validation(t *testing.T) {
	tests := []struct {
		name      string
		excType   ExceptionType
		wantValid bool
	}{
		{"deleted is valid", ExceptionTypeDeleted, true},
		{"rescheduled is valid", ExceptionTypeRescheduled, true},
		{"edited is valid", ExceptionTypeEdited, true},
		{"empty is invalid", ExceptionType(""), false},
		{"random is invalid", ExceptionType("random"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.excType.Validate()
			if tt.wantValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
