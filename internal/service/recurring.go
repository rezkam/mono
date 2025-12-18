package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/core"
	"github.com/rezkam/mono/internal/storage/sql/repository"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CreateRecurringTemplate creates a new recurring task template.
func (s *MonoService) CreateRecurringTemplate(ctx context.Context, req *monov1.CreateRecurringTemplateRequest) (*monov1.CreateRecurringTemplateResponse, error) {
	if req.ListId == "" {
		return nil, status.Error(codes.InvalidArgument, "list_id is required")
	}
	if req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "title is required")
	}

	templateIDObj, err := uuid.NewV7()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate template id: %v", err)
	}
	templateID := templateIDObj.String()

	windowDays := req.GenerationWindowDays
	if windowDays == 0 {
		windowDays = 30 // Default
	}

	recurrenceConfig, err := convertRecurrenceConfig(req.RecurrenceConfig)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid recurrence config: %v", err)
	}

	template := &core.RecurringTaskTemplate{
		ID:                   templateID,
		ListID:               req.ListId,
		Title:                req.Title,
		Tags:                 req.Tags,
		Priority:             convertToCorePriority(req.Priority),
		EstimatedDuration:    convertToDuration(req.EstimatedDuration),
		RecurrencePattern:    convertToCoreRecurrencePattern(req.RecurrencePattern),
		RecurrenceConfig:     recurrenceConfig,
		DueOffset:            convertToDuration(req.DueOffset),
		IsActive:             true,
		CreatedAt:            time.Now().UTC(),
		UpdatedAt:            time.Now().UTC(),
		LastGeneratedUntil:   time.Now().UTC(),
		GenerationWindowDays: int(windowDays),
	}

	// Create and store template
	if err := s.storage.CreateRecurringTemplate(ctx, template); err != nil {
		if errors.Is(err, repository.ErrListNotFound) {
			return nil, status.Error(codes.NotFound, "list not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to create template: %v", err)
	}

	return &monov1.CreateRecurringTemplateResponse{
		Template: toProtoRecurringTemplate(template),
	}, nil
}

// GetRecurringTemplate retrieves a recurring template by its ID.
func (s *MonoService) GetRecurringTemplate(ctx context.Context, req *monov1.GetRecurringTemplateRequest) (*monov1.GetRecurringTemplateResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	template, err := s.storage.GetRecurringTemplate(ctx, req.Id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "template not found")
		}
		if errors.Is(err, repository.ErrInvalidID) {
			return nil, status.Error(codes.InvalidArgument, "invalid template ID")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve template: %v", err)
	}

	return &monov1.GetRecurringTemplateResponse{
		Template: toProtoRecurringTemplate(template),
	}, nil
}

// UpdateRecurringTemplate updates an existing recurring template.
func (s *MonoService) UpdateRecurringTemplate(ctx context.Context, req *monov1.UpdateRecurringTemplateRequest) (*monov1.UpdateRecurringTemplateResponse, error) {
	if req.Template == nil {
		return nil, status.Error(codes.InvalidArgument, "template is required")
	}

	// Get existing template
	existing, err := s.storage.GetRecurringTemplate(ctx, req.Template.Id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "template not found")
		}
		if errors.Is(err, repository.ErrInvalidID) {
			return nil, status.Error(codes.InvalidArgument, "invalid template ID")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve template: %v", err)
	}

	// Apply updates based on field mask
	if req.UpdateMask == nil || len(req.UpdateMask.Paths) == 0 {
		// No mask = update all mutable fields
		recurrenceConfig, err := convertRecurrenceConfig(req.Template.RecurrenceConfig)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid recurrence config: %v", err)
		}

		existing.Title = req.Template.Title
		existing.Tags = req.Template.Tags
		existing.Priority = convertToCorePriority(req.Template.Priority)
		existing.EstimatedDuration = convertToDuration(req.Template.EstimatedDuration)
		existing.RecurrencePattern = convertToCoreRecurrencePattern(req.Template.RecurrencePattern)
		existing.RecurrenceConfig = recurrenceConfig
		existing.DueOffset = convertToDuration(req.Template.DueOffset)
	} else {
		// Use field mask to update only specified fields
		for _, path := range req.UpdateMask.Paths {
			switch path {
			case "title":
				existing.Title = req.Template.Title
			case "tags":
				existing.Tags = req.Template.Tags
			case "priority":
				existing.Priority = convertToCorePriority(req.Template.Priority)
			case "estimated_duration":
				existing.EstimatedDuration = convertToDuration(req.Template.EstimatedDuration)
			case "recurrence_pattern":
				existing.RecurrencePattern = convertToCoreRecurrencePattern(req.Template.RecurrencePattern)
			case "recurrence_config":
				recurrenceConfig, err := convertRecurrenceConfig(req.Template.RecurrenceConfig)
				if err != nil {
					return nil, status.Errorf(codes.InvalidArgument, "invalid recurrence config: %v", err)
				}
				existing.RecurrenceConfig = recurrenceConfig
			case "due_offset":
				existing.DueOffset = convertToDuration(req.Template.DueOffset)
			}
		}
	}

	existing.UpdatedAt = time.Now().UTC()

	if err := s.storage.UpdateRecurringTemplate(ctx, existing); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update template: %v", err)
	}

	return &monov1.UpdateRecurringTemplateResponse{
		Template: toProtoRecurringTemplate(existing),
	}, nil
}

// DeleteRecurringTemplate deletes a recurring template.
func (s *MonoService) DeleteRecurringTemplate(ctx context.Context, req *monov1.DeleteRecurringTemplateRequest) (*monov1.DeleteRecurringTemplateResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	if err := s.storage.DeleteRecurringTemplate(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete template: %v", err)
	}

	return &monov1.DeleteRecurringTemplateResponse{}, nil
}

// ListRecurringTemplates lists all recurring templates for a list.
func (s *MonoService) ListRecurringTemplates(ctx context.Context, req *monov1.ListRecurringTemplatesRequest) (*monov1.ListRecurringTemplatesResponse, error) {
	if req.ListId == "" {
		return nil, status.Error(codes.InvalidArgument, "list_id is required")
	}

	templates, err := s.storage.ListRecurringTemplates(ctx, req.ListId, req.ActiveOnly)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list templates: %v", err)
	}

	protoTemplates := make([]*monov1.RecurringTaskTemplate, len(templates))
	for i, tmpl := range templates {
		protoTemplates[i] = toProtoRecurringTemplate(tmpl)
	}

	return &monov1.ListRecurringTemplatesResponse{
		Templates: protoTemplates,
	}, nil
}

// Helper conversion functions

func convertToCorePriority(p monov1.TaskPriority) *core.TaskPriority {
	if p == monov1.TaskPriority_TASK_PRIORITY_UNSPECIFIED {
		return nil
	}

	var priority core.TaskPriority
	switch p {
	case monov1.TaskPriority_TASK_PRIORITY_LOW:
		priority = core.TaskPriorityLow
	case monov1.TaskPriority_TASK_PRIORITY_MEDIUM:
		priority = core.TaskPriorityMedium
	case monov1.TaskPriority_TASK_PRIORITY_HIGH:
		priority = core.TaskPriorityHigh
	case monov1.TaskPriority_TASK_PRIORITY_URGENT:
		priority = core.TaskPriorityUrgent
	default:
		return nil
	}

	return &priority
}

func convertToDuration(d *durationpb.Duration) *time.Duration {
	if d == nil {
		return nil
	}

	duration := d.AsDuration()
	return &duration
}

func convertToCoreRecurrencePattern(p monov1.RecurrencePattern) core.RecurrencePattern {
	switch p {
	case monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY:
		return core.RecurrenceDaily
	case monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKLY:
		return core.RecurrenceWeekly
	case monov1.RecurrencePattern_RECURRENCE_PATTERN_BIWEEKLY:
		return core.RecurrenceBiweekly
	case monov1.RecurrencePattern_RECURRENCE_PATTERN_MONTHLY:
		return core.RecurrenceMonthly
	case monov1.RecurrencePattern_RECURRENCE_PATTERN_YEARLY:
		return core.RecurrenceYearly
	case monov1.RecurrencePattern_RECURRENCE_PATTERN_QUARTERLY:
		return core.RecurrenceQuarterly
	case monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKDAYS:
		return core.RecurrenceWeekdays
	default:
		return ""
	}
}

func convertRecurrenceConfig(configJSON string) (map[string]interface{}, error) {
	if configJSON == "" {
		return make(map[string]interface{}), nil
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return nil, fmt.Errorf("invalid recurrence config JSON: %w", err)
	}

	return config, nil
}

func convertToProtoRecurrencePattern(p core.RecurrencePattern) monov1.RecurrencePattern {
	switch p {
	case core.RecurrenceDaily:
		return monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY
	case core.RecurrenceWeekly:
		return monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKLY
	case core.RecurrenceBiweekly:
		return monov1.RecurrencePattern_RECURRENCE_PATTERN_BIWEEKLY
	case core.RecurrenceMonthly:
		return monov1.RecurrencePattern_RECURRENCE_PATTERN_MONTHLY
	case core.RecurrenceYearly:
		return monov1.RecurrencePattern_RECURRENCE_PATTERN_YEARLY
	case core.RecurrenceQuarterly:
		return monov1.RecurrencePattern_RECURRENCE_PATTERN_QUARTERLY
	case core.RecurrenceWeekdays:
		return monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKDAYS
	default:
		return monov1.RecurrencePattern_RECURRENCE_PATTERN_UNSPECIFIED
	}
}

func toProtoRecurringTemplate(t *core.RecurringTaskTemplate) *monov1.RecurringTaskTemplate {
	template := &monov1.RecurringTaskTemplate{
		Id:                   t.ID,
		ListId:               t.ListID,
		Title:                t.Title,
		Tags:                 t.Tags,
		RecurrencePattern:    convertToProtoRecurrencePattern(t.RecurrencePattern),
		IsActive:             t.IsActive,
		CreatedAt:            timestamppb.New(t.CreatedAt),
		UpdatedAt:            timestamppb.New(t.UpdatedAt),
		LastGeneratedUntil:   timestamppb.New(t.LastGeneratedUntil),
		GenerationWindowDays: int32(t.GenerationWindowDays),
	}

	if t.Priority != nil {
		template.Priority = toProtoPriority(*t.Priority)
	}

	if t.EstimatedDuration != nil {
		template.EstimatedDuration = durationpb.New(*t.EstimatedDuration)
	}

	if t.DueOffset != nil {
		template.DueOffset = durationpb.New(*t.DueOffset)
	}

	// Convert RecurrenceConfig to JSON string
	if t.RecurrenceConfig != nil {
		if configBytes, err := json.Marshal(t.RecurrenceConfig); err == nil {
			template.RecurrenceConfig = string(configBytes)
		}
	}

	return template
}
