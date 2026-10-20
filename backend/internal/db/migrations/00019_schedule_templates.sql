-- +goose Up
CREATE TABLE schedule_templates (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    pattern_type TEXT NOT NULL CHECK (pattern_type IN ('5/2', '2/2', 'custom')),
    workdays     SMALLINT[] NOT NULL,
    start_time   TIME NOT NULL,
    end_time     TIME NOT NULL,
    created_by   UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_schedule_templates_name ON schedule_templates (lower(name));

-- +goose Down
DROP TABLE IF EXISTS schedule_templates;
