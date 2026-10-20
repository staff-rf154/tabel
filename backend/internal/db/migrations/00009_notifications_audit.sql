-- +goose Up
CREATE TABLE notifications (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type           TEXT NOT NULL,
    title          TEXT NOT NULL,
    body           TEXT,
    payload        JSONB,
    target_user_id UUID REFERENCES users (id) ON DELETE CASCADE,
    is_read        BOOLEAN NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notifications_target_user ON notifications (target_user_id, is_read);
CREATE INDEX idx_notifications_created_at ON notifications (created_at DESC);

CREATE TABLE audit_log (
    id            BIGSERIAL PRIMARY KEY,
    actor_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    action        TEXT NOT NULL,
    entity_type   TEXT,
    entity_id     TEXT,
    details       JSONB,
    ip            INET,
    user_agent    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_log_action ON audit_log (action);
CREATE INDEX idx_audit_log_entity ON audit_log (entity_type, entity_id);
CREATE INDEX idx_audit_log_created_at ON audit_log (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS notifications;
