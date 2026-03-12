-- +goose Up

CREATE TABLE IF NOT EXISTS usage_events (
    sequence BIGSERIAL PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE,
    producer TEXT NOT NULL,
    region_id TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL,
    subject_type TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    team_id TEXT NOT NULL DEFAULT '',
    user_id TEXT NOT NULL DEFAULT '',
    sandbox_id TEXT,
    volume_id TEXT,
    snapshot_id TEXT,
    template_id TEXT,
    cluster_id TEXT,
    occurred_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    data JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_usage_events_sequence ON usage_events(sequence);
CREATE INDEX IF NOT EXISTS idx_usage_events_occurred_at ON usage_events(occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_team_id ON usage_events(team_id);
CREATE INDEX IF NOT EXISTS idx_usage_events_event_type ON usage_events(event_type);
CREATE INDEX IF NOT EXISTS idx_usage_events_subject ON usage_events(subject_type, subject_id);

CREATE TABLE IF NOT EXISTS usage_windows (
    sequence BIGSERIAL PRIMARY KEY,
    window_id TEXT NOT NULL UNIQUE,
    producer TEXT NOT NULL,
    region_id TEXT NOT NULL DEFAULT '',
    window_type TEXT NOT NULL,
    subject_type TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    team_id TEXT NOT NULL DEFAULT '',
    user_id TEXT NOT NULL DEFAULT '',
    sandbox_id TEXT,
    volume_id TEXT,
    snapshot_id TEXT,
    template_id TEXT,
    cluster_id TEXT,
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    value BIGINT NOT NULL,
    unit TEXT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    data JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_usage_windows_sequence ON usage_windows(sequence);
CREATE INDEX IF NOT EXISTS idx_usage_windows_range ON usage_windows(window_start, window_end);
CREATE INDEX IF NOT EXISTS idx_usage_windows_type ON usage_windows(window_type);
CREATE INDEX IF NOT EXISTS idx_usage_windows_subject ON usage_windows(subject_type, subject_id);

CREATE TABLE IF NOT EXISTS producer_watermarks (
    producer TEXT PRIMARY KEY,
    region_id TEXT NOT NULL DEFAULT '',
    complete_before TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS manager_sandbox_projection_state (
    sandbox_id TEXT PRIMARY KEY,
    namespace TEXT NOT NULL,
    team_id TEXT NOT NULL DEFAULT '',
    user_id TEXT NOT NULL DEFAULT '',
    template_id TEXT NOT NULL DEFAULT '',
    cluster_id TEXT NOT NULL DEFAULT '',
    claimed_at TIMESTAMPTZ,
    active_since TIMESTAMPTZ,
    paused BOOLEAN NOT NULL DEFAULT FALSE,
    paused_at TIMESTAMPTZ,
    terminated_at TIMESTAMPTZ,
    last_observed_at TIMESTAMPTZ NOT NULL,
    last_resource_version TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_manager_sandbox_projection_state_observed_at
    ON manager_sandbox_projection_state(last_observed_at);

-- +goose Down

DROP INDEX IF EXISTS idx_manager_sandbox_projection_state_observed_at;
DROP TABLE IF EXISTS manager_sandbox_projection_state;
DROP TABLE IF EXISTS producer_watermarks;
DROP INDEX IF EXISTS idx_usage_windows_subject;
DROP INDEX IF EXISTS idx_usage_windows_type;
DROP INDEX IF EXISTS idx_usage_windows_range;
DROP INDEX IF EXISTS idx_usage_windows_sequence;
DROP TABLE IF EXISTS usage_windows;
DROP INDEX IF EXISTS idx_usage_events_subject;
DROP INDEX IF EXISTS idx_usage_events_event_type;
DROP INDEX IF EXISTS idx_usage_events_team_id;
DROP INDEX IF EXISTS idx_usage_events_occurred_at;
DROP INDEX IF EXISTS idx_usage_events_sequence;
DROP TABLE IF EXISTS usage_events;
