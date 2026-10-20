-- +goose Up
ALTER TABLE qr_tokens ADD COLUMN revoked_reason TEXT;

CREATE TABLE departments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE,
    is_active  BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE positions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE,
    is_active  BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE employees ADD COLUMN department_id UUID REFERENCES departments (id) ON DELETE SET NULL;
ALTER TABLE employees ADD COLUMN position_id UUID REFERENCES positions (id) ON DELETE SET NULL;

CREATE INDEX idx_employees_department_id ON employees (department_id);
CREATE INDEX idx_employees_position_id ON employees (position_id);

-- +goose Down
DROP INDEX IF EXISTS idx_employees_position_id;
DROP INDEX IF EXISTS idx_employees_department_id;
ALTER TABLE employees DROP COLUMN IF EXISTS position_id;
ALTER TABLE employees DROP COLUMN IF EXISTS department_id;
DROP TABLE IF EXISTS positions;
DROP TABLE IF EXISTS departments;
ALTER TABLE qr_tokens DROP COLUMN IF EXISTS revoked_reason;
