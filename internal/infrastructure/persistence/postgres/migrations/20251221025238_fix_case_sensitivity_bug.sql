-- +goose Up
-- +goose StatementBegin

-- ============================================================================
-- Fix Case Sensitivity Bug in Indexes and Functions
-- ============================================================================
--
-- PROBLEM:
-- - CHECK constraints enforce lowercase status values ('todo', 'in_progress', etc.)
-- - But indexes and functions used uppercase ('TODO', 'IN_PROGRESS')
-- - Result: Indexes match zero rows (useless), functions always return false
--
-- IMPACT:
-- - Performance: Full table scans instead of index usage
-- - Functionality: calculate_actual_duration() always returns 0
-- - Silent failure: No errors, just wrong behavior
--
-- FIX:
-- - Recreate indexes with lowercase WHERE clauses
-- - Replace functions with lowercase comparisons

-- ============================================================================
-- Fix Indexes on todo_items
-- ============================================================================

-- Fix idx_todo_items_status (line 57 in initial schema)
-- Was: WHERE status NOT IN ('ARCHIVED', 'CANCELLED')
-- Now: WHERE status NOT IN ('archived', 'cancelled')
DROP INDEX IF EXISTS idx_todo_items_status;
CREATE INDEX idx_todo_items_status ON todo_items(status)
    WHERE status NOT IN ('archived', 'cancelled');

-- Fix idx_todo_items_active_due (lines 79-80 in initial schema)
-- Was: WHERE status IN ('TODO', 'IN_PROGRESS', 'BLOCKED')
-- Now: WHERE status IN ('todo', 'in_progress', 'blocked')
DROP INDEX IF EXISTS idx_todo_items_active_due;
CREATE INDEX idx_todo_items_active_due ON todo_items(status, due_time)
    WHERE status IN ('todo', 'in_progress', 'blocked');

-- ============================================================================
-- Fix Indexes on task_status_history
-- ============================================================================

-- Fix idx_status_history_in_progress (lines 107-108 in initial schema)
-- Was: WHERE to_status IN ('IN_PROGRESS', 'BLOCKED', 'DONE', 'CANCELLED')
-- Now: WHERE to_status IN ('in_progress', 'blocked', 'done', 'cancelled')
DROP INDEX IF EXISTS idx_status_history_in_progress;
CREATE INDEX idx_status_history_in_progress ON task_status_history(task_id, to_status, changed_at)
    WHERE to_status IN ('in_progress', 'blocked', 'done', 'cancelled');

-- ============================================================================
-- Fix calculate_actual_duration Function
-- ============================================================================

-- Replace function with lowercase comparison
-- Was: IF rec.to_status = 'IN_PROGRESS'
-- Now: IF rec.to_status = 'in_progress'
CREATE OR REPLACE FUNCTION calculate_actual_duration(p_task_id uuid)
RETURNS INTERVAL AS $$
DECLARE
    v_total_duration INTERVAL := INTERVAL '0';
    rec RECORD;
BEGIN
    FOR rec IN (
        SELECT
            changed_at as start_time,
            LEAD(changed_at) OVER (ORDER BY changed_at) as end_time,
            to_status,
            LEAD(to_status) OVER (ORDER BY changed_at) as next_status
        FROM task_status_history
        WHERE task_id = p_task_id
        ORDER BY changed_at
    ) LOOP
        -- FIX: Use lowercase 'in_progress' instead of uppercase 'IN_PROGRESS'
        IF rec.to_status = 'in_progress' AND rec.next_status IS NOT NULL THEN
            v_total_duration := v_total_duration + (rec.end_time - rec.start_time);
        END IF;
    END LOOP;

    RETURN v_total_duration;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- Fix update_actual_duration_on_status_change Function
-- ============================================================================

-- Replace function with lowercase comparisons
-- Was: IF OLD.status = 'IN_PROGRESS' AND NEW.status != 'IN_PROGRESS'
-- Now: IF OLD.status = 'in_progress' AND NEW.status != 'in_progress'
CREATE OR REPLACE FUNCTION update_actual_duration_on_status_change()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status != OLD.status THEN
        -- Insert status history record
        INSERT INTO task_status_history (task_id, from_status, to_status, changed_at)
        VALUES (NEW.id, OLD.status, NEW.status, now());

        -- Auto-calculate actual_duration when leaving in_progress
        -- FIX: Use lowercase 'in_progress' instead of uppercase 'IN_PROGRESS'
        IF OLD.status = 'in_progress' AND NEW.status != 'in_progress' THEN
            NEW.actual_duration := calculate_actual_duration(NEW.id);
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- ============================================================================
-- Rollback: Restore Original (Broken) Indexes and Functions
-- ============================================================================

-- Restore broken indexes (uppercase WHERE clauses)
DROP INDEX IF EXISTS idx_todo_items_status;
CREATE INDEX idx_todo_items_status ON todo_items(status)
    WHERE status NOT IN ('ARCHIVED', 'CANCELLED');

DROP INDEX IF EXISTS idx_todo_items_active_due;
CREATE INDEX idx_todo_items_active_due ON todo_items(status, due_time)
    WHERE status IN ('TODO', 'IN_PROGRESS', 'BLOCKED');

DROP INDEX IF EXISTS idx_status_history_in_progress;
CREATE INDEX idx_status_history_in_progress ON task_status_history(task_id, to_status, changed_at)
    WHERE to_status IN ('IN_PROGRESS', 'BLOCKED', 'DONE', 'CANCELLED');

-- Restore broken functions (uppercase comparisons)
CREATE OR REPLACE FUNCTION calculate_actual_duration(p_task_id uuid)
RETURNS INTERVAL AS $$
DECLARE
    v_total_duration INTERVAL := INTERVAL '0';
    rec RECORD;
BEGIN
    FOR rec IN (
        SELECT
            changed_at as start_time,
            LEAD(changed_at) OVER (ORDER BY changed_at) as end_time,
            to_status,
            LEAD(to_status) OVER (ORDER BY changed_at) as next_status
        FROM task_status_history
        WHERE task_id = p_task_id
        ORDER BY changed_at
    ) LOOP
        IF rec.to_status = 'IN_PROGRESS' AND rec.next_status IS NOT NULL THEN
            v_total_duration := v_total_duration + (rec.end_time - rec.start_time);
        END IF;
    END LOOP;

    RETURN v_total_duration;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_actual_duration_on_status_change()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status != OLD.status THEN
        -- Insert status history record
        INSERT INTO task_status_history (task_id, from_status, to_status, changed_at)
        VALUES (NEW.id, OLD.status, NEW.status, now());

        -- Auto-calculate actual_duration when leaving IN_PROGRESS
        IF OLD.status = 'IN_PROGRESS' AND NEW.status != 'IN_PROGRESS' THEN
            NEW.actual_duration := calculate_actual_duration(NEW.id);
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd
