-- +goose Up
CREATE TABLE delegations (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    to_user_id   UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status       TEXT NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'approved', 'rejected', 'revoked', 'expired')),
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_until  TIMESTAMPTZ NOT NULL,
    decided_by   UUID REFERENCES users (id) ON DELETE SET NULL,
    decided_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_delegation_distinct_users CHECK (from_user_id <> to_user_id)
);

CREATE INDEX idx_delegations_from_user ON delegations (from_user_id);
CREATE INDEX idx_delegations_to_user ON delegations (to_user_id);
CREATE INDEX idx_delegations_status ON delegations (status);

-- +goose Down
DROP TABLE IF EXISTS delegations;
