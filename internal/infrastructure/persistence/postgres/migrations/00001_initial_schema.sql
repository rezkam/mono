-- +goose Up
-- +goose StatementBegin

-- ============================================================================
-- Core Tables
-- ============================================================================

-- Todo Lists
CREATE TABLE todo_lists (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    title TEXT NOT NULL,
    create_time timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_todo_lists_create_time ON todo_lists(create_time DESC);

-- Todo Items (Tasks)
CREATE TABLE todo_items (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    list_id uuid NOT NULL,
    title TEXT NOT NULL,

    -- Status and priority
    status TEXT NOT NULL DEFAULT 'todo'
        CHECK (status IN ('todo', 'in_progress', 'blocked', 'done', 'archived', 'cancelled')),
    priority TEXT
        CHECK (priority IS NULL OR priority IN ('low', 'medium', 'high', 'urgent')),

    -- Time tracking
    estimated_duration INTERVAL,
    actual_duration INTERVAL,

    -- Timestamps
    create_time timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    due_time timestamptz,

    -- Tags stored as JSONB array
    tags jsonb,

    -- Link to recurring template (null if not a recurring instance)
    recurring_template_id uuid,
    instance_date DATE,  -- Anchor date for recurring instances

    -- Timezone for due_time interpretation
    -- NULL = floating time (9am stays 9am regardless of user's location)
    -- Non-NULL = fixed timezone (absolute moment in IANA timezone like 'Europe/Stockholm')
    timezone TEXT,

    FOREIGN KEY (list_id) REFERENCES todo_lists(id) ON DELETE CASCADE
);

-- Indexes for common query patterns
CREATE INDEX idx_todo_items_list_id ON todo_items(list_id);

CREATE INDEX idx_todo_items_status ON todo_items(status)
    WHERE status NOT IN ('archived', 'cancelled');

CREATE INDEX idx_todo_items_priority ON todo_items(priority)
    WHERE priority IS NOT NULL;

CREATE INDEX idx_todo_items_due_time ON todo_items(due_time)
    WHERE due_time IS NOT NULL;

CREATE INDEX idx_todo_items_updated_at ON todo_items(updated_at DESC);

CREATE INDEX idx_todo_items_estimated_duration ON todo_items(estimated_duration)
    WHERE estimated_duration IS NOT NULL;

-- GIN index for JSONB tags (fast tag queries)
CREATE INDEX idx_todo_items_tags_gin ON todo_items USING gin(tags)
    WHERE tags IS NOT NULL;

-- Timezone index for filtering tasks with fixed timezones
CREATE INDEX idx_todo_items_timezone ON todo_items(timezone)
    WHERE timezone IS NOT NULL;

-- Composite index for common query: active tasks by due date
CREATE INDEX idx_todo_items_active_due ON todo_items(status, due_time)
    WHERE status IN ('todo', 'in_progress', 'blocked');

CREATE INDEX idx_todo_items_recurring_template ON todo_items(recurring_template_id)
    WHERE recurring_template_id IS NOT NULL;

CREATE INDEX idx_todo_items_instance_date ON todo_items(instance_date)
    WHERE instance_date IS NOT NULL;

-- ============================================================================
-- Time Tracking
-- ============================================================================

-- Track all status transitions for time calculation and audit trail
CREATE TABLE task_status_history (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    task_id uuid NOT NULL,
    from_status TEXT,
    to_status TEXT NOT NULL
        CHECK (to_status IN ('todo', 'in_progress', 'blocked', 'done', 'archived', 'cancelled')),
    changed_at timestamptz NOT NULL DEFAULT now(),
    notes TEXT,

    FOREIGN KEY (task_id) REFERENCES todo_items(id) ON DELETE CASCADE
);

CREATE INDEX idx_status_history_task_time ON task_status_history(task_id, changed_at DESC);

CREATE INDEX idx_status_history_in_progress ON task_status_history(task_id, to_status, changed_at)
    WHERE to_status IN ('in_progress', 'blocked', 'done', 'cancelled');

-- ============================================================================
-- Recurring Tasks
-- ============================================================================

-- Templates for recurring tasks
CREATE TABLE recurring_task_templates (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    list_id uuid NOT NULL,

    -- Template fields (same structure as todo_items)
    title TEXT NOT NULL,
    tags jsonb,
    priority TEXT
        CHECK (priority IS NULL OR priority IN ('low', 'medium', 'high', 'urgent')),
    estimated_duration INTERVAL,

    -- Recurrence pattern
    recurrence_pattern TEXT NOT NULL
        CHECK (recurrence_pattern IN ('daily', 'weekly', 'biweekly', 'monthly', 'yearly', 'quarterly', 'weekdays')),

    -- Pattern-specific configuration (JSONB for flexibility)
    recurrence_config jsonb NOT NULL,
    /* Examples:
       DAILY: {"interval": 1}
       WEEKLY: {"interval": 1, "days_of_week": [1,3,5]}  -- Mon, Wed, Fri
       BIWEEKLY: {"days_of_week": [1,3,5]}  -- Mon, Wed, Fri every 2 weeks
       MONTHLY: {"interval": 1, "day_of_month": 15} or {"day_of_month": "last"}
       YEARLY: {"month": 3, "day": 15}  -- March 15
       QUARTERLY: {"month_offset": 0, "day": 1}  -- First day of quarter
       WEEKDAYS: {}  -- Mon-Fri
    */

    -- Relative due date offset from instance date
    due_offset INTERVAL,

    -- Template state
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- Generation tracking
    last_generated_until DATE NOT NULL DEFAULT CURRENT_DATE,
    generation_window_days INTEGER NOT NULL DEFAULT 30,

    FOREIGN KEY (list_id) REFERENCES todo_lists(id) ON DELETE CASCADE
);

-- Add FK from todo_items to recurring_task_templates
ALTER TABLE todo_items
    ADD CONSTRAINT fk_todo_items_recurring_template
    FOREIGN KEY (recurring_template_id) REFERENCES recurring_task_templates(id) ON DELETE SET NULL;

-- Indexes
CREATE INDEX idx_recurring_templates_active ON recurring_task_templates(is_active, last_generated_until)
    WHERE is_active = true;

CREATE INDEX idx_recurring_templates_list ON recurring_task_templates(list_id)
    WHERE is_active = true;

CREATE INDEX idx_recurring_templates_config_gin ON recurring_task_templates USING gin(recurrence_config);

-- ============================================================================
-- Job Queue for Recurring Task Generation
-- ============================================================================

CREATE TABLE recurring_generation_jobs (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    template_id uuid NOT NULL,

    -- Job scheduling
    -- DEFAULT now(): Prevents clock skew race condition between application and database.
    -- When a job should be immediately available, the application passes NULL which triggers
    -- this DEFAULT, ensuring the database's transaction timestamp is used as the single source
    -- of truth. This eliminates the race condition where time.Now() in Go might be slightly
    -- ahead of PostgreSQL's now(), causing jobs to be temporarily unclaimable despite being
    -- intended for immediate processing. For future-scheduled jobs, an explicit timestamp
    -- overrides this default.
    scheduled_for timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    completed_at timestamptz,
    failed_at timestamptz,

    -- Job state
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'completed', 'failed')),

    error_message TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,

    -- Generation range
    generate_from DATE NOT NULL,
    generate_until DATE NOT NULL,

    created_at timestamptz NOT NULL DEFAULT now(),

    FOREIGN KEY (template_id) REFERENCES recurring_task_templates(id) ON DELETE CASCADE
);

CREATE INDEX idx_generation_jobs_pending ON recurring_generation_jobs(scheduled_for, status)
    WHERE status = 'pending';

CREATE INDEX idx_generation_jobs_template ON recurring_generation_jobs(template_id, created_at DESC);

-- ============================================================================
-- Authentication
-- ============================================================================

CREATE TABLE api_keys (
    id uuid PRIMARY KEY DEFAULT uuidv7(),

    -- API Key structure: {key_type}-{service}-{version}-{short_token}-{long_secret}
    -- Example: sk-mono-v1-a7f3d8e2-8h3k2jf9s7d6f5g4h3j2k1
    --          └┘ └──┘ └┘  └──────┘  └────────────────────┘
    --          |   |    |      |              |
    --      key_type service version short    long secret (hashed)

    key_type TEXT NOT NULL,  -- Type of key (e.g., "sk" for secret key, "pk" for public key)
    service TEXT NOT NULL,  -- Service name (e.g., "mono")
    version TEXT NOT NULL,  -- API version (e.g., "v1", "v2")
    short_token VARCHAR(16) NOT NULL,  -- Short token for O(1) lookup (12 hex chars derived from BLAKE2b hash)
    long_secret_hash TEXT NOT NULL,  -- BLAKE2b-256 hash of the long secret part

    name TEXT NOT NULL,  -- Descriptive name (e.g., "Production API Key")
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    expires_at timestamptz,

    -- Ensure short_token is unique across all keys
    CONSTRAINT unique_short_token UNIQUE (short_token)
);

-- Index on short_token for O(1) lookups (industry standard pattern)
CREATE INDEX idx_api_keys_short_token ON api_keys(short_token)
    WHERE is_active = true;

-- Index on key_type for filtering different key types
CREATE INDEX idx_api_keys_type ON api_keys(key_type)
    WHERE is_active = true;

-- Index on version for filtering by API version
CREATE INDEX idx_api_keys_version ON api_keys(version)
    WHERE is_active = true;

-- ============================================================================
-- Functions and Triggers
-- ============================================================================

-- Function to auto-update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger for todo_items.updated_at
CREATE TRIGGER update_todo_items_updated_at
    BEFORE UPDATE ON todo_items
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Trigger for recurring_task_templates.updated_at
CREATE TRIGGER update_recurring_templates_updated_at
    BEFORE UPDATE ON recurring_task_templates
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Function to calculate actual time from IN_PROGRESS sessions
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
        IF rec.to_status = 'in_progress' AND rec.next_status IS NOT NULL THEN
            v_total_duration := v_total_duration + (rec.end_time - rec.start_time);
        END IF;
    END LOOP;

    RETURN v_total_duration;
END;
$$ LANGUAGE plpgsql;

-- Function and trigger to track status changes and update actual_duration
CREATE OR REPLACE FUNCTION update_actual_duration_on_status_change()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status != OLD.status THEN
        -- Insert status history record
        INSERT INTO task_status_history (task_id, from_status, to_status, changed_at)
        VALUES (NEW.id, OLD.status, NEW.status, now());

        -- Auto-calculate actual_duration when leaving in_progress
        -- (unless user manually set it - we'll add a flag for this later if needed)
        IF OLD.status = 'in_progress' AND NEW.status != 'in_progress' THEN
            NEW.actual_duration := calculate_actual_duration(NEW.id);
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER track_status_changes
    BEFORE UPDATE OF status ON todo_items
    FOR EACH ROW
    WHEN (OLD.status IS DISTINCT FROM NEW.status)
    EXECUTE FUNCTION update_actual_duration_on_status_change();

-- Function to track initial status on insert
CREATE OR REPLACE FUNCTION track_initial_status()
RETURNS TRIGGER AS $$
BEGIN
    -- Insert initial status history record
    INSERT INTO task_status_history (task_id, from_status, to_status, changed_at)
    VALUES (NEW.id, NULL, NEW.status, now());

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER track_initial_status_on_insert
    AFTER INSERT ON todo_items
    FOR EACH ROW
    EXECUTE FUNCTION track_initial_status();

-- Function to claim next generation job (using SKIP LOCKED for concurrent workers)
CREATE OR REPLACE FUNCTION claim_next_generation_job()
RETURNS uuid AS $$
DECLARE
    v_job_id uuid;
BEGIN
    SELECT id INTO v_job_id
    FROM recurring_generation_jobs
    WHERE status = 'pending'
        AND scheduled_for <= now()
    ORDER BY scheduled_for
    LIMIT 1
    FOR UPDATE SKIP LOCKED;

    IF v_job_id IS NOT NULL THEN
        UPDATE recurring_generation_jobs
        SET status = 'running',
            started_at = now()
        WHERE id = v_job_id;
    END IF;

    RETURN v_job_id;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Drop in reverse order to handle dependencies
DROP TRIGGER IF EXISTS track_initial_status_on_insert ON todo_items;
DROP TRIGGER IF EXISTS track_status_changes ON todo_items;
DROP TRIGGER IF EXISTS update_recurring_templates_updated_at ON recurring_task_templates;
DROP TRIGGER IF EXISTS update_todo_items_updated_at ON todo_items;

DROP FUNCTION IF EXISTS claim_next_generation_job();
DROP FUNCTION IF EXISTS track_initial_status();
DROP FUNCTION IF EXISTS update_actual_duration_on_status_change();
DROP FUNCTION IF EXISTS calculate_actual_duration(uuid);
DROP FUNCTION IF EXISTS update_updated_at_column();

DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS recurring_generation_jobs;
DROP TABLE IF EXISTS recurring_task_templates CASCADE;
DROP TABLE IF EXISTS task_status_history;
DROP TABLE IF EXISTS todo_items;
DROP TABLE IF EXISTS todo_lists;

-- +goose StatementEnd
