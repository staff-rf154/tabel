-- +goose Up
CREATE TABLE users (
    id                           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id                  UUID UNIQUE REFERENCES employees (id) ON DELETE SET NULL,
    role                         TEXT NOT NULL CHECK (role IN ('employee', 'operator', 'admin')),
    login                        TEXT NOT NULL UNIQUE,
    password_hash                TEXT NOT NULL,
    password_algo                TEXT NOT NULL DEFAULT 'argon2id',
    failed_attempts              INTEGER NOT NULL DEFAULT 0,
    is_locked                    BOOLEAN NOT NULL DEFAULT false,
    auth_blocked                 BOOLEAN NOT NULL DEFAULT false,
    is_main_admin                BOOLEAN NOT NULL DEFAULT false,
    registration_reset_allowed   BOOLEAN NOT NULL DEFAULT false,
    registered_at                TIMESTAMPTZ,
    last_login_at                TIMESTAMPTZ,
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_role ON users (role);
CREATE INDEX idx_users_employee_id ON users (employee_id);

CREATE TABLE sessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    user_agent  TEXT,
    ip          INET,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sessions_user_id ON sessions (user_id);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);

-- +goose Down
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
