package repo

import (
	"context"
)

const departmentColumns = `id::text, name, is_active, to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')`

func scanDepartment(row interface{ Scan(...any) error }, d *Department) error {
	return row.Scan(&d.ID, &d.Name, &d.IsActive, &d.CreatedAt)
}

const positionColumns = `id::text, name, is_active, to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')`

func scanPosition(row interface{ Scan(...any) error }, p *Position) error {
	return row.Scan(&p.ID, &p.Name, &p.IsActive, &p.CreatedAt)
}

func (r *Repo) ListDepartments(ctx context.Context, activeOnly bool) ([]Department, error) {
	rows, err := r.Pool.Query(ctx, `
SELECT `+departmentColumns+`
FROM departments
WHERE (NOT $1::boolean OR is_active)
ORDER BY name`, activeOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Department
	for rows.Next() {
		var item Department
		if err := scanDepartment(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repo) CreateDepartment(ctx context.Context, in DepartmentInput) (Department, error) {
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	var item Department
	err := scanDepartment(r.Pool.QueryRow(ctx, `
INSERT INTO departments (name, is_active)
VALUES ($1, $2)
RETURNING `+departmentColumns, in.Name, active), &item)
	if isUniqueViolation(err) {
		return item, ErrNameTaken
	}
	return item, err
}

func (r *Repo) FindDepartmentByName(ctx context.Context, name string) (Department, error) {
	var item Department
	err := scanDepartment(r.Pool.QueryRow(ctx, `SELECT `+departmentColumns+` FROM departments WHERE lower(name) = lower($1)`, name), &item)
	if isNoRows(err) {
		return item, ErrNotFound
	}
	return item, err
}

func (r *Repo) ListPositions(ctx context.Context, activeOnly bool) ([]Position, error) {
	rows, err := r.Pool.Query(ctx, `
SELECT `+positionColumns+`
FROM positions
WHERE (NOT $1::boolean OR is_active)
ORDER BY name`, activeOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Position
	for rows.Next() {
		var item Position
		if err := scanPosition(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repo) CreatePosition(ctx context.Context, in PositionInput) (Position, error) {
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	var item Position
	err := scanPosition(r.Pool.QueryRow(ctx, `
INSERT INTO positions (name, is_active)
VALUES ($1, $2)
RETURNING `+positionColumns, in.Name, active), &item)
	if isUniqueViolation(err) {
		return item, ErrNameTaken
	}
	return item, err
}

func (r *Repo) FindPositionByName(ctx context.Context, name string) (Position, error) {
	var item Position
	err := scanPosition(r.Pool.QueryRow(ctx, `SELECT `+positionColumns+` FROM positions WHERE lower(name) = lower($1)`, name), &item)
	if isNoRows(err) {
		return item, ErrNotFound
	}
	return item, err
}
