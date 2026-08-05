-- PostgreSQL-compatible schema for the Agent task service.

CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    goal TEXT NOT NULL,
    status TEXT NOT NULL,
    result_json JSONB,
    error_code TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS tasks_user_status_idx ON tasks (user_id, status);
CREATE INDEX IF NOT EXISTS tasks_updated_at_idx ON tasks (updated_at);

CREATE TABLE IF NOT EXISTS agent_runs (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(id),
    step INTEGER NOT NULL,
    kind TEXT NOT NULL,
    name TEXT,
    input_json JSONB,
    output_json JSONB,
    status TEXT NOT NULL,
    duration_ms BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS agent_runs_task_idx ON agent_runs (task_id, step);
