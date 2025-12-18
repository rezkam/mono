package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/rezkam/mono/internal/storage/sql/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	pgURL := os.Getenv("TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("TEST_POSTGRES_URL not set, skipping integration tests")
	}

	db, err := sql.Open("pgx", pgURL)
	require.NoError(t, err)

	// Ping to verify connection
	require.NoError(t, db.Ping())

	// Run migrations
	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(db, "."))

	cleanup := func() {
		// Truncate all tables
		db.Exec("TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
		db.Close()
	}

	return db, cleanup
}

func TestSchema_AllTablesExist(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	tables := []string{
		"todo_lists",
		"todo_items",
		"task_status_history",
		"recurring_task_templates",
		"recurring_generation_jobs",
		"api_keys",
		"goose_db_version", // Migration tracking table
	}

	for _, table := range tables {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.tables 
				WHERE table_schema = 'public' AND table_name = $1
			)
		`, table).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "table %s should exist", table)
	}
}

func TestSchema_CriticalIndexesExist(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	indexes := []string{
		"idx_todo_items_list_id",
		"idx_todo_items_status",
		"idx_todo_items_priority",
		"idx_todo_items_due_time",
		"idx_todo_items_updated_at",
		"idx_todo_items_tags_gin",
		"idx_todo_items_active_due",
		"idx_recurring_templates_active",
	}

	for _, index := range indexes {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT FROM pg_indexes 
				WHERE schemaname = 'public' AND indexname = $1
			)
		`, index).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "index %s should exist", index)
	}
}

func TestSchema_FunctionsExist(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	functions := []string{
		"update_updated_at_column",
		"calculate_actual_duration",
		"update_actual_duration_on_status_change",
		"claim_next_generation_job",
	}

	for _, fn := range functions {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (SELECT FROM pg_proc WHERE proname = $1)
		`, fn).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "function %s should exist", fn)
	}
}

func TestCRUD_TodoLists(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create
	listID := "550e8400-e29b-41d4-a716-446655440000"
	_, err := db.ExecContext(ctx, `
		INSERT INTO todo_lists (id, title, create_time)
		VALUES ($1, $2, $3)
	`, listID, "Test List", time.Now())
	require.NoError(t, err)

	// Read
	var title string
	err = db.QueryRowContext(ctx, `
		SELECT title FROM todo_lists WHERE id = $1
	`, listID).Scan(&title)
	require.NoError(t, err)
	assert.Equal(t, "Test List", title)

	// Update
	_, err = db.ExecContext(ctx, `
		UPDATE todo_lists SET title = $1 WHERE id = $2
	`, "Updated List", listID)
	require.NoError(t, err)

	err = db.QueryRowContext(ctx, `
		SELECT title FROM todo_lists WHERE id = $1
	`, listID).Scan(&title)
	require.NoError(t, err)
	assert.Equal(t, "Updated List", title)

	// Delete
	_, err = db.ExecContext(ctx, "DELETE FROM todo_lists WHERE id = $1", listID)
	require.NoError(t, err)

	err = db.QueryRowContext(ctx, `
		SELECT title FROM todo_lists WHERE id = $1
	`, listID).Scan(&title)
	assert.Equal(t, sql.ErrNoRows, err)
}

func TestCRUD_TodoItems_WithNewFields(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create list
	listID := "550e8400-e29b-41d4-a716-446655440001"
	_, err := db.ExecContext(ctx, `
		INSERT INTO todo_lists (id, title, create_time) VALUES ($1, $2, $3)
	`, listID, "Test List", time.Now())
	require.NoError(t, err)

	// Create item with all new fields
	itemID := "550e8400-e29b-41d4-a716-446655440002"
	dueTime := time.Now().Add(24 * time.Hour)
	tags := `["urgent", "bug", "backend"]`

	_, err = db.ExecContext(ctx, `
		INSERT INTO todo_items (
			id, list_id, title, status, priority,
			estimated_duration, create_time, updated_at, due_time, tags
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, itemID, listID, "Fix Auth Bug", "TODO", "URGENT",
		"3 hours", time.Now(), time.Now(), dueTime, tags)
	require.NoError(t, err)

	// Read and verify all fields
	var (
		title             string
		status            string
		priority          sql.NullString
		estimatedDuration sql.NullString
		actualDuration    sql.NullString
		tagsJSON          []byte
		retrievedDueTime  sql.NullTime
	)

	err = db.QueryRowContext(ctx, `
		SELECT title, status, priority, estimated_duration, actual_duration, tags, due_time
		FROM todo_items WHERE id = $1
	`, itemID).Scan(&title, &status, &priority, &estimatedDuration, &actualDuration, &tagsJSON, &retrievedDueTime)
	require.NoError(t, err)

	assert.Equal(t, "Fix Auth Bug", title)
	assert.Equal(t, "TODO", status)
	assert.Equal(t, "URGENT", priority.String)
	assert.True(t, estimatedDuration.Valid)
	assert.Contains(t, estimatedDuration.String, "03:00:00")

	var parsedTags []string
	require.NoError(t, json.Unmarshal(tagsJSON, &parsedTags))
	assert.Contains(t, parsedTags, "urgent")
	assert.Contains(t, parsedTags, "bug")
	assert.Contains(t, parsedTags, "backend")
}

func TestTrigger_AutoUpdateTimestamp(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Setup
	listID := "550e8400-e29b-41d4-a716-446655440003"
	itemID := "550e8400-e29b-41d4-a716-446655440004"

	_, err := db.ExecContext(ctx, `
		INSERT INTO todo_lists (id, title, create_time) VALUES ($1, $2, $3)
	`, listID, "Test List", time.Now())
	require.NoError(t, err)

	now := time.Now()
	_, err = db.ExecContext(ctx, `
		INSERT INTO todo_items (id, list_id, title, status, create_time, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, itemID, listID, "Test", "TODO", now, now)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	// Update - trigger should auto-update updated_at
	_, err = db.ExecContext(ctx, `
		UPDATE todo_items SET title = $1 WHERE id = $2
	`, "Updated", itemID)
	require.NoError(t, err)

	var updatedAt time.Time
	err = db.QueryRowContext(ctx, `
		SELECT updated_at FROM todo_items WHERE id = $1
	`, itemID).Scan(&updatedAt)
	require.NoError(t, err)

	assert.True(t, updatedAt.After(now), "updated_at should be newer")
}

func TestTrigger_StatusHistory(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Setup
	listID := "550e8400-e29b-41d4-a716-446655440005"
	itemID := "550e8400-e29b-41d4-a716-446655440006"

	_, err := db.ExecContext(ctx, `
		INSERT INTO todo_lists (id, title, create_time) VALUES ($1, $2, $3)
	`, listID, "Test List", time.Now())
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO todo_items (id, list_id, title, status, create_time, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, itemID, listID, "Test", "TODO", time.Now(), time.Now())
	require.NoError(t, err)

	// Verify initial status was recorded
	var initialFromStatus sql.NullString
	var initialToStatus string
	err = db.QueryRowContext(ctx, `
		SELECT from_status, to_status
		FROM task_status_history
		WHERE task_id = $1
		ORDER BY changed_at
		LIMIT 1
	`, itemID).Scan(&initialFromStatus, &initialToStatus)
	require.NoError(t, err)

	assert.False(t, initialFromStatus.Valid, "initial from_status should be NULL")
	assert.Equal(t, "TODO", initialToStatus)

	// Change status - should create history entry
	_, err = db.ExecContext(ctx, `
		UPDATE todo_items SET status = $1 WHERE id = $2
	`, "IN_PROGRESS", itemID)
	require.NoError(t, err)

	// Verify transition history
	var fromStatus sql.NullString
	var toStatus string
	err = db.QueryRowContext(ctx, `
		SELECT from_status, to_status
		FROM task_status_history
		WHERE task_id = $1 AND from_status IS NOT NULL
		ORDER BY changed_at
		LIMIT 1
	`, itemID).Scan(&fromStatus, &toStatus)
	require.NoError(t, err)

	assert.Equal(t, "TODO", fromStatus.String)
	assert.Equal(t, "IN_PROGRESS", toStatus)

	// Change again
	_, err = db.ExecContext(ctx, `
		UPDATE todo_items SET status = $1 WHERE id = $2
	`, "DONE", itemID)
	require.NoError(t, err)

	// Count history entries (should be 3: initial + 2 transitions)
	var count int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM task_status_history WHERE task_id = $1
	`, itemID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestQuery_FilterByStatusAndPriority(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Setup
	listID := "550e8400-e29b-41d4-a716-446655440007"
	_, err := db.ExecContext(ctx, `
		INSERT INTO todo_lists (id, title, create_time) VALUES ($1, $2, $3)
	`, listID, "Test List", time.Now())
	require.NoError(t, err)

	// Create test items
	testItems := []struct {
		id       string
		status   string
		priority string
	}{
		{"550e8400-e29b-41d4-a716-446655440100", "TODO", "HIGH"},
		{"550e8400-e29b-41d4-a716-446655440101", "TODO", "LOW"},
		{"550e8400-e29b-41d4-a716-446655440102", "IN_PROGRESS", "HIGH"},
		{"550e8400-e29b-41d4-a716-446655440103", "DONE", "HIGH"},
		{"550e8400-e29b-41d4-a716-446655440104", "ARCHIVED", "HIGH"},
	}

	for _, item := range testItems {
		_, err = db.ExecContext(ctx, `
			INSERT INTO todo_items (id, list_id, title, status, priority, create_time, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, item.id, listID, "Task", item.status, item.priority, time.Now(), time.Now())
		require.NoError(t, err)
	}

	// Query: Active (not DONE/ARCHIVED/CANCELLED) + HIGH priority
	rows, err := db.QueryContext(ctx, `
		SELECT id FROM todo_items
		WHERE list_id = $1
		  AND status NOT IN ('DONE', 'ARCHIVED', 'CANCELLED')
		  AND priority = 'HIGH'
		ORDER BY status
	`, listID)
	require.NoError(t, err)
	defer rows.Close()

	var results []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		results = append(results, id)
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, 2, len(results), "should find 2 active HIGH priority items")
}

func TestQuery_TagsWithJSONB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	listID := "550e8400-e29b-41d4-a716-446655440008"
	_, err := db.ExecContext(ctx, `
		INSERT INTO todo_lists (id, title, create_time) VALUES ($1, $2, $3)
	`, listID, "Test List", time.Now())
	require.NoError(t, err)

	// Create items with different tag combinations
	testCases := []struct {
		id   string
		tags string
	}{
		{"550e8400-e29b-41d4-a716-446655440200", `["urgent", "bug"]`},
		{"550e8400-e29b-41d4-a716-446655440201", `["urgent", "feature"]`},
		{"550e8400-e29b-41d4-a716-446655440202", `["feature", "backend"]`},
		{"550e8400-e29b-41d4-a716-446655440203", `["frontend"]`},
	}

	for _, tc := range testCases {
		_, err = db.ExecContext(ctx, `
			INSERT INTO todo_items (id, list_id, title, status, create_time, updated_at, tags)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, tc.id, listID, "Task", "TODO", time.Now(), time.Now(), tc.tags)
		require.NoError(t, err)
	}

	// Test JSONB contains operator (@>)
	rows, err := db.QueryContext(ctx, `
		SELECT id FROM todo_items WHERE tags @> $1
	`, `["urgent"]`)
	require.NoError(t, err)
	defer rows.Close()

	var count int
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		count++
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, 2, count, "should find 2 items with 'urgent' tag")
}

func TestRecurringTemplate_Create(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	listID := "550e8400-e29b-41d4-a716-446655440009"
	_, err := db.ExecContext(ctx, `
		INSERT INTO todo_lists (id, title, create_time) VALUES ($1, $2, $3)
	`, listID, "Test List", time.Now())
	require.NoError(t, err)

	templateID := "550e8400-e29b-41d4-a716-446655440010"
	config := `{"interval": 1, "days_of_week": [1, 3, 5]}`

	_, err = db.ExecContext(ctx, `
		INSERT INTO recurring_task_templates (
			id, list_id, title, recurrence_pattern, recurrence_config,
			due_offset, is_active, created_at, updated_at,
			last_generated_until, generation_window_days
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, templateID, listID, "Daily Standup", "WEEKLY", config,
		"2 hours", true, time.Now(), time.Now(), time.Now(), 30)
	require.NoError(t, err)

	// Verify
	var title, pattern string
	var configJSON []byte
	err = db.QueryRowContext(ctx, `
		SELECT title, recurrence_pattern, recurrence_config
		FROM recurring_task_templates WHERE id = $1
	`, templateID).Scan(&title, &pattern, &configJSON)
	require.NoError(t, err)

	assert.Equal(t, "Daily Standup", title)
	assert.Equal(t, "WEEKLY", pattern)

	var parsedConfig map[string]interface{}
	require.NoError(t, json.Unmarshal(configJSON, &parsedConfig))
	assert.Equal(t, float64(1), parsedConfig["interval"])
}

func TestFunction_ClaimGenerationJob(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Setup
	listID := "550e8400-e29b-41d4-a716-446655440011"
	templateID := "550e8400-e29b-41d4-a716-446655440012"

	_, err := db.ExecContext(ctx, `
		INSERT INTO todo_lists (id, title, create_time) VALUES ($1, $2, $3)
	`, listID, "Test List", time.Now())
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO recurring_task_templates (
			id, list_id, title, recurrence_pattern, recurrence_config,
			is_active, created_at, updated_at, last_generated_until, generation_window_days
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, templateID, listID, "Template", "DAILY", `{}`,
		true, time.Now(), time.Now(), time.Now(), 30)
	require.NoError(t, err)

	// Create pending job
	jobID := "550e8400-e29b-41d4-a716-446655440013"
	_, err = db.ExecContext(ctx, `
		INSERT INTO recurring_generation_jobs (
			id, template_id, scheduled_for, status,
			generate_from, generate_until, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, jobID, templateID, time.Now().Add(-1*time.Hour), "PENDING",
		time.Now(), time.Now().Add(30*24*time.Hour), time.Now())
	require.NoError(t, err)

	// Claim job using function
	var claimedID sql.NullString
	err = db.QueryRowContext(ctx, "SELECT claim_next_generation_job()").Scan(&claimedID)
	require.NoError(t, err)
	assert.True(t, claimedID.Valid)
	assert.Equal(t, jobID, claimedID.String)

	// Verify status changed
	var status string
	err = db.QueryRowContext(ctx, `
		SELECT status FROM recurring_generation_jobs WHERE id = $1
	`, jobID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "RUNNING", status)
}

func TestCascadeDelete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	listID := "550e8400-e29b-41d4-a716-446655440014"
	itemID := "550e8400-e29b-41d4-a716-446655440015"

	_, err := db.ExecContext(ctx, `
		INSERT INTO todo_lists (id, title, create_time) VALUES ($1, $2, $3)
	`, listID, "Test List", time.Now())
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO todo_items (id, list_id, title, status, create_time, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, itemID, listID, "Test", "TODO", time.Now(), time.Now())
	require.NoError(t, err)

	// Delete list - should cascade
	_, err = db.ExecContext(ctx, "DELETE FROM todo_lists WHERE id = $1", listID)
	require.NoError(t, err)

	// Verify item was deleted
	var count int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM todo_items WHERE id = $1
	`, itemID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
