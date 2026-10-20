package repo

import (
	"context"
)

const shiftColumns = `
	id::text, employee_id::text, schedule_id::text, started_at, ended_at, status, overtime`

func scanShift(row interface{ Scan(...any) error }, s *Shift) error {
	return row.Scan(&s.ID, &s.EmployeeID, &s.ScheduleID, &s.StartedAt, &s.EndedAt, &s.Status, &s.Overtime)
}

// GetOpenShift возвращает текущую открытую смену сотрудника или ErrNotFound.
func (r *Repo) GetOpenShift(ctx context.Context, employeeID string) (*Shift, error) {
	var s Shift
	err := scanShift(r.Pool.QueryRow(ctx, `
SELECT `+shiftColumns+`
FROM shifts
WHERE employee_id = $1::uuid AND status = 'open'
ORDER BY started_at DESC
LIMIT 1`, employeeID), &s)
	if isNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repo) GetShiftByID(ctx context.Context, id string) (Shift, error) {
	var s Shift
	err := scanShift(r.Pool.QueryRow(ctx, `SELECT `+shiftColumns+` FROM shifts WHERE id = $1::uuid`, id), &s)
	if isNoRows(err) {
		return s, ErrNotFound
	}
	return s, err
}
