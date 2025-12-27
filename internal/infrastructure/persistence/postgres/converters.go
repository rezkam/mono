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

// stringPtrToText converts *string to pgtype.Text for sqlc params.
func stringPtrToText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

// int32PtrToInt4 converts *int32 to pgtype.Int4 for sqlc params.
func int32PtrToInt4(i *int32) pgtype.Int4 {
	if i == nil {
		return pgtype.Int4{Valid: false}
	}
	return pgtype.Int4{Int32: *i, Valid: true}
}

// boolPtrToBool converts *bool to pgtype.Bool for sqlc params.
func boolPtrToBool(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{Valid: false}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

// pgtypeToDate converts pgtype.Date to time.Time (zero if invalid).
func pgtypeToDate(d pgtype.Date) time.Time {
	if !d.Valid {
		return time.Time{}
	}
	return d.Time
}

// === sql.Null[T] ↔ Pointer Conversion Helpers ===
// These helpers translate between database sql.Null[T] and domain pointers (*T).
// Used ONLY in infrastructure layer - domain layer must never import database/sql.

// nullTimeToPtr converts sql.Null[time.Time] from database to *time.Time for domain.
func nullTimeToPtr(n sql.Null[time.Time]) *time.Time {
	if !n.Valid {
		return nil
	}
	return &n.V
}

// ptrToNullTime converts *time.Time from domain to sql.Null[time.Time] for database.
func ptrToNullTime(ptr *time.Time) sql.Null[time.Time] {
	if ptr == nil {
		return sql.Null[time.Time]{Valid: false}
	}
	return sql.Null[time.Time]{V: *ptr, Valid: true}
}

// nullStringToPtr converts sql.Null[string] from database to *string for domain.
func nullStringToPtr(n sql.Null[string]) *string {
	if !n.Valid {
		return nil
	}
	return &n.V
}

// ptrToNullString converts *string from domain to sql.Null[string] for database.
func ptrToNullString(ptr *string) sql.Null[string] {
	if ptr == nil {
		return sql.Null[string]{Valid: false}
	}
	return sql.Null[string]{V: *ptr, Valid: true}
}

// nullUUIDToStringPtr converts uuid.NullUUID from database to *string for domain.
func nullUUIDToStringPtr(n uuid.NullUUID) *string {
	if !n.Valid {
		return nil
	}
	str := n.UUID.String()
	return &str
}

// stringPtrToNullUUID converts *string from domain to uuid.NullUUID for database.
func stringPtrToNullUUID(ptr *string) (uuid.NullUUID, error) {
	if ptr == nil {
		return uuid.NullUUID{Valid: false}, nil
	}
	templateUUID, err := uuid.Parse(*ptr)
	if err != nil {
		return uuid.NullUUID{}, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}
	return uuid.NullUUID{UUID: templateUUID, Valid: true}, nil
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
		LastUsedAt:     nullTimeToPtr(dbKey.LastUsedAt), // DB sql.Null[time.Time] → Domain *time.Time
		ExpiresAt:      nullTimeToPtr(dbKey.ExpiresAt),  // DB sql.Null[time.Time] → Domain *time.Time
	}
}

// === Todo List Conversions ===

func domainTodoListToDB(list *domain.TodoList) (pgtype.UUID, string, pgtype.Timestamptz, error) {
	id, err := uuid.Parse(list.ID)
	if err != nil {
		return pgtype.UUID{}, "", pgtype.Timestamptz{}, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	return uuidToPgtype(id), list.Title, timeToPgtype(list.CreateTime), nil
}

// taskStatusesToStrings converts domain TaskStatus slice to string slice for SQL queries.
func taskStatusesToStrings(statuses []domain.TaskStatus) []string {
	result := make([]string, len(statuses))
	for i, s := range statuses {
		result[i] = string(s)
	}
	return result
}

// taskPrioritiesToStrings converts domain TaskPriority slice to string slice for SQL queries.
func taskPrioritiesToStrings(priorities []domain.TaskPriority) []string {
	result := make([]string, len(priorities))
	for i, p := range priorities {
		result[i] = string(p)
	}
	return result
}

// === Todo Item Conversions ===

// todoItemFields holds the common fields present in both sqlcgen.TodoItem and sqlcgen.ListTasksWithFiltersRow
type todoItemFields struct {
	ID                  pgtype.UUID
	ListID              pgtype.UUID
	Title               string
	Status              string
	Priority            sql.Null[string]
	CreateTime          pgtype.Timestamptz
	UpdatedAt           pgtype.Timestamptz
	DueTime             sql.Null[time.Time]
	Timezone            sql.Null[string]
	EstimatedDuration   pgtype.Interval
	ActualDuration      pgtype.Interval
	Tags                []byte
	RecurringTemplateID uuid.NullUUID
	InstanceDate        sql.Null[time.Time]
	Version             int32
}

// convertTodoItemFields converts common todo item fields from database to domain model.
func convertTodoItemFields(fields todoItemFields) (domain.TodoItem, error) {
	item := domain.TodoItem{
		ID:         pgtypeToUUIDString(fields.ID),
		ListID:     pgtypeToUUIDString(fields.ListID),
		Title:      fields.Title,
		Status:     domain.TaskStatus(fields.Status),
		CreateTime: pgtypeToTime(fields.CreateTime),
		UpdatedAt:  pgtypeToTime(fields.UpdatedAt),
		DueTime:    nullTimeToPtr(fields.DueTime),    // DB sql.Null[time.Time] → Domain *time.Time
		Timezone:   nullStringToPtr(fields.Timezone), // DB sql.Null[string] → Domain *string
		Version:    int(fields.Version),
		Tags:       []string{},
	}

	// Priority: Keep as pointer in domain (custom enum type)
	if fields.Priority.Valid {
		priority := domain.TaskPriority(fields.Priority.V)
		item.Priority = &priority
	}

	// Estimated Duration
	if fields.EstimatedDuration.Valid {
		duration := intervalToDuration(fields.EstimatedDuration)
		item.EstimatedDuration = &duration
	}

	// Actual Duration
	if fields.ActualDuration.Valid {
		duration := intervalToDuration(fields.ActualDuration)
		item.ActualDuration = &duration
	}

	// Tags
	if len(fields.Tags) > 0 {
		var tags []string
		if err := json.Unmarshal(fields.Tags, &tags); err != nil {
			return domain.TodoItem{}, fmt.Errorf("invalid tags JSON for item %s: %w", pgtypeToUUIDString(fields.ID), err)
		}
		item.Tags = tags
	}

	// Recurring Template ID: DB uuid.NullUUID → Domain *string
	item.RecurringTemplateID = nullUUIDToStringPtr(fields.RecurringTemplateID)

	// Instance Date: DB sql.Null[time.Time] → Domain *time.Time
	item.InstanceDate = nullTimeToPtr(fields.InstanceDate)

	return item, nil
}

func dbTodoItemToDomain(dbItem sqlcgen.TodoItem) (domain.TodoItem, error) {
	return convertTodoItemFields(todoItemFields{
		ID:                  dbItem.ID,
		ListID:              dbItem.ListID,
		Title:               dbItem.Title,
		Status:              dbItem.Status,
		Priority:            dbItem.Priority,
		CreateTime:          dbItem.CreateTime,
		UpdatedAt:           dbItem.UpdatedAt,
		DueTime:             dbItem.DueTime,
		Timezone:            dbItem.Timezone,
		EstimatedDuration:   dbItem.EstimatedDuration,
		ActualDuration:      dbItem.ActualDuration,
		Tags:                dbItem.Tags,
		RecurringTemplateID: dbItem.RecurringTemplateID,
		InstanceDate:        dbItem.InstanceDate,
		Version:             dbItem.Version,
	})
}

// dbListTasksRowToDomain converts a ListTasksWithFiltersRow to a domain TodoItem.
// This is needed because the query includes COUNT(*) OVER() which creates a different struct.
func dbListTasksRowToDomain(dbItem sqlcgen.ListTasksWithFiltersRow) (domain.TodoItem, error) {
	return convertTodoItemFields(todoItemFields{
		ID:                  dbItem.ID,
		ListID:              dbItem.ListID,
		Title:               dbItem.Title,
		Status:              dbItem.Status,
		Priority:            dbItem.Priority,
		CreateTime:          dbItem.CreateTime,
		UpdatedAt:           dbItem.UpdatedAt,
		DueTime:             dbItem.DueTime,
		Timezone:            dbItem.Timezone,
		EstimatedDuration:   dbItem.EstimatedDuration,
		ActualDuration:      dbItem.ActualDuration,
		Tags:                dbItem.Tags,
		RecurringTemplateID: dbItem.RecurringTemplateID,
		InstanceDate:        dbItem.InstanceDate,
		Version:             dbItem.Version,
	})
}

func domainTodoItemToDB(item *domain.TodoItem, listID string) (sqlcgen.CreateTodoItemParams, error) {
	itemID, err := uuid.Parse(item.ID)
	if err != nil {
		return sqlcgen.CreateTodoItemParams{}, fmt.Errorf("%w: item %w", domain.ErrInvalidID, err)
	}

	listUUID, err := uuid.Parse(listID)
	if err != nil {
		return sqlcgen.CreateTodoItemParams{}, fmt.Errorf("%w: list %w", domain.ErrInvalidID, err)
	}

	params := sqlcgen.CreateTodoItemParams{
		ID:         uuidToPgtype(itemID),
		ListID:     uuidToPgtype(listUUID),
		Title:      item.Title,
		Status:     string(item.Status),
		CreateTime: timeToPgtype(item.CreateTime),
		UpdatedAt:  timeToPgtype(item.UpdatedAt),
		DueTime:    ptrToNullTime(item.DueTime),    // Domain *time.Time → DB sql.Null[time.Time]
		Timezone:   ptrToNullString(item.Timezone), // Domain *string → DB sql.Null[string]
	}

	// Priority: Convert from *TaskPriority to sql.Null[string]
	if item.Priority != nil {
		priorityStr := string(*item.Priority)
		params.Priority = sql.Null[string]{V: priorityStr, Valid: true}
	}

	// Estimated Duration
	if item.EstimatedDuration != nil {
		params.EstimatedDuration = durationToInterval(*item.EstimatedDuration)
	}

	// Actual Duration
	if item.ActualDuration != nil {
		params.ActualDuration = durationToInterval(*item.ActualDuration)
	}

	// Tags
	if len(item.Tags) > 0 {
		tagsJSON, err := json.Marshal(item.Tags)
		if err != nil {
			return params, fmt.Errorf("failed to marshal tags: %w", err)
		}
		params.Tags = tagsJSON
	}

	// Recurring Template ID: Domain *string → DB uuid.NullUUID
	params.RecurringTemplateID, err = stringPtrToNullUUID(item.RecurringTemplateID)
	if err != nil {
		return params, fmt.Errorf("invalid recurring template ID: %w", err)
	}

	// Instance Date: Domain *time.Time → DB sql.Null[time.Time]
	params.InstanceDate = ptrToNullTime(item.InstanceDate)

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
		Tags:                 []string{},
	}

	// Tags (now []byte in pgx)
	if len(dbTemplate.Tags) > 0 {
		var tags []string
		if err := json.Unmarshal(dbTemplate.Tags, &tags); err != nil {
			return nil, fmt.Errorf("invalid tags JSON for template %s: %w", pgtypeToUUIDString(dbTemplate.ID), err)
		}
		template.Tags = tags
	}

	// Priority: Convert from sql.Null[string] to *TaskPriority
	if dbTemplate.Priority.Valid {
		priority := domain.TaskPriority(dbTemplate.Priority.V)
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
		return sqlcgen.CreateRecurringTemplateParams{}, fmt.Errorf("%w: template %w", domain.ErrInvalidID, err)
	}

	listID, err := uuid.Parse(template.ListID)
	if err != nil {
		return sqlcgen.CreateRecurringTemplateParams{}, fmt.Errorf("%w: list %w", domain.ErrInvalidID, err)
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

	// Priority: Convert from *TaskPriority to sql.Null[string]
	if template.Priority != nil {
		priority := string(*template.Priority)
		params.Priority = sql.Null[string]{V: priority, Valid: true}
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
		StartedAt:     nullTimeToPtr(dbJob.StartedAt),      // DB sql.Null[time.Time] → Domain *time.Time
		CompletedAt:   nullTimeToPtr(dbJob.CompletedAt),    // DB sql.Null[time.Time] → Domain *time.Time
		FailedAt:      nullTimeToPtr(dbJob.FailedAt),       // DB sql.Null[time.Time] → Domain *time.Time
		ErrorMessage:  nullStringToPtr(dbJob.ErrorMessage), // DB sql.Null[string] → Domain *string
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

// === Batch Insert Conversions ===

// domainTodoItemsToBatchParams converts domain items to batch insert params.
func domainTodoItemsToBatchParams(items []domain.TodoItem, listID string) ([]sqlcgen.BatchCreateTodoItemsParams, error) {
	listUUID, err := uuid.Parse(listID)
	if err != nil {
		return nil, fmt.Errorf("%w: list %w", domain.ErrInvalidID, err)
	}
	listPgUUID := uuidToPgtype(listUUID)

	params := make([]sqlcgen.BatchCreateTodoItemsParams, 0, len(items))
	for i := range items {
		item := &items[i]

		itemID, err := uuid.Parse(item.ID)
		if err != nil {
			return nil, fmt.Errorf("%w: item %w", domain.ErrInvalidID, err)
		}

		p := sqlcgen.BatchCreateTodoItemsParams{
			ID:           uuidToPgtype(itemID),
			ListID:       listPgUUID,
			Title:        item.Title,
			Status:       string(item.Status),
			CreateTime:   timeToPgtype(item.CreateTime),
			UpdatedAt:    timeToPgtype(item.UpdatedAt),
			DueTime:      ptrToNullTime(item.DueTime),
			Timezone:     ptrToNullString(item.Timezone),
			InstanceDate: ptrToNullTime(item.InstanceDate),
			Version:      1, // New items always start at version 1
		}

		if item.Priority != nil {
			p.Priority = sql.Null[string]{V: string(*item.Priority), Valid: true}
		}

		if item.EstimatedDuration != nil {
			p.EstimatedDuration = durationToInterval(*item.EstimatedDuration)
		}

		if item.ActualDuration != nil {
			p.ActualDuration = durationToInterval(*item.ActualDuration)
		}

		if len(item.Tags) > 0 {
			tagsJSON, err := json.Marshal(item.Tags)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal tags for item %s: %w", item.ID, err)
			}
			p.Tags = tagsJSON
		}

		p.RecurringTemplateID, err = stringPtrToNullUUID(item.RecurringTemplateID)
		if err != nil {
			return nil, fmt.Errorf("invalid recurring template ID for item %s: %w", item.ID, err)
		}

		params = append(params, p)
	}

	return params, nil
}
