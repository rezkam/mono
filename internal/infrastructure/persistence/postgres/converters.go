package postgres

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres/sqlcgen"
	"github.com/sqlc-dev/pqtype"
)

// === API Key Conversions ===

func dbAPIKeyToDomain(dbKey sqlcgen.ApiKey) *domain.APIKey {
	key := &domain.APIKey{
		ID:             dbKey.ID.String(),
		KeyType:        dbKey.KeyType,
		Service:        dbKey.Service,
		Version:        dbKey.Version,
		ShortToken:     dbKey.ShortToken,
		LongSecretHash: dbKey.LongSecretHash,
		Name:           dbKey.Name,
		IsActive:       dbKey.IsActive,
		CreatedAt:      dbKey.CreatedAt,
	}

	if dbKey.LastUsedAt.Valid {
		key.LastUsedAt = &dbKey.LastUsedAt.Time
	}

	if dbKey.ExpiresAt.Valid {
		key.ExpiresAt = &dbKey.ExpiresAt.Time
	}

	return key
}

// === Todo List Conversions ===

func dbTodoListToDomain(dbList sqlcgen.TodoList) *domain.TodoList {
	return &domain.TodoList{
		ID:         dbList.ID.String(),
		Title:      dbList.Title,
		Items:      []domain.TodoItem{}, // Populated separately if needed
		CreateTime: dbList.CreateTime,
	}
}

func domainTodoListToDB(list *domain.TodoList) (uuid.UUID, string, time.Time, error) {
	id, err := uuid.Parse(list.ID)
	if err != nil {
		return uuid.UUID{}, "", time.Time{}, fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	return id, list.Title, list.CreateTime, nil
}

// === Todo Item Conversions ===

func dbTodoItemToDomain(dbItem sqlcgen.TodoItem) domain.TodoItem {
	item := domain.TodoItem{
		ID:         dbItem.ID.String(),
		ListID:     dbItem.ListID.String(),
		Title:      dbItem.Title,
		Status:     domain.TaskStatus(dbItem.Status),
		CreateTime: dbItem.CreateTime,
		UpdatedAt:  dbItem.UpdatedAt,
	}

	// Priority
	if dbItem.Priority.Valid {
		priority := domain.TaskPriority(dbItem.Priority.String)
		item.Priority = &priority
	}

	// Estimated Duration
	if dbItem.EstimatedDuration.Valid {
		duration := intervalToDuration(dbItem.EstimatedDuration)
		item.EstimatedDuration = &duration
	}

	// Actual Duration
	if dbItem.ActualDuration.Valid {
		duration := intervalToDuration(dbItem.ActualDuration)
		item.ActualDuration = &duration
	}

	// Due Time
	if dbItem.DueTime.Valid {
		item.DueTime = &dbItem.DueTime.Time
	}

	// Tags
	if dbItem.Tags.Valid {
		var tags []string
		if err := json.Unmarshal(dbItem.Tags.RawMessage, &tags); err == nil {
			item.Tags = tags
		}
	}

	// Recurring Template ID
	if dbItem.RecurringTemplateID.Valid {
		templateID := dbItem.RecurringTemplateID.UUID.String()
		item.RecurringTemplateID = &templateID
	}

	// Instance Date
	if dbItem.InstanceDate.Valid {
		item.InstanceDate = &dbItem.InstanceDate.Time
	}

	// Timezone
	if dbItem.Timezone.Valid {
		item.Timezone = &dbItem.Timezone.String
	}

	return item
}

// dbListTasksRowToDomain converts a ListTasksWithFiltersRow to a domain TodoItem.
// This is needed because the query includes COUNT(*) OVER() which creates a different struct.
func dbListTasksRowToDomain(dbItem sqlcgen.ListTasksWithFiltersRow) domain.TodoItem {
	item := domain.TodoItem{
		ID:         dbItem.ID.String(),
		ListID:     dbItem.ListID.String(),
		Title:      dbItem.Title,
		Status:     domain.TaskStatus(dbItem.Status),
		CreateTime: dbItem.CreateTime,
		UpdatedAt:  dbItem.UpdatedAt,
	}

	// Priority
	if dbItem.Priority.Valid {
		priority := domain.TaskPriority(dbItem.Priority.String)
		item.Priority = &priority
	}

	// Estimated Duration
	if dbItem.EstimatedDuration.Valid {
		duration := intervalToDuration(dbItem.EstimatedDuration)
		item.EstimatedDuration = &duration
	}

	// Actual Duration
	if dbItem.ActualDuration.Valid {
		duration := intervalToDuration(dbItem.ActualDuration)
		item.ActualDuration = &duration
	}

	// Due Time
	if dbItem.DueTime.Valid {
		item.DueTime = &dbItem.DueTime.Time
	}

	// Tags
	if dbItem.Tags.Valid {
		var tags []string
		if err := json.Unmarshal(dbItem.Tags.RawMessage, &tags); err == nil {
			item.Tags = tags
		}
	}

	// Recurring Template ID
	if dbItem.RecurringTemplateID.Valid {
		templateID := dbItem.RecurringTemplateID.UUID.String()
		item.RecurringTemplateID = &templateID
	}

	// Instance Date
	if dbItem.InstanceDate.Valid {
		item.InstanceDate = &dbItem.InstanceDate.Time
	}

	// Timezone
	if dbItem.Timezone.Valid {
		item.Timezone = &dbItem.Timezone.String
	}

	return item
}

func domainTodoItemToDB(item *domain.TodoItem, listID string) (sqlcgen.CreateTodoItemParams, error) {
	itemID, err := uuid.Parse(item.ID)
	if err != nil {
		return sqlcgen.CreateTodoItemParams{}, fmt.Errorf("%w: item %v", domain.ErrInvalidID, err)
	}

	listUUID, err := uuid.Parse(listID)
	if err != nil {
		return sqlcgen.CreateTodoItemParams{}, fmt.Errorf("%w: list %v", domain.ErrInvalidID, err)
	}

	params := sqlcgen.CreateTodoItemParams{
		ID:         itemID,
		ListID:     listUUID,
		Title:      item.Title,
		Status:     string(item.Status),
		CreateTime: item.CreateTime,
		UpdatedAt:  item.UpdatedAt,
	}

	// Priority
	if item.Priority != nil {
		params.Priority = sql.NullString{
			String: string(*item.Priority),
			Valid:  true,
		}
	}

	// Estimated Duration
	if item.EstimatedDuration != nil {
		params.EstimatedDuration = durationToInterval(*item.EstimatedDuration)
	}

	// Actual Duration
	if item.ActualDuration != nil {
		params.ActualDuration = durationToInterval(*item.ActualDuration)
	}

	// Due Time
	if item.DueTime != nil {
		params.DueTime = sql.NullTime{
			Time:  *item.DueTime,
			Valid: true,
		}
	}

	// Tags
	if len(item.Tags) > 0 {
		tagsJSON, err := json.Marshal(item.Tags)
		if err != nil {
			return params, fmt.Errorf("failed to marshal tags: %w", err)
		}
		params.Tags = pqtype.NullRawMessage{
			RawMessage: tagsJSON,
			Valid:      true,
		}
	}

	// Recurring Template ID
	if item.RecurringTemplateID != nil {
		templateUUID, err := uuid.Parse(*item.RecurringTemplateID)
		if err != nil {
			return params, fmt.Errorf("%w: recurring template %v", domain.ErrInvalidID, err)
		}
		params.RecurringTemplateID = uuid.NullUUID{
			UUID:  templateUUID,
			Valid: true,
		}
	}

	// Instance Date
	if item.InstanceDate != nil {
		params.InstanceDate = sql.NullTime{
			Time:  *item.InstanceDate,
			Valid: true,
		}
	}

	// Timezone
	if item.Timezone != nil {
		params.Timezone = sql.NullString{
			String: *item.Timezone,
			Valid:  true,
		}
	}

	return params, nil
}

func domainTodoItemToUpdateParams(item *domain.TodoItem) (sqlcgen.UpdateTodoItemParams, error) {
	itemID, err := uuid.Parse(item.ID)
	if err != nil {
		return sqlcgen.UpdateTodoItemParams{}, fmt.Errorf("%w: item %v", domain.ErrInvalidID, err)
	}

	params := sqlcgen.UpdateTodoItemParams{
		ID:        itemID,
		Title:     item.Title,
		Status:    string(item.Status),
		UpdatedAt: item.UpdatedAt,
	}

	// Priority
	if item.Priority != nil {
		params.Priority = sql.NullString{
			String: string(*item.Priority),
			Valid:  true,
		}
	}

	// Estimated Duration
	if item.EstimatedDuration != nil {
		params.EstimatedDuration = durationToInterval(*item.EstimatedDuration)
	}

	// Actual Duration
	if item.ActualDuration != nil {
		params.ActualDuration = durationToInterval(*item.ActualDuration)
	}

	// Due Time
	if item.DueTime != nil {
		params.DueTime = sql.NullTime{
			Time:  *item.DueTime,
			Valid: true,
		}
	}

	// Tags
	if len(item.Tags) > 0 {
		tagsJSON, err := json.Marshal(item.Tags)
		if err != nil {
			return params, fmt.Errorf("failed to marshal tags: %w", err)
		}
		params.Tags = pqtype.NullRawMessage{
			RawMessage: tagsJSON,
			Valid:      true,
		}
	}

	// Timezone
	if item.Timezone != nil {
		params.Timezone = sql.NullString{
			String: *item.Timezone,
			Valid:  true,
		}
	}

	return params, nil
}

// === Recurring Template Conversions ===

func dbRecurringTemplateToDomain(dbTemplate sqlcgen.RecurringTaskTemplate) (*domain.RecurringTemplate, error) {
	template := &domain.RecurringTemplate{
		ID:                   dbTemplate.ID.String(),
		ListID:               dbTemplate.ListID.String(),
		Title:                dbTemplate.Title,
		RecurrencePattern:    domain.RecurrencePattern(dbTemplate.RecurrencePattern),
		IsActive:             dbTemplate.IsActive,
		CreatedAt:            dbTemplate.CreatedAt,
		UpdatedAt:            dbTemplate.UpdatedAt,
		LastGeneratedUntil:   dbTemplate.LastGeneratedUntil,
		GenerationWindowDays: int(dbTemplate.GenerationWindowDays),
	}

	// Tags
	if dbTemplate.Tags.Valid {
		var tags []string
		if err := json.Unmarshal(dbTemplate.Tags.RawMessage, &tags); err == nil {
			template.Tags = tags
		}
	}

	// Priority
	if dbTemplate.Priority.Valid {
		priority := domain.TaskPriority(dbTemplate.Priority.String)
		template.Priority = &priority
	}

	// Estimated Duration
	if dbTemplate.EstimatedDuration.Valid {
		duration := intervalToDuration(dbTemplate.EstimatedDuration)
		template.EstimatedDuration = &duration
	}

	// Recurrence Config
	if len(dbTemplate.RecurrenceConfig) > 0 {
		var config map[string]interface{}
		if err := json.Unmarshal(dbTemplate.RecurrenceConfig, &config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal recurrence config: %w", err)
		}
		template.RecurrenceConfig = config
	}

	// Due Offset
	if dbTemplate.DueOffset.Valid {
		duration := intervalToDuration(dbTemplate.DueOffset)
		template.DueOffset = &duration
	}

	return template, nil
}

func domainRecurringTemplateToDB(template *domain.RecurringTemplate) (sqlcgen.CreateRecurringTemplateParams, error) {
	templateID, err := uuid.Parse(template.ID)
	if err != nil {
		return sqlcgen.CreateRecurringTemplateParams{}, fmt.Errorf("%w: template %v", domain.ErrInvalidID, err)
	}

	listID, err := uuid.Parse(template.ListID)
	if err != nil {
		return sqlcgen.CreateRecurringTemplateParams{}, fmt.Errorf("%w: list %v", domain.ErrInvalidID, err)
	}

	params := sqlcgen.CreateRecurringTemplateParams{
		ID:                   templateID,
		ListID:               listID,
		Title:                template.Title,
		RecurrencePattern:    string(template.RecurrencePattern),
		IsActive:             template.IsActive,
		CreatedAt:            template.CreatedAt,
		UpdatedAt:            template.UpdatedAt,
		LastGeneratedUntil:   template.LastGeneratedUntil,
		GenerationWindowDays: int32(template.GenerationWindowDays),
	}

	// Tags
	if len(template.Tags) > 0 {
		tagsJSON, err := json.Marshal(template.Tags)
		if err != nil {
			return params, fmt.Errorf("failed to marshal tags: %w", err)
		}
		params.Tags = pqtype.NullRawMessage{
			RawMessage: tagsJSON,
			Valid:      true,
		}
	}

	// Priority
	if template.Priority != nil {
		params.Priority = sql.NullString{
			String: string(*template.Priority),
			Valid:  true,
		}
	}

	// Estimated Duration
	if template.EstimatedDuration != nil {
		params.EstimatedDuration = durationToInterval(*template.EstimatedDuration)
	}

	// Recurrence Config
	if template.RecurrenceConfig != nil {
		configJSON, err := json.Marshal(template.RecurrenceConfig)
		if err != nil {
			return params, fmt.Errorf("failed to marshal recurrence config: %w", err)
		}
		params.RecurrenceConfig = configJSON
	}

	// Due Offset
	if template.DueOffset != nil {
		params.DueOffset = durationToInterval(*template.DueOffset)
	}

	return params, nil
}

// === Generation Job Conversions ===

func dbGenerationJobToDomain(dbJob sqlcgen.RecurringGenerationJob) *domain.GenerationJob {
	job := &domain.GenerationJob{
		ID:            dbJob.ID.String(),
		TemplateID:    dbJob.TemplateID.String(),
		ScheduledFor:  dbJob.ScheduledFor,
		Status:        dbJob.Status,
		GenerateFrom:  dbJob.GenerateFrom,
		GenerateUntil: dbJob.GenerateUntil,
		CreatedAt:     dbJob.CreatedAt,
		RetryCount:    int(dbJob.RetryCount),
	}

	if dbJob.StartedAt.Valid {
		job.StartedAt = &dbJob.StartedAt.Time
	}

	if dbJob.CompletedAt.Valid {
		job.CompletedAt = &dbJob.CompletedAt.Time
	}

	if dbJob.FailedAt.Valid {
		job.FailedAt = &dbJob.FailedAt.Time
	}

	if dbJob.ErrorMessage.Valid {
		job.ErrorMessage = &dbJob.ErrorMessage.String
	}

	return job
}

// === Helper Functions ===

// intervalToDuration converts PostgreSQL interval to Go duration.
// Note: This assumes intervals are stored as microseconds.
func intervalToDuration(interval pgtype.Interval) time.Duration {
	// pgtype.Interval has Microseconds field
	return time.Duration(interval.Microseconds) * time.Microsecond
}

// durationToInterval converts Go duration to PostgreSQL interval.
func durationToInterval(d time.Duration) pgtype.Interval {
	return pgtype.Interval{
		Microseconds: d.Microseconds(),
		Valid:        true,
	}
}
