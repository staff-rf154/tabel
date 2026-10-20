-- +goose Up
ALTER TABLE employees ADD COLUMN last_name TEXT;
ALTER TABLE employees ADD COLUMN first_name TEXT;
ALTER TABLE employees ADD COLUMN middle_name TEXT;

WITH split AS (
    SELECT id, regexp_split_to_array(btrim(full_name), '\s+') AS parts
    FROM employees
    WHERE btrim(full_name) <> ''
)
UPDATE employees e
SET last_name = split.parts[1],
    first_name = NULLIF(split.parts[2], ''),
    middle_name = NULLIF(array_to_string(split.parts[3:], ' '), '')
FROM split
WHERE split.id = e.id;

CREATE INDEX idx_employees_last_name ON employees (lower(last_name));
CREATE INDEX idx_employees_first_name ON employees (lower(first_name));

DELETE FROM notifications n
WHERE n.type LIKE 'delegation.%'
  AND n.payload ? 'delegation_id'
  AND NOT EXISTS (
      SELECT 1 FROM delegations d
      WHERE d.id::text = n.payload->>'delegation_id'
  );

-- +goose Down
DROP INDEX IF EXISTS idx_employees_first_name;
DROP INDEX IF EXISTS idx_employees_last_name;
ALTER TABLE employees DROP COLUMN IF EXISTS middle_name;
ALTER TABLE employees DROP COLUMN IF EXISTS first_name;
ALTER TABLE employees DROP COLUMN IF EXISTS last_name;
