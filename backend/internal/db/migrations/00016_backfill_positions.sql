-- +goose Up
INSERT INTO positions (name)
SELECT DISTINCT btrim(position)
FROM employees
WHERE position IS NOT NULL AND btrim(position) <> ''
ON CONFLICT (name) DO NOTHING;

UPDATE employees e
SET position_id = p.id
FROM positions p
WHERE e.position_id IS NULL
  AND e.position IS NOT NULL
  AND lower(btrim(e.position)) = lower(p.name);

UPDATE employees e
SET position = p.name
FROM positions p
WHERE e.position_id = p.id
  AND e.position IS DISTINCT FROM p.name;

-- +goose Down
