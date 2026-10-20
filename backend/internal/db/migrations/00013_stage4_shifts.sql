-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS uq_shifts_open_per_employee
    ON shifts (employee_id)
    WHERE status = 'open';

CREATE INDEX IF NOT EXISTS idx_schedules_employee_valid
    ON schedules (employee_id, valid_from DESC, COALESCE(valid_to, 'infinity'::date));

-- +goose Down
DROP INDEX IF EXISTS idx_schedules_employee_valid;
DROP INDEX IF EXISTS uq_shifts_open_per_employee;
