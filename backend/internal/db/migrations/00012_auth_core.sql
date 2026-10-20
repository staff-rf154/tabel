-- +goose Up
CREATE INDEX IF NOT EXISTS idx_marks_employee_event_ts
    ON marks (employee_id, event_type, timestamp_server DESC);

CREATE INDEX IF NOT EXISTS idx_sessions_token_hash_active
    ON sessions (token_hash) WHERE revoked_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_main_admin
    ON users (is_main_admin) WHERE is_main_admin;

-- +goose Down
DROP INDEX IF EXISTS uq_users_main_admin;
DROP INDEX IF EXISTS idx_sessions_token_hash_active;
DROP INDEX IF EXISTS idx_marks_employee_event_ts;
