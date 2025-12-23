package postgres

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres/sqlcgen"
)

// === pgtype Conversion Helpers ===

// uuidToPgtype converts google/uuid.UUID to pgtype.UUID.
func uuidToPgtype(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// pgtypeToUUIDString converts pgtype.UUID to string (empty if invalid).
func pgtypeToUUIDString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

// timeToPgtype converts time.Time to pgtype.Timestamptz.
func timeToPgtype(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// pgtypeToTime converts pgtype.Timestamptz to time.Time (zero if invalid).
// Always returns time in UTC location for consistent timezone handling.
func pgtypeToTime(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time.UTC()
}

// pgtypeToTimePtr converts pgtype.Timestamptz to *time.Time (nil if invalid).
// Always returns time in UTC location for consistent timezone handling.
func pgtypeToTimePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	utcTime := t.Time.UTC()
	return &utcTime
}

// timePtrToPgtype converts *time.Time to pgtype.Timestamptz.
// For nil pointers, returns NULL (Valid: false) to store NULL in the database.
// Use this for actual data fields like DueTime, ExpiresAt, etc.
func timePtrToPgtype(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// timePtrToPgtypeForFilter converts *time.Time to pgtype.Timestamptz for filter parameters.
// For nil pointers, returns a valid zero time (0001-01-01) instead of NULL.
// This is required for SQL skip-filter patterns like:
//
//	($5::timestamptz = '0001-01-01 00:00:00+00' OR due_time <= $5)
//
// With NULL, both sides of the OR would return NULL (falsy), breaking the pattern.
func timePtrToPgtypeForFilter(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		// Return valid zero time to match SQL's skip-filter pattern
		return pgtype.Timestamptz{Time: time.Time{}, Valid: true}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// dateToPgtype converts time.Time to pgtype.Date.
func dateToPgtype(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: true}
}

// pgtypeToDate converts pgtype.Date to time.Time (zero if invalid).
func pgtypeToDate(d pgtype.Date) time.Time {
	if !d.Valid {
		return time.Time{}
	}
	return d.Time
}

// pgtypeToDatePtr converts pgtype.Date to *time.Time (nil if invalid).
func pgtypeToDatePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	return &d.Time
}

// === API Key Conversions ===

func dbAPIKeyToDomain(dbKey sqlcgen.ApiKey) *domain.APIKey {
	return &domain.APIKey{
		ID:             pgtypeToUUIDString(dbKey.ID),
		KeyType:        dbKey.KeyType,
		Service:        dbKey.Service,
		Version:        dbKey.Version,
		ShortToken:     dbKey.ShortToken,
		LongSecretHash: dbKey.LongSecretHash,
		Name:           dbKey.Name,
		IsActive:       dbKey.IsActive,
		CreatedAt:      pgtypeToTime(dbKey.CreatedAt),
		LastUsedAt:     pgtypeToTimePtr(dbKey.LastUsedAt),
		ExpiresAt:      pgtypeToTimePtr(dbKey.ExpiresAt),
	}
}

// === Todo List Conversions ===

func domainTodoListToDB(list *domain.TodoList) (pgtype.UUID, string, pgtype.Timestamptz, error) {
	id, err := uuid.Parse(list.ID)
	if err != nil {
		return pgtype.UUID{}, "", pgtype.Timestamptz{}, fmt.Errorf("%w: %v", domain.ErrInvalidID, err)
	}

	return uuidToPgtype(id), list.Title, timeToPgtype(list.CreateTime), nil
}

// domainStatusesToStrings converts domain TaskStatus slice to string slice for SQL queries.
func domainStatusesToStrings(statuses []domain.TaskStatus) []string {
	result := make([]string, len(statuses))
	for i, s := range statuses {
		result[i] = string(s)
	}
	return result
}

// === Todo Item Conversions ===

func dbTodoItemToDomain(dbItem sqlcgen.TodoItem) (domain.TodoItem, error) {
	item := domain.TodoItem{
		ID:         pgtypeToUUIDString(dbItem.ID),
		ListID:     pgtypeToUUIDString(dbItem.ListID),
		Title:      dbItem.Title,
		Status:     domain.TaskStatus(dbItem.Status),
		CreateTime: pgtypeToTime(dbItem.CreateTime),
		UpdatedAt:  pgtypeToTime(dbItem.UpdatedAt),
		DueTime:    pgtypeToTimePtr(dbItem.DueTime),
		Timezone:   dbItem.Timezone,
		Version:    int(dbItem.Version),
	}

	// Priority (now *string in pgx)
	if dbItem.Priority != nil {
		priority := domain.TaskPriority(*dbItem.Priority)
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

	// Tags (now []byte in pgx)
	if len(dbItem.Tags) > 0 {
		var tags []string
		if err := json.Unmarshal(dbItem.Tags, &tags); err != nil {
			return domain.TodoItem{}, fmt.Errorf("invalid tags JSON for item %s: %w", pgtypeToUUIDString(dbItem.ID), err)
		}
		item.Tags = tags
	}

	// Recurring Template ID
	if dbItem.RecurringTemplateID.Valid {
		templateID := pgtypeToUUIDString(dbItem.RecurringTemplateID)
		item.RecurringTemplateID = &templateID
	}

	// Instance Date
	item.InstanceDate = pgtypeToDatePtr(dbItem.InstanceDate)

	return item, nil
}

// dbListTasksRowToDomain converts a ListTasksWithFiltersRow to a domain TodoItem.
// This is needed because the query includes COUNT(*) OVER() which creates a different struct.
func dbListTasksRowToDomain(dbItem sqlcgen.ListTasksWithFiltersRow) (domain.TodoItem, error) {
	item := domain.TodoItem{
		ID:         pgtypeToUUIDString(dbItem.ID),
		ListID:     pgtypeToUUIDString(dbItem.ListID),
		Title:      dbItem.Title,
		Status:     domain.TaskStatus(dbItem.Status),
		CreateTime: pgtypeToTime(dbItem.CreateTime),
		UpdatedAt:  pgtypeToTime(dbItem.UpdatedAt),
		DueTime:    pgtypeToTimePtr(dbItem.DueTime),
		Timezone:   dbItem.Timezone,
		Version:    int(dbItem.Version),
	}

	// Priority (now *string in pgx)
	if dbItem.Priority != nil {
		priority := domain.TaskPriority(*dbItem.Priority)
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

	// Tags (now []byte in pgx)
	if len(dbItem.Tags) > 0 {
		var tags []string
		if err := json.Unmarshal(dbItem.Tags, &tags); err != nil {
			return domain.TodoItem{}, fmt.Errorf("invalid tags JSON for item %s: %w", pgtypeToUUIDString(dbItem.ID), err)
		}
		item.Tags = tags
	}

	// Recurring Template ID
	if dbItem.RecurringTemplateID.Valid {
		templateID := pgtypeToUUIDString(dbItem.RecurringTemplateID)
		item.RecurringTemplateID = &templateID
	}

	// Instance Date
	item.InstanceDate = pgtypeToDatePtr(dbItem.InstanceDate)

	return item, nil
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
		ID:         uuidToPgtype(itemID),
		ListID:     uuidToPgtype(listUUID),
		Title:      item.Title,
		Status:     string(item.Status),
		CreateTime: timeToPgtype(item.CreateTime),
		UpdatedAt:  timeToPgtype(item.UpdatedAt),
		DueTime:    timePtrToPgtype(item.DueTime),
		Timezone:   item.Timezone,
	}

	// Priority (now *string in pgx)
	if item.Priority != nil {
		priority := string(*item.Priority)
		params.Priority = &priority
	}

	// Estimated Duration
	if item.EstimatedDuration != nil {
		params.EstimatedDuration = durationToInterval(*item.EstimatedDuration)
	}

	// Actual Duration
	if item.ActualDuration != nil {
		params.ActualDuration = durationToInterval(*item.ActualDuration)
	}

	// Tags (now []byte in pgx)
	if len(item.Tags) > 0 {
		tagsJSON, err := json.Marshal(item.Tags)
		if err != nil {
			return params, fmt.Errorf("failed to marshal tags: %w", err)
		}
		params.Tags = tagsJSON
	}

	// Recurring Template ID
	if item.RecurringTemplateID != nil {
		templateUUID, err := uuid.Parse(*item.RecurringTemplateID)
		if err != nil {
			return params, fmt.Errorf("%w: recurring template %v", domain.ErrInvalidID, err)
		}
		params.RecurringTemplateID = uuidToPgtype(templateUUID)
	}

	// Instance Date
	if item.InstanceDate != nil {
		params.InstanceDate = dateToPgtype(*item.InstanceDate)
	}

	return params, nil
}

// === Recurring Template Conversions ===

func dbRecurringTemplateToDomain(dbTemplate sqlcgen.RecurringTaskTemplate) (*domain.RecurringTemplate, error) {
	template := &domain.RecurringTemplate{
		ID:                   pgtypeToUUIDString(dbTemplate.ID),
		ListID:               pgtypeToUUIDString(dbTemplate.ListID),
		Title:                dbTemplate.Title,
		RecurrencePattern:    domain.RecurrencePattern(dbTemplate.RecurrencePattern),
		IsActive:             dbTemplate.IsActive,
		CreatedAt:            pgtypeToTime(dbTemplate.CreatedAt),
		UpdatedAt:            pgtypeToTime(dbTemplate.UpdatedAt),
		LastGeneratedUntil:   pgtypeToDate(dbTemplate.LastGeneratedUntil),
		GenerationWindowDays: int(dbTemplate.GenerationWindowDays),
	}

	// Tags (now []byte in pgx)
	if len(dbTemplate.Tags) > 0 {
		var tags []string
		if err := json.Unmarshal(dbTemplate.Tags, &tags); err != nil {
			return nil, fmt.Errorf("invalid tags JSON for template %s: %w", pgtypeToUUIDString(dbTemplate.ID), err)
		}
		template.Tags = tags
	}

	// Priority (now *string in pgx)
	if dbTemplate.Priority != nil {
		priority := domain.TaskPriority(*dbTemplate.Priority)
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
		ID:                   uuidToPgtype(templateID),
		ListID:               uuidToPgtype(listID),
		Title:                template.Title,
		RecurrencePattern:    string(template.RecurrencePattern),
		IsActive:             template.IsActive,
		CreatedAt:            timeToPgtype(template.CreatedAt),
		UpdatedAt:            timeToPgtype(template.UpdatedAt),
		LastGeneratedUntil:   dateToPgtype(template.LastGeneratedUntil),
		GenerationWindowDays: int32(template.GenerationWindowDays),
	}

	// Tags (now []byte in pgx)
	if len(template.Tags) > 0 {
		tagsJSON, err := json.Marshal(template.Tags)
		if err != nil {
			return params, fmt.Errorf("failed to marshal tags: %w", err)
		}
		params.Tags = tagsJSON
	}

	// Priority (now *string in pgx)
	if template.Priority != nil {
		priority := string(*template.Priority)
		params.Priority = &priority
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
	return &domain.GenerationJob{
		ID:            pgtypeToUUIDString(dbJob.ID),
		TemplateID:    pgtypeToUUIDString(dbJob.TemplateID),
		ScheduledFor:  pgtypeToTime(dbJob.ScheduledFor),
		Status:        dbJob.Status,
		GenerateFrom:  pgtypeToDate(dbJob.GenerateFrom),
		GenerateUntil: pgtypeToDate(dbJob.GenerateUntil),
		CreatedAt:     pgtypeToTime(dbJob.CreatedAt),
		RetryCount:    int(dbJob.RetryCount),
		StartedAt:     pgtypeToTimePtr(dbJob.StartedAt),
		CompletedAt:   pgtypeToTimePtr(dbJob.CompletedAt),
		FailedAt:      pgtypeToTimePtr(dbJob.FailedAt),
		ErrorMessage:  dbJob.ErrorMessage, // Now directly *string
	}
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

// durationPtrToPgtypeInterval converts optional Go duration pointer to pgtype.Interval.
// If duration is nil, returns invalid interval (for use with COALESCE in SQL).
func durationPtrToPgtypeInterval(d *time.Duration) pgtype.Interval {
	if d == nil {
		return pgtype.Interval{Valid: false}
	}
	return durationToInterval(*d)
}
