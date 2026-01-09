-- +goose Up
-- +goose StatementBegin

-- ============================================================================
-- Core Tables
-- ============================================================================

-- Todo Lists
CREATE TABLE todo_lists (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    title TEXT NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    version INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX idx_todo_lists_created_at ON todo_lists(created_at DESC);

-- Todo Items (Tasks)
CREATE TABLE todo_items (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    list_id uuid NOT NULL,
    title TEXT NOT NULL,

    -- Status and priority (using TEXT with CHECK constraints)
    status TEXT NOT NULL DEFAULT 'todo'
        CHECK (status IN ('todo', 'in_progress', 'blocked', 'done', 'archived', 'cancelled')),
    priority TEXT
        CHECK (priority IS NULL OR priority IN ('low', 'medium', 'high', 'urgent')),

    -- Time tracking
    estimated_duration INTERVAL,
    actual_duration INTERVAL,

    -- Timestamps
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    due_at timestamptz,

    -- Tags stored as TEXT array for native PostgreSQL array operations
    -- Query patterns:
    --   tags @> ARRAY['work']           -- has tag "work"
    --   tags @> ARRAY['work', 'urgent'] -- has ALL tags (AND)
    --   tags && ARRAY['work', 'urgent'] -- has ANY tag (OR)
    tags TEXT[],

    -- Recurring task link
    recurring_template_id uuid,

    -- Scheduling fields
    -- starts_at: when the task becomes active/visible
    starts_at DATE,

    -- occurs_at: exact timestamp this recurring instance represents
    -- Uses full timestamp to support intra-day recurrence patterns
    occurs_at TIMESTAMPTZ,

    -- due_offset: Duration from starts_at to calculate due_at
    -- If set: due_at = starts_at + due_offset
    -- If nil: due_at is set directly
    due_offset INTERVAL,

    -- Timezone controls interpretation of task times (starts_at, occurs_at, due_at)
    -- Does NOT affect operational times (created_at, updated_at) which are always UTC
    --
    -- NULL (floating time):
    --   Time value stays constant across timezones (9am is always 9am)
    --   Use for location-independent tasks ("wake up at 9am")
    --
    -- Non-NULL (fixed timezone, IANA format like 'Europe/Stockholm'):
    --   Time anchored to specific timezone, represents absolute UTC moment
    --   9am Stockholm (UTC+1) = 08:00 UTC = 10am Helsinki (UTC+2) = 4am New York (UTC-5)
    --   Use for location-specific tasks ("Stockholm office meeting at 9am")
    timezone TEXT,

    version INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY (list_id) REFERENCES todo_lists(id) ON DELETE CASCADE,

    -- Recurring instance integrity: occurs_at requires recurring_template_id
    -- Prevents orphan recurring instances (occurs_at set without template)
    CHECK (occurs_at IS NULL OR recurring_template_id IS NOT NULL)
);

-- Indexes for common query patterns
CREATE INDEX idx_todo_items_list_id ON todo_items(list_id);
CREATE INDEX idx_todo_items_list_created ON todo_items(list_id, created_at);

CREATE INDEX idx_todo_items_status ON todo_items(status)
    WHERE status NOT IN ('archived', 'cancelled');

CREATE INDEX idx_todo_items_priority ON todo_items(priority)
    WHERE priority IS NOT NULL;

CREATE INDEX idx_todo_items_due_at ON todo_items(due_at)
    WHERE due_at IS NOT NULL;

CREATE INDEX idx_todo_items_updated_at ON todo_items(updated_at DESC);

CREATE INDEX idx_todo_items_tags_gin ON todo_items USING gin(tags)
    WHERE tags IS NOT NULL;

CREATE INDEX idx_todo_items_active_due ON todo_items(status, due_at)
    WHERE status IN ('todo', 'in_progress', 'blocked');

CREATE INDEX idx_todo_items_recurring_template ON todo_items(recurring_template_id)
    WHERE recurring_template_id IS NOT NULL;

CREATE INDEX idx_todo_items_recurring_cleanup ON todo_items(recurring_template_id, occurs_at, status)
    WHERE recurring_template_id IS NOT NULL;

CREATE UNIQUE INDEX idx_items_unique_recurring_instance
    ON todo_items(recurring_template_id, occurs_at)
    WHERE recurring_template_id IS NOT NULL;

CREATE INDEX idx_todo_items_active_starts ON todo_items(status, starts_at)
    WHERE status IN ('todo', 'in_progress', 'blocked') AND starts_at IS NOT NULL;

-- ============================================================================
-- Time Tracking
-- ============================================================================

-- Track all status transitions for time calculation and audit trail
CREATE TABLE task_status_history (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    task_id uuid NOT NULL,
    from_status TEXT
        CHECK (from_status IS NULL OR from_status IN ('todo', 'in_progress', 'blocked', 'done', 'archived', 'cancelled')),
    to_status TEXT NOT NULL
        CHECK (to_status IN ('todo', 'in_progress', 'blocked', 'done', 'archived', 'cancelled')),
    changed_at timestamptz NOT NULL DEFAULT now(),
    notes TEXT,

    FOREIGN KEY (task_id) REFERENCES todo_items(id) ON DELETE CASCADE
);

CREATE INDEX idx_status_history_task_time ON task_status_history(task_id, changed_at DESC);

-- ============================================================================
-- Recurring Tasks
-- ============================================================================

-- Templates for recurring tasks
CREATE TABLE recurring_task_templates (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    list_id uuid NOT NULL,

    -- Template fields (same structure as todo_items)
    title TEXT NOT NULL,
    tags TEXT[],
    priority TEXT
        CHECK (priority IS NULL OR priority IN ('low', 'medium', 'high', 'urgent')),
    estimated_duration INTERVAL,

    -- Recurrence pattern (using TEXT with CHECK constraint)
    -- 'interval' pattern uses recurrence_config.interval_hours for intra-day tasks (e.g., every 8 hours)
    recurrence_pattern TEXT NOT NULL
        CHECK (recurrence_pattern IN ('daily', 'weekly', 'biweekly', 'monthly', 'yearly', 'quarterly', 'weekdays', 'interval')),

    -- Pattern-specific configuration (JSONB for flexibility)
    recurrence_config jsonb NOT NULL,
    /* Examples:
       DAILY: {"time": "09:00"}
       WEEKLY: {"day_of_week": 1, "time": "09:00"}  -- Monday at 9am
       BIWEEKLY: {"days_of_week": [1,3,5]}  -- Mon, Wed, Fri every 2 weeks
       MONTHLY: {"interval": 1, "day_of_month": 15} or {"day_of_month": "last"}
       YEARLY: {"month": 3, "day": 15}  -- March 15
       QUARTERLY: {"month_offset": 0, "day": 1}  -- First day of quarter
       WEEKDAYS: {}  -- Mon-Fri
       INTERVAL: {"interval_hours": 8, "start_time": "06:00"}  -- Every 8h starting 6am
    */

    -- Relative due date offset from instance date
    due_offset INTERVAL,

    -- Template state
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- Generation tracking
    -- generated_through: Last date we generated tasks through
    generated_through DATE NOT NULL DEFAULT CURRENT_DATE,
    -- Generation configuration (horizon = how far ahead to generate)
    -- Values must be provided by application layer configuration
    sync_horizon_days INTEGER NOT NULL,
    generation_horizon_days INTEGER NOT NULL,

    -- Optimistic locking version - incremented on each update to detect concurrent modifications
    version INTEGER NOT NULL DEFAULT 1,

    FOREIGN KEY (list_id) REFERENCES todo_lists(id) ON DELETE CASCADE
);

-- Add FK from todo_items to recurring_task_templates
ALTER TABLE todo_items
    ADD CONSTRAINT fk_todo_items_recurring_template
    FOREIGN KEY (recurring_template_id) REFERENCES recurring_task_templates(id) ON DELETE SET NULL;

-- Indexes
CREATE INDEX idx_recurring_templates_active ON recurring_task_templates(is_active, generated_through)
    WHERE is_active = true;

CREATE INDEX idx_recurring_templates_list ON recurring_task_templates(list_id)
    WHERE is_active = true;

-- ============================================================================
-- Job Queue for Recurring Task Generation
-- ============================================================================

CREATE TABLE recurring_generation_jobs (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    template_id uuid NOT NULL,

    -- Job status
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'scheduled', 'running', 'completed', 'failed', 'discarded', 'cancelling', 'cancelled')),

    -- Availability timeout for stuck job recovery (Kubernetes-inspired coordination)
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    claimed_by TEXT,
    claimed_at TIMESTAMPTZ,

    -- Generation range
    generate_from TIMESTAMPTZ NOT NULL,
    generate_until TIMESTAMPTZ NOT NULL,
    scheduled_for TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,

    -- Retry tracking
    error_message TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,

    FOREIGN KEY (template_id) REFERENCES recurring_task_templates(id) ON DELETE CASCADE
);

-- Index for job claiming with stuck job recovery
-- Covers: pending jobs ready to run, OR stuck running jobs past availability timeout
CREATE INDEX idx_generation_jobs_claimable ON recurring_generation_jobs(scheduled_for, status, available_at)
    WHERE status IN ('pending', 'running');

-- Index for finding jobs by template
CREATE INDEX idx_generation_jobs_template ON recurring_generation_jobs(template_id, status)
    WHERE status IN ('pending', 'running');

-- Index for cleanup of old completed jobs
CREATE INDEX idx_generation_jobs_completed ON recurring_generation_jobs(completed_at)
    WHERE status = 'completed';

-- ============================================================================
-- Cron Job Leases (Lease-Based Exclusive Execution)
-- ============================================================================

-- Provides crash and deadlock recovery for single-instance workers
CREATE TABLE cron_job_leases (
    run_type TEXT PRIMARY KEY,
    holder_id TEXT NOT NULL,
    acquired_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    renewed_at TIMESTAMPTZ NOT NULL,
    run_count BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_cron_leases_expired ON cron_job_leases(expires_at);

-- ============================================================================
-- Recurring Template Exceptions
-- ============================================================================

-- Tracks deleted or rescheduled instances of recurring templates
-- Prevents regeneration of user-deleted instances
CREATE TABLE recurring_template_exceptions (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    template_id UUID NOT NULL REFERENCES recurring_task_templates(id) ON DELETE CASCADE,

    -- Exact timestamp this exception applies to
    occurs_at TIMESTAMPTZ NOT NULL,

    -- Why this exception exists ('deleted', 'rescheduled', or 'edited')
    exception_type TEXT NOT NULL CHECK (exception_type IN ('deleted', 'rescheduled', 'edited')),

    -- For rescheduled: reference to the detached item
    item_id UUID REFERENCES todo_items(id) ON DELETE SET NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (template_id, occurs_at)
);

CREATE INDEX idx_template_exceptions_lookup ON recurring_template_exceptions(template_id, occurs_at);

-- ============================================================================
-- Dead Letter Queue
-- ============================================================================

-- Jobs that fail permanently or exhaust retries
-- Preserves full context for debugging and manual intervention
CREATE TABLE dead_letter_jobs (
    id UUID PRIMARY KEY DEFAULT uuidv7(),

    -- Original job details (UUID to match recurring_generation_jobs.id)
    -- Nullable: job may be cleaned up before DLQ review, but we keep the DLQ entry for audit
    original_job_id UUID,
    template_id UUID NOT NULL,
    generate_from TIMESTAMPTZ NOT NULL,
    generate_until TIMESTAMPTZ NOT NULL,

    -- Failure classification
    error_type TEXT NOT NULL CHECK (error_type IN ('permanent', 'exhausted', 'panic')),
    error_message TEXT,
    stack_trace TEXT,

    -- Timeline
    failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at TIMESTAMPTZ,

    -- Admin resolution
    resolution TEXT CHECK (resolution IN ('retried', 'discarded')),
    reviewed_by TEXT,
    reviewer_note TEXT,

    -- Full job context for reproduction
    retry_count INTEGER NOT NULL,
    last_worker_id TEXT,
    original_scheduled_for TIMESTAMPTZ NOT NULL,
    original_created_at TIMESTAMPTZ NOT NULL,

    -- FK to original job - SET NULL on delete to preserve DLQ for audit
    FOREIGN KEY (original_job_id) REFERENCES recurring_generation_jobs(id) ON DELETE SET NULL
);

-- Admin review (pending items first, then by failure time)
CREATE INDEX idx_dead_letter_pending ON dead_letter_jobs(resolution, failed_at DESC)
    WHERE resolution IS NULL;

-- Cleanup old resolved items
CREATE INDEX idx_dead_letter_cleanup ON dead_letter_jobs(reviewed_at)
    WHERE resolution IS NOT NULL;

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

CREATE INDEX idx_api_keys_short_token ON api_keys(short_token)
    WHERE is_active = true;

CREATE INDEX idx_api_keys_active_created ON api_keys(created_at DESC)
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

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Drop in reverse order to handle dependencies
DROP TRIGGER IF EXISTS track_initial_status_on_insert ON todo_items;
DROP TRIGGER IF EXISTS track_status_changes ON todo_items;
DROP TRIGGER IF EXISTS update_recurring_templates_updated_at ON recurring_task_templates;
DROP TRIGGER IF EXISTS update_todo_items_updated_at ON todo_items;

DROP FUNCTION IF EXISTS track_initial_status();
DROP FUNCTION IF EXISTS update_actual_duration_on_status_change();
DROP FUNCTION IF EXISTS calculate_actual_duration(uuid);
DROP FUNCTION IF EXISTS update_updated_at_column();

DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS recurring_template_exceptions;
DROP TABLE IF EXISTS cron_job_leases;
DROP TABLE IF EXISTS recurring_generation_jobs;
DROP TABLE IF EXISTS recurring_task_templates CASCADE;
DROP TABLE IF EXISTS task_status_history;
DROP TABLE IF EXISTS todo_items;
DROP TABLE IF EXISTS todo_lists;

-- +goose StatementEnd
