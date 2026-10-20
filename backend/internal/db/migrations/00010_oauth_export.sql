-- +goose Up
CREATE TABLE oauth_clients (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id          TEXT NOT NULL UNIQUE,
    client_secret_hash TEXT NOT NULL,
    name               TEXT,
    is_active          BOOLEAN NOT NULL DEFAULT true,
    secret_rotated_at  TIMESTAMPTZ,
    next_rotation_at   TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE oauth_access_tokens (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id  TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_oauth_access_tokens_client ON oauth_access_tokens (client_id);
CREATE INDEX idx_oauth_access_tokens_expires_at ON oauth_access_tokens (expires_at);

CREATE TABLE ip_whitelist (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    oauth_client_id UUID REFERENCES oauth_clients (id) ON DELETE CASCADE,
    cidr            CIDR NOT NULL,
    description     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ip_whitelist_client ON ip_whitelist (oauth_client_id);

CREATE TABLE export_acks (
    mark_id   UUID PRIMARY KEY,
    client_id TEXT,
    acked_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_export_acks_acked_at ON export_acks (acked_at);

CREATE TABLE offline_imports (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    uploaded_by UUID REFERENCES users (id) ON DELETE SET NULL,
    filename    TEXT NOT NULL,
    rows_total  INTEGER NOT NULL DEFAULT 0,
    rows_ok     INTEGER NOT NULL DEFAULT 0,
    rows_error  INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL,
    report      JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS offline_imports;
DROP TABLE IF EXISTS export_acks;
DROP TABLE IF EXISTS ip_whitelist;
DROP TABLE IF EXISTS oauth_access_tokens;
DROP TABLE IF EXISTS oauth_clients;
