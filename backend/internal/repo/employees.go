package repo

import (
	"context"
)

const employeeColumns = `
	id::text, employee_number, full_name, last_name, first_name, middle_name,
	email, phone, position,
	department_id::text, position_id::text, is_active,
	to_char(hired_at, 'YYYY-MM-DD'), to_char(fired_at, 'YYYY-MM-DD')`

func scanEmployee(row interface{ Scan(...any) error }, e *Employee) error {
	return row.Scan(
		&e.ID, &e.EmployeeNumber, &e.FullName, &e.LastName, &e.FirstName, &e.MiddleName,
		&e.Email, &e.Phone, &e.Position,
		&e.DepartmentID, &e.PositionID, &e.IsActive, &e.HiredAt, &e.FiredAt,
	)
}

func (r *Repo) ListEmployees(ctx context.Context, query string, isActive *bool, limit, offset int) ([]Employee, error) {
	limit = normalizedLimit(limit, 50, 500)
	offset = normalizedOffset(offset)
	rows, err := r.Pool.Query(ctx, `
SELECT `+employeeColumns+`
FROM employees
WHERE ($1 = '' OR full_name ILIKE '%' || $1 || '%'
       OR employee_number ILIKE '%' || $1 || '%'
       OR COALESCE(phone, '') ILIKE '%' || $1 || '%'
       OR COALESCE(email, '') ILIKE '%' || $1 || '%')
  AND ($2::boolean IS NULL OR is_active = $2)
ORDER BY full_name
LIMIT $3 OFFSET $4`, query, isActive, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var employees []Employee
	for rows.Next() {
		var e Employee
		if err := scanEmployee(rows, &e); err != nil {
			return nil, err
		}
		employees = append(employees, e)
	}
	return employees, rows.Err()
}

func (r *Repo) GetEmployee(ctx context.Context, id string) (Employee, error) {
	var e Employee
	err := scanEmployee(r.Pool.QueryRow(ctx, `SELECT `+employeeColumns+` FROM employees WHERE id = $1::uuid`, id), &e)
	if isNoRows(err) {
		return e, ErrNotFound
	}
	return e, err
}

func (r *Repo) CreateEmployee(ctx context.Context, in EmployeeInput) (Employee, error) {
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	var e Employee
	err := scanEmployee(r.Pool.QueryRow(ctx, `
INSERT INTO employees (employee_number, full_name, last_name, first_name, middle_name, email, phone, position, department_id, position_id, is_active, hired_at, fired_at)
VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), $6, $7,
        (SELECT name FROM positions WHERE id = $9::uuid), $8::uuid, $9::uuid, $10, $11::date, $12::date)
RETURNING `+employeeColumns,
		in.EmployeeNumber, in.FullName, in.LastName, in.FirstName, in.MiddleName,
		in.Email, in.Phone, in.DepartmentID, in.PositionID, active, in.HiredAt, in.FiredAt), &e)
	if isUniqueViolation(err) {
		return e, ErrEmployeeNumberTaken
	}
	if isForeignKeyViolation(err) {
		return e, ErrReferenceNotFound
	}
	return e, err
}

func (r *Repo) UpdateEmployee(ctx context.Context, id string, in EmployeeInput) (Employee, error) {
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	var e Employee
	err := scanEmployee(r.Pool.QueryRow(ctx, `
UPDATE employees
SET employee_number = $2, full_name = $3, last_name = NULLIF($4, ''), first_name = NULLIF($5, ''),
    middle_name = NULLIF($6, ''), email = $7, phone = $8,
    position = (SELECT name FROM positions WHERE id = $10::uuid),
    department_id = $9::uuid, position_id = $10::uuid,
    is_active = $11, hired_at = $12::date, fired_at = $13::date, updated_at = now()
WHERE id = $1::uuid
RETURNING `+employeeColumns,
		id, in.EmployeeNumber, in.FullName, in.LastName, in.FirstName, in.MiddleName,
		in.Email, in.Phone, in.DepartmentID, in.PositionID, active, in.HiredAt, in.FiredAt), &e)
	if isNoRows(err) {
		return e, ErrNotFound
	}
	if isUniqueViolation(err) {
		return e, ErrEmployeeNumberTaken
	}
	if isForeignKeyViolation(err) {
		return e, ErrReferenceNotFound
	}
	return e, err
}
