-- +goose Up
CREATE TABLE employees (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_number TEXT NOT NULL UNIQUE,
    full_name       TEXT NOT NULL,
    email           TEXT,
    phone           TEXT,
    position        TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    hired_at        DATE,
    fired_at        DATE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_employees_full_name ON employees (full_name);
CREATE INDEX idx_employees_phone ON employees (phone);
CREATE INDEX idx_employees_email ON employees (email);

-- +goose Down
DROP TABLE IF EXISTS employees;
