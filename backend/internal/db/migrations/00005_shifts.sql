-- +goose Up
CREATE TABLE shifts (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id       UUID NOT NULL REFERENCES employees (id) ON DELETE CASCADE,
    schedule_id       UUID REFERENCES schedules (id) ON DELETE SET NULL,
    check_in_mark_id  UUID,
    check_out_mark_id UUID,
    started_at        TIMESTAMPTZ NOT NULL,
    ended_at          TIMESTAMPTZ,
    status            TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed', 'auto_closed')),
    overtime          BOOLEAN NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_shifts_employee_started ON shifts (employee_id, started_at DESC);
CREATE INDEX idx_shifts_status ON shifts (status);

-- +goose Down
DROP TABLE IF EXISTS shifts;
