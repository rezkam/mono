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
	"github.com/rezkam/mono/internal/ptr"
)

// === pgtype Conversion Helpers ===
// Note: With sqlc configured to use standard Go types, most pgtype conversions are obsolete.
// However, some query parameters that aren't tied to specific columns still use pgtype.
// Only these minimal conversions remain for query parameters and duration conversion.

// uuidToQueryParam converts uuid.UUID to pgtype.UUID for query parameters.
// Used only for queries where sqlc cannot infer column type from parameter usage.
func uuidToQueryParam(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// timePtrToQueryParam converts *time.Time to pgtype.Timestamptz for query parameters.
// For nil pointers, returns zero time (0001-01-01) to enable SQL skip-filter patterns like:
//
//	($5::timestamptz = '0001-01-01 00:00:00+00' OR due_at <= $5)
//
// With NULL, both sides of the OR would return NULL (falsy), breaking the pattern.
func timePtrToQueryParam(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		// Return zero time to match SQL's skip-filter pattern
		return pgtype.Timestamptz{Time: time.Time{}, Valid: true}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
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

// === sql.Null[T] ↔ Pointer Conversion Helpers ===
// These helpers translate between database sql.Null[T] and domain pointers (*T).
// Used ONLY in infrastructure layer - domain layer must never import database/sql.

// nullTimeToPtr converts sql.Null[time.Time] from database to *time.Time for domain.
// Always returns time in UTC location for consistent timezone handling.
func nullTimeToPtr(n sql.Null[time.Time]) *time.Time {
	if !n.Valid {
		return nil
	}
	return ptr.To(n.V.UTC())
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
	return ptr.To(n.V)
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
	return ptr.To(n.UUID.String())
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
		ID:             dbKey.ID,
		KeyType:        dbKey.KeyType,
		Service:        dbKey.Service,
		Version:        dbKey.Version,
		ShortToken:     dbKey.ShortToken,
		LongSecretHash: dbKey.LongSecretHash,
		Name:           dbKey.Name,
		IsActive:       dbKey.IsActive,
		CreatedAt:      dbKey.CreatedAt.UTC(),
		LastUsedAt:     nullTimeToPtr(dbKey.LastUsedAt), // DB sql.Null[time.Time] → Domain *time.Time
		ExpiresAt:      nullTimeToPtr(dbKey.ExpiresAt),  // DB sql.Null[time.Time] → Domain *time.Time
	}
}

// === Todo List Conversions ===

func domainTodoListToDB(list *domain.TodoList) (string, string, time.Time, error) {
	if _, err := uuid.Parse(list.ID); err != nil {
		return "", "", time.Time{}, fmt.Errorf("%w: %w", domain.ErrInvalidID, err)
	}

	return list.ID, list.Title, list.CreatedAt, nil
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
	ID                  string
	ListID              string
	Title               string
	Status              string
	Priority            sql.Null[string]
	CreatedAt           pgtype.Timestamptz
	UpdatedAt           time.Time
	DueAt               pgtype.Timestamptz
	Timezone            sql.Null[string]
	EstimatedDuration   pgtype.Interval
	ActualDuration      pgtype.Interval
	Tags                []byte
	RecurringTemplateID uuid.NullUUID
	StartsAt            pgtype.Date
	OccursAt            pgtype.Timestamptz
	DueOffset           pgtype.Interval
	Version             int32
}

// convertTodoItemFields converts common todo item fields from database to domain model.
func convertTodoItemFields(fields todoItemFields) (domain.TodoItem, error) {
	item := domain.TodoItem{
		ID:        fields.ID,
		ListID:    fields.ListID,
		Title:     fields.Title,
		Status:    domain.TaskStatus(fields.Status),
		CreatedAt: timestamptzToTime(fields.CreatedAt),
		UpdatedAt: fields.UpdatedAt.UTC(),
		DueAt:     pgtypeTimestamptzToTimePtr(fields.DueAt), // DB pgtype.Timestamptz → Domain *time.Time
		Timezone:  nullStringToPtr(fields.Timezone),         // DB sql.Null[string] → Domain *string
		Version:   int(fields.Version),
		Tags:      []string{},
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
			return domain.TodoItem{}, fmt.Errorf("invalid tags JSON for item %s: %w", fields.ID, err)
		}
		item.Tags = tags
	}

	// Recurring Template ID: DB uuid.NullUUID → Domain *string
	item.RecurringTemplateID = nullUUIDToStringPtr(fields.RecurringTemplateID)

	// StartsAt: DB pgtype.Date → Domain *time.Time
	item.StartsAt = pgtypeDateToTimePtr(fields.StartsAt)

	// OccursAt: DB pgtype.Timestamptz → Domain *time.Time
	item.OccursAt = pgtypeTimestamptzToTimePtr(fields.OccursAt)

	// DueOffset: DB pgtype.Interval → Domain *time.Duration
	item.DueOffset = pgtypeIntervalToDurationPtr(fields.DueOffset)

	return item, nil
}

func dbTodoItemToDomain(dbItem sqlcgen.TodoItem) (domain.TodoItem, error) {
	return convertTodoItemFields(todoItemFields{
		ID:                  dbItem.ID,
		ListID:              dbItem.ListID,
		Title:               dbItem.Title,
		Status:              dbItem.Status,
		Priority:            dbItem.Priority,
		CreatedAt:           dbItem.CreatedAt,
		UpdatedAt:           dbItem.UpdatedAt,
		DueAt:               dbItem.DueAt,
		Timezone:            dbItem.Timezone,
		EstimatedDuration:   dbItem.EstimatedDuration,
		ActualDuration:      dbItem.ActualDuration,
		Tags:                dbItem.Tags,
		RecurringTemplateID: dbItem.RecurringTemplateID,
		StartsAt:            dbItem.StartsAt,
		OccursAt:            dbItem.OccursAt,
		DueOffset:           dbItem.DueOffset,
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
		CreatedAt:           dbItem.CreatedAt,
		UpdatedAt:           dbItem.UpdatedAt,
		DueAt:               dbItem.DueAt,
		Timezone:            dbItem.Timezone,
		EstimatedDuration:   dbItem.EstimatedDuration,
		ActualDuration:      dbItem.ActualDuration,
		Tags:                dbItem.Tags,
		RecurringTemplateID: dbItem.RecurringTemplateID,
		StartsAt:            dbItem.StartsAt,
		OccursAt:            dbItem.OccursAt,
		DueOffset:           dbItem.DueOffset,
		Version:             dbItem.Version,
	})
}

func domainTodoItemToDB(item *domain.TodoItem, listID string) (sqlcgen.CreateTodoItemParams, error) {
	if _, err := uuid.Parse(item.ID); err != nil {
		return sqlcgen.CreateTodoItemParams{}, fmt.Errorf("%w: item %w", domain.ErrInvalidID, err)
	}

	if _, err := uuid.Parse(listID); err != nil {
		return sqlcgen.CreateTodoItemParams{}, fmt.Errorf("%w: list %w", domain.ErrInvalidID, err)
	}

	params := sqlcgen.CreateTodoItemParams{
		ID:        item.ID,
		ListID:    listID,
		Title:     item.Title,
		Status:    string(item.Status),
		CreatedAt: timeToTimestamptz(item.CreatedAt),
		UpdatedAt: item.UpdatedAt,
		DueAt:     timePtrToTimestamptz(item.DueAt), // Domain *time.Time → DB pgtype.Timestamptz
		Timezone:  ptrToNullString(item.Timezone),   // Domain *string → DB sql.Null[string]
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
	recurringTemplateID, err := stringPtrToNullUUID(item.RecurringTemplateID)
	if err != nil {
		return params, fmt.Errorf("invalid recurring template ID: %w", err)
	}
	params.RecurringTemplateID = recurringTemplateID

	// StartsAt: Domain *time.Time → DB pgtype.Date
	if item.StartsAt != nil {
		params.StartsAt = timeToDate(*item.StartsAt)
	}

	// OccursAt: Domain *time.Time → DB pgtype.Timestamptz
	if item.OccursAt != nil {
		params.OccursAt = timeToTimestamptz(*item.OccursAt)
	}

	// DueOffset: Domain *time.Duration → DB pgtype.Interval
	params.DueOffset = durationPtrToPgtypeInterval(item.DueOffset)

	return params, nil
}

// === Recurring Template Conversions ===

func dbRecurringTemplateToDomain(dbTemplate sqlcgen.RecurringTaskTemplate) (*domain.RecurringTemplate, error) {
	template := &domain.RecurringTemplate{
		ID:                    dbTemplate.ID,
		ListID:                dbTemplate.ListID,
		Title:                 dbTemplate.Title,
		RecurrencePattern:     domain.RecurrencePattern(dbTemplate.RecurrencePattern),
		IsActive:              dbTemplate.IsActive,
		CreatedAt:             dbTemplate.CreatedAt.UTC(),
		UpdatedAt:             dbTemplate.UpdatedAt.UTC(),
		GeneratedThrough:      pgtypeDateToTime(dbTemplate.GeneratedThrough),
		SyncHorizonDays:       int(dbTemplate.SyncHorizonDays),
		GenerationHorizonDays: int(dbTemplate.GenerationHorizonDays),
		Version:               int(dbTemplate.Version),
		Tags:                  []string{},
	}

	// Tags (now []byte in pgx)
	if len(dbTemplate.Tags) > 0 {
		var tags []string
		if err := json.Unmarshal(dbTemplate.Tags, &tags); err != nil {
			return nil, fmt.Errorf("invalid tags JSON for template %s: %w", dbTemplate.ID, err)
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
		var config map[string]any
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
	if _, err := uuid.Parse(template.ID); err != nil {
		return sqlcgen.CreateRecurringTemplateParams{}, fmt.Errorf("%w: template %w", domain.ErrInvalidID, err)
	}

	if _, err := uuid.Parse(template.ListID); err != nil {
		return sqlcgen.CreateRecurringTemplateParams{}, fmt.Errorf("%w: list %w", domain.ErrInvalidID, err)
	}

	params := sqlcgen.CreateRecurringTemplateParams{
		ID:                    template.ID,
		ListID:                template.ListID,
		Title:                 template.Title,
		RecurrencePattern:     string(template.RecurrencePattern),
		IsActive:              template.IsActive,
		CreatedAt:             template.CreatedAt,
		UpdatedAt:             template.UpdatedAt,
		GeneratedThrough:      timeToDate(template.GeneratedThrough),
		SyncHorizonDays:       int32(template.SyncHorizonDays),
		GenerationHorizonDays: int32(template.GenerationHorizonDays),
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
		ID:            dbJob.ID,
		TemplateID:    dbJob.TemplateID,
		ScheduledFor:  dbJob.ScheduledFor.UTC(),
		Status:        dbJob.Status,
		GenerateFrom:  dbJob.GenerateFrom,
		GenerateUntil: dbJob.GenerateUntil,
		CreatedAt:     dbJob.CreatedAt.UTC(),
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
	if _, err := uuid.Parse(listID); err != nil {
		return nil, fmt.Errorf("%w: list %w", domain.ErrInvalidID, err)
	}

	params := make([]sqlcgen.BatchCreateTodoItemsParams, 0, len(items))
	for i := range items {
		item := &items[i]

		if _, err := uuid.Parse(item.ID); err != nil {
			return nil, fmt.Errorf("%w: item %w", domain.ErrInvalidID, err)
		}

		p := sqlcgen.BatchCreateTodoItemsParams{
			ID:        item.ID,
			ListID:    listID,
			Title:     item.Title,
			Status:    string(item.Status),
			CreatedAt: timeToTimestamptz(item.CreatedAt),
			UpdatedAt: item.UpdatedAt,
			DueAt:     timePtrToTimestamptz(item.DueAt),
			Timezone:  ptrToNullString(item.Timezone),
			Version:   1, // New items always start at version 1
		}

		// Handle new recurring fields
		if item.StartsAt != nil {
			p.StartsAt = timeToDate(*item.StartsAt)
		}
		if item.OccursAt != nil {
			p.OccursAt = timeToTimestamptz(*item.OccursAt)
		}
		if item.DueOffset != nil {
			p.DueOffset = durationToInterval(*item.DueOffset)
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

		recurringTemplateID, err := stringPtrToNullUUID(item.RecurringTemplateID)
		if err != nil {
			return nil, fmt.Errorf("invalid recurring template ID for item %s: %w", item.ID, err)
		}
		p.RecurringTemplateID = recurringTemplateID

		params = append(params, p)
	}

	return params, nil
}

// === Additional Helpers for Hybrid Recurring Refactoring ===

// jsonMarshalHelper marshals any value to JSON bytes.
func jsonMarshalHelper(v any) ([]byte, error) {
	return json.Marshal(v)
}

// timeToDate converts time.Time to pgtype.Date for database DATE columns.
func timeToDate(t time.Time) pgtype.Date {
	return pgtype.Date{
		Time:  t,
		Valid: true,
	}
}

// timeToTimestamptz converts time.Time to pgtype.Timestamptz for database TIMESTAMPTZ columns.
func timeToTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{
		Time:  t,
		Valid: true,
	}
}

// timePtrToTimestamptz converts *time.Time to pgtype.Timestamptz for nullable TIMESTAMPTZ columns.
// Returns invalid timestamptz when pointer is nil.
func timePtrToTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// pgtypeDateToTime converts pgtype.Date to time.Time for domain models.
// Returns zero time if the date is not valid.
func pgtypeDateToTime(d pgtype.Date) time.Time {
	if !d.Valid {
		return time.Time{}
	}
	return d.Time
}

// pgtypeDateToTimePtr converts pgtype.Date to *time.Time for domain models.
func pgtypeDateToTimePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	utcTime := d.Time.UTC()
	return &utcTime
}

// pgtypeTimestamptzToTimePtr converts pgtype.Timestamptz to *time.Time for domain models.
func pgtypeTimestamptzToTimePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	utcTime := ts.Time.UTC()
	return &utcTime
}

// pgtypeIntervalToDurationPtr converts pgtype.Interval to *time.Duration for domain models.
func pgtypeIntervalToDurationPtr(interval pgtype.Interval) *time.Duration {
	if !interval.Valid {
		return nil
	}
	duration := intervalToDuration(interval)
	return &duration
}

// === Exception Converters ===

// dbExceptionToDomain converts database exception to domain model.
func dbExceptionToDomain(dbExc sqlcgen.RecurringTemplateException) (*domain.RecurringTemplateException, error) {
	return &domain.RecurringTemplateException{
		ID:            dbExc.ID.String(),
		TemplateID:    dbExc.TemplateID.String(),
		OccursAt:      dbExc.OccursAt.Time,
		ExceptionType: domain.ExceptionType(dbExc.ExceptionType),
		ItemID:        uuidToStringPtr(dbExc.ItemID),
		CreatedAt:     dbExc.CreatedAt.Time,
	}, nil
}

// stringPtrToText converts *string to pgtype.Text for nullable UUID fields.
// uuidToStringPtr converts pgtype.UUID to *string for nullable UUID fields.
func uuidToStringPtr(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}
	s := u.String()
	return &s
}
