-- +goose Up
CREATE TABLE qr_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id  UUID NOT NULL REFERENCES employees (id) ON DELETE CASCADE,
    type         TEXT NOT NULL CHECK (type IN ('static', 'paper')),
    token        UUID NOT NULL UNIQUE,
    payload      JSONB NOT NULL,
    signature    TEXT NOT NULL,
    valid_from   DATE,
    valid_until  DATE,
    status       TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked', 'expired')),
    issued_by    UUID REFERENCES users (id) ON DELETE SET NULL,
    issued_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at   TIMESTAMPTZ,
    revoked_by   UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_qr_tokens_employee_id ON qr_tokens (employee_id);
CREATE INDEX idx_qr_tokens_status ON qr_tokens (status);

CREATE UNIQUE INDEX uq_qr_tokens_active_paper_per_employee
    ON qr_tokens (employee_id)
    WHERE type = 'paper' AND status = 'active';

CREATE UNIQUE INDEX uq_qr_tokens_active_static_per_employee
    ON qr_tokens (employee_id)
    WHERE type = 'static' AND status = 'active';

-- +goose Down
DROP TABLE IF EXISTS qr_tokens;
