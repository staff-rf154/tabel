-- +goose Up
ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('employee', 'operator', 'admin', 'shift_lead', 'director', 'hr'));

CREATE TABLE mobile_builds (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    platform    TEXT NOT NULL CHECK (platform IN ('android', 'ios')),
    version     TEXT NOT NULL,
    filename    TEXT NOT NULL,
    stored_path TEXT NOT NULL,
    size_bytes  BIGINT NOT NULL DEFAULT 0,
    uploaded_by UUID REFERENCES users (id) ON DELETE SET NULL,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_mobile_builds_platform ON mobile_builds (platform, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS mobile_builds;
ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('employee', 'operator', 'admin'));
