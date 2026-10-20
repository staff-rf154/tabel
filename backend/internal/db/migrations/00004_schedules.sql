-- +goose Up
CREATE TABLE schedules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id  UUID NOT NULL REFERENCES employees (id) ON DELETE CASCADE,
    pattern_type TEXT NOT NULL CHECK (pattern_type IN ('5/2', '2/2', 'custom')),
    workdays     SMALLINT[] NOT NULL,
    start_time   TIME NOT NULL,
    end_time     TIME NOT NULL,
    valid_from   DATE NOT NULL,
    valid_to     DATE,
    is_active    BOOLEAN NOT NULL DEFAULT true,
    source_file  TEXT,
    created_by   UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_schedules_active_per_employee
    ON schedules (employee_id)
    WHERE is_active;

CREATE INDEX idx_schedules_employee_id ON schedules (employee_id);

-- +goose Down
DROP TABLE IF EXISTS schedules;
