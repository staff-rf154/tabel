-- +goose Up
CREATE TABLE marks (
    id                   UUID NOT NULL DEFAULT gen_random_uuid(),
    employee_id          UUID NOT NULL REFERENCES employees (id) ON DELETE RESTRICT,
    operator_id          UUID REFERENCES users (id) ON DELETE SET NULL,
    operator_number      TEXT,
    event_type           TEXT NOT NULL CHECK (event_type IN ('check_in', 'check_out')),
    timestamp_client     TIMESTAMPTZ,
    timestamp_server     TIMESTAMPTZ NOT NULL DEFAULT now(),
    lat                  DOUBLE PRECISION,
    lon                  DOUBLE PRECISION,
    accuracy_m           DOUBLE PRECISION,
    location_age_sec     INTEGER,
    device_os            TEXT,
    device_os_version    TEXT,
    device_model         TEXT,
    app_version          TEXT,
    mock_location        BOOLEAN NOT NULL DEFAULT false,
    outside_office       BOOLEAN NOT NULL DEFAULT false,
    overtime             BOOLEAN NOT NULL DEFAULT false,
    auto_closed_previous BOOLEAN NOT NULL DEFAULT false,
    qr_source            TEXT CHECK (qr_source IN ('static', 'paper')),
    qr_data_mismatch     BOOLEAN NOT NULL DEFAULT false,
    client_type          TEXT CHECK (client_type IN ('mobile', 'web')),
    shift_id             UUID REFERENCES shifts (id) ON DELETE SET NULL,
    idempotency_key      UUID NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, timestamp_server)
) PARTITION BY RANGE (timestamp_server);

CREATE INDEX idx_marks_employee_ts ON marks (employee_id, timestamp_server DESC);
CREATE INDEX idx_marks_operator_ts ON marks (operator_id, timestamp_server);
CREATE INDEX idx_marks_shift_id ON marks (shift_id);
CREATE INDEX idx_marks_idempotency ON marks (idempotency_key);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION create_marks_partition(target_date date)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    part_name text;
    range_start timestamptz;
    range_end timestamptz;
BEGIN
    part_name := 'marks_' || to_char(target_date, 'YYYY_MM');
    range_start := date_trunc('month', target_date)::timestamptz;
    range_end := (range_start + interval '1 month');

    IF NOT EXISTS (SELECT 1 FROM pg_class WHERE relname = part_name) THEN
        EXECUTE format(
            'CREATE TABLE %I PARTITION OF marks FOR VALUES FROM (%L) TO (%L)',
            part_name, range_start, range_end
        );
    END IF;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
DECLARE
    i integer;
BEGIN
    FOR i IN 0..6 LOOP
        PERFORM create_marks_partition((now() + (i || ' month')::interval)::date);
    END LOOP;
END;
$$;
-- +goose StatementEnd

CREATE TABLE mark_idempotency (
    idempotency_key UUID PRIMARY KEY,
    mark_id         UUID,
    employee_id     UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_mark_idempotency_created_at ON mark_idempotency (created_at);

-- +goose Down
DROP TABLE IF EXISTS mark_idempotency;
DROP FUNCTION IF EXISTS create_marks_partition(date);
DROP TABLE IF EXISTS marks;
