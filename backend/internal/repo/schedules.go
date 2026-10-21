package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

const scheduleColumns = `
	id::text, employee_id::text, pattern_type, workdays::int[],
	to_char(start_time, 'HH24:MI'), to_char(end_time, 'HH24:MI'),
	to_char(valid_from, 'YYYY-MM-DD'), to_char(valid_to, 'YYYY-MM-DD'),
	is_active, source_file`

func scanSchedule(row interface{ Scan(...any) error }, s *Schedule) error {
	var workdays []int32
	if err := row.Scan(
		&s.ID, &s.EmployeeID, &s.PatternType, &workdays,
		&s.StartTime, &s.EndTime, &s.ValidFrom, &s.ValidTo,
		&s.IsActive, &s.SourceFile,
	); err != nil {
		return err
	}
	s.Workdays = make([]int, len(workdays))
	for i, day := range workdays {
		s.Workdays[i] = int(day)
	}
	return nil
}

func (r *Repo) GetActiveScheduleForDate(ctx context.Context, employeeID, date string) (Schedule, error) {
	var s Schedule
	err := scanSchedule(r.Pool.QueryRow(ctx, `
SELECT `+scheduleColumns+`
FROM schedules
WHERE employee_id = $1::uuid
  AND is_active
  AND valid_from <= $2::date
  AND (valid_to IS NULL OR valid_to >= $2::date)
ORDER BY valid_from DESC
LIMIT 1`, employeeID, date), &s)
	if isNoRows(err) {
		return s, ErrNotFound
	}
	return s, err
}

func (r *Repo) GetScheduleByID(ctx context.Context, id string) (Schedule, error) {
	var s Schedule
	err := scanSchedule(r.Pool.QueryRow(ctx, `SELECT `+scheduleColumns+` FROM schedules WHERE id = $1::uuid`, id), &s)
	if isNoRows(err) {
		return s, ErrNotFound
	}
	return s, err
}

func (r *Repo) ListSchedules(ctx context.Context, employeeID string) ([]Schedule, error) {
	rows, err := r.Pool.Query(ctx, `
SELECT `+scheduleColumns+`
FROM schedules
WHERE (NULLIF($1, '') IS NULL OR employee_id = NULLIF($1, '')::uuid)
ORDER BY is_active DESC, valid_from DESC, created_at DESC
LIMIT 1000`, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []Schedule
	for rows.Next() {
		var s Schedule
		if err := scanSchedule(rows, &s); err != nil {
			return nil, err
		}
		schedules = append(schedules, s)
	}
	return schedules, rows.Err()
}

// ListSchedulesPage возвращает страницу графиков с поиском по табельному номеру
// и ФИО сотрудника. Поля сотрудника заполняются, чтобы интерфейс не зависел от
// загрузки всего справочника.
func (r *Repo) ListSchedulesPage(ctx context.Context, employeeID, query string, limit, offset int) ([]Schedule, error) {
	limit = normalizedLimit(limit, 50, 500)
	offset = normalizedOffset(offset)
	rows, err := r.Pool.Query(ctx, `
SELECT s.id::text, s.employee_id::text, s.pattern_type, s.workdays::int[],
       to_char(s.start_time, 'HH24:MI'), to_char(s.end_time, 'HH24:MI'),
       to_char(s.valid_from, 'YYYY-MM-DD'), to_char(s.valid_to, 'YYYY-MM-DD'),
       s.is_active, s.source_file,
       e.employee_number, e.full_name
FROM schedules s
JOIN employees e ON e.id = s.employee_id
WHERE (NULLIF($1, '') IS NULL OR s.employee_id = NULLIF($1, '')::uuid)
  AND ($2 = '' OR e.employee_number ILIKE '%' || $2 || '%'
       OR e.full_name ILIKE '%' || $2 || '%')
ORDER BY s.is_active DESC, s.valid_from DESC, s.created_at DESC, s.id
LIMIT $3 OFFSET $4`, employeeID, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []Schedule
	for rows.Next() {
		var s Schedule
		var workdays []int32
		if err := rows.Scan(
			&s.ID, &s.EmployeeID, &s.PatternType, &workdays,
			&s.StartTime, &s.EndTime, &s.ValidFrom, &s.ValidTo,
			&s.IsActive, &s.SourceFile, &s.EmployeeNumber, &s.EmployeeFullName,
		); err != nil {
			return nil, err
		}
		s.Workdays = make([]int, len(workdays))
		for i, day := range workdays {
			s.Workdays[i] = int(day)
		}
		schedules = append(schedules, s)
	}
	return schedules, rows.Err()
}

// ReplaceSchedules заменяет активный график сотрудника набором новых записей.
// Активной становится запись, действующая на дату today; если такой нет —
// ближайшая будущая, иначе последняя прошедшая.
func (r *Repo) ReplaceSchedules(ctx context.Context, employeeID, today string, inputs []ScheduleInput) ([]Schedule, error) {
	if len(inputs) == 0 {
		return nil, errors.New("нет графиков для сохранения")
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var active bool
	if err := tx.QueryRow(ctx, `SELECT is_active FROM employees WHERE id = $1::uuid FOR UPDATE`, employeeID).Scan(&active); err != nil {
		if isNoRows(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if !active {
		return nil, ErrEmployeeInactive
	}

	if _, err := tx.Exec(ctx, `
UPDATE schedules SET is_active = false, updated_at = now()
WHERE employee_id = $1::uuid AND is_active`, employeeID); err != nil {
		return nil, err
	}

	created := make([]Schedule, 0, len(inputs))
	ids := make([]string, 0, len(inputs))
	for _, in := range inputs {
		var id string
		var createdBy any
		if in.CreatedBy != nil {
			createdBy = *in.CreatedBy
		}
		err := tx.QueryRow(ctx, `
INSERT INTO schedules (
    employee_id, pattern_type, workdays, start_time, end_time,
    valid_from, valid_to, is_active, source_file, created_by
) VALUES (
    $1::uuid, $2, $3::smallint[], $4::time, $5::time,
    $6::date, $7::date, false, $8, $9::uuid
)
RETURNING id::text`,
			employeeID, in.PatternType, toInt16(in.Workdays), in.StartTime, in.EndTime,
			in.ValidFrom, in.ValidTo, in.SourceFile, createdBy).Scan(&id)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
		created = append(created, Schedule{
			ID:          id,
			EmployeeID:  employeeID,
			PatternType: in.PatternType,
			Workdays:    in.Workdays,
			StartTime:   in.StartTime,
			EndTime:     in.EndTime,
			ValidFrom:   in.ValidFrom,
			ValidTo:     in.ValidTo,
			SourceFile:  in.SourceFile,
		})
	}

	activeIndex := chooseActiveSchedule(inputs, today)
	if activeIndex >= 0 {
		if _, err := tx.Exec(ctx, `
UPDATE schedules SET is_active = true, updated_at = now()
WHERE id = $1::uuid`, ids[activeIndex]); err != nil {
			return nil, err
		}
		created[activeIndex].IsActive = true
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return created, nil
}

func chooseActiveSchedule(inputs []ScheduleInput, today string) int {
	covering := -1
	future := -1
	past := -1
	for i, in := range inputs {
		switch {
		case in.ValidFrom <= today && (in.ValidTo == nil || today <= *in.ValidTo):
			if covering < 0 || in.ValidFrom > inputs[covering].ValidFrom {
				covering = i
			}
		case in.ValidFrom > today:
			if future < 0 || in.ValidFrom < inputs[future].ValidFrom {
				future = i
			}
		default:
			if past < 0 || in.ValidFrom > inputs[past].ValidFrom {
				past = i
			}
		}
	}
	switch {
	case covering >= 0:
		return covering
	case future >= 0:
		return future
	default:
		return past
	}
}

func toInt16(values []int) []int16 {
	out := make([]int16, len(values))
	for i, value := range values {
		out[i] = int16(value)
	}
	return out
}

func scheduleOverlapsTx(ctx context.Context, tx pgx.Tx, employeeID, excludeID, validFrom string, validTo *string) (bool, error) {
	var exists bool
	if excludeID == "" {
		err := tx.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM schedules
    WHERE employee_id = $1::uuid
      AND is_active
      AND valid_from <= COALESCE($3::date, 'infinity'::date)
      AND COALESCE(valid_to, 'infinity'::date) >= $2::date
)`, employeeID, validFrom, validTo).Scan(&exists)
		return exists, err
	}
	err := tx.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM schedules
    WHERE employee_id = $1::uuid
      AND is_active
      AND id <> $2::uuid
      AND valid_from <= COALESCE($4::date, 'infinity'::date)
      AND COALESCE(valid_to, 'infinity'::date) >= $3::date
)`, employeeID, excludeID, validFrom, validTo).Scan(&exists)
	return exists, err
}

func (r *Repo) CreateSchedule(ctx context.Context, in ScheduleInput, active bool) (Schedule, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Schedule{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var employeeActive bool
	if err := tx.QueryRow(ctx, `
SELECT is_active FROM employees WHERE id = $1::uuid FOR UPDATE`, in.EmployeeID).Scan(&employeeActive); err != nil {
		if isNoRows(err) {
			return Schedule{}, ErrNotFound
		}
		return Schedule{}, err
	}
	if !employeeActive {
		return Schedule{}, ErrEmployeeInactive
	}
	if active {
		if _, err := tx.Exec(ctx, `
UPDATE schedules SET is_active = false, updated_at = now()
WHERE employee_id = $1::uuid AND is_active`, in.EmployeeID); err != nil {
			return Schedule{}, err
		}
	}
	overlaps, err := scheduleOverlapsTx(ctx, tx, in.EmployeeID, "", in.ValidFrom, in.ValidTo)
	if err != nil {
		return Schedule{}, err
	}
	if overlaps {
		return Schedule{}, ErrScheduleOverlap
	}

	var createdBy any
	if in.CreatedBy != nil {
		createdBy = *in.CreatedBy
	}
	var s Schedule
	err = scanSchedule(tx.QueryRow(ctx, `
INSERT INTO schedules (
    employee_id, pattern_type, workdays, start_time, end_time,
    valid_from, valid_to, is_active, source_file, created_by
) VALUES (
    $1::uuid, $2, $3::smallint[], $4::time, $5::time,
    $6::date, $7::date, $8, NULL, $9::uuid
)
RETURNING `+scheduleColumns,
		in.EmployeeID, in.PatternType, toInt16(in.Workdays), in.StartTime, in.EndTime,
		in.ValidFrom, in.ValidTo, active, createdBy), &s)
	if err != nil {
		return Schedule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Schedule{}, err
	}
	return s, nil
}

func (r *Repo) UpdateSchedule(ctx context.Context, id string, in ScheduleInput, active bool) (Schedule, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Schedule{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var employeeID string
	if err := tx.QueryRow(ctx, `
SELECT employee_id::text FROM schedules WHERE id = $1::uuid FOR UPDATE`, id).Scan(&employeeID); err != nil {
		if isNoRows(err) {
			return Schedule{}, ErrNotFound
		}
		return Schedule{}, err
	}
	var employeeActive bool
	if err := tx.QueryRow(ctx, `
SELECT is_active FROM employees WHERE id = $1::uuid`, employeeID).Scan(&employeeActive); err != nil {
		return Schedule{}, err
	}
	if !employeeActive {
		return Schedule{}, ErrEmployeeInactive
	}
	if active {
		if _, err := tx.Exec(ctx, `
UPDATE schedules SET is_active = false, updated_at = now()
WHERE employee_id = $1::uuid AND is_active AND id <> $2::uuid`, employeeID, id); err != nil {
			return Schedule{}, err
		}
	}
	overlaps, err := scheduleOverlapsTx(ctx, tx, employeeID, id, in.ValidFrom, in.ValidTo)
	if err != nil {
		return Schedule{}, err
	}
	if overlaps {
		return Schedule{}, ErrScheduleOverlap
	}

	var s Schedule
	err = scanSchedule(tx.QueryRow(ctx, `
UPDATE schedules SET
    pattern_type = $2, workdays = $3::smallint[], start_time = $4::time, end_time = $5::time,
    valid_from = $6::date, valid_to = $7::date, is_active = $8, updated_at = now()
WHERE id = $1::uuid
RETURNING `+scheduleColumns,
		id, in.PatternType, toInt16(in.Workdays), in.StartTime, in.EndTime,
		in.ValidFrom, in.ValidTo, active), &s)
	if err != nil {
		return Schedule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Schedule{}, err
	}
	return s, nil
}

func (r *Repo) DeleteSchedule(ctx context.Context, id string) error {
	tag, err := r.Pool.Exec(ctx, `DELETE FROM schedules WHERE id = $1::uuid`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
