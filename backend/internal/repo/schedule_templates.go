package repo

import (
	"context"
)

const scheduleTemplateColumns = `
	id::text, name, pattern_type, workdays::int[],
	to_char(start_time, 'HH24:MI'), to_char(end_time, 'HH24:MI')`

func scanScheduleTemplate(row interface{ Scan(...any) error }, t *ScheduleTemplate) error {
	var workdays []int32
	if err := row.Scan(&t.ID, &t.Name, &t.PatternType, &workdays, &t.StartTime, &t.EndTime); err != nil {
		return err
	}
	t.Workdays = make([]int, len(workdays))
	for i, day := range workdays {
		t.Workdays[i] = int(day)
	}
	return nil
}

func (r *Repo) ListScheduleTemplates(ctx context.Context) ([]ScheduleTemplate, error) {
	rows, err := r.Pool.Query(ctx, `
SELECT `+scheduleTemplateColumns+`
FROM schedule_templates
ORDER BY lower(name), created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ScheduleTemplate
	for rows.Next() {
		var item ScheduleTemplate
		if err := scanScheduleTemplate(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repo) GetScheduleTemplate(ctx context.Context, id string) (ScheduleTemplate, error) {
	var item ScheduleTemplate
	err := scanScheduleTemplate(r.Pool.QueryRow(ctx, `
SELECT `+scheduleTemplateColumns+` FROM schedule_templates WHERE id = $1::uuid`, id), &item)
	if isNoRows(err) {
		return ScheduleTemplate{}, ErrNotFound
	}
	return item, err
}

func (r *Repo) CreateScheduleTemplate(ctx context.Context, in ScheduleTemplateInput) (ScheduleTemplate, error) {
	var createdBy any
	if in.CreatedBy != nil {
		createdBy = *in.CreatedBy
	}
	var item ScheduleTemplate
	err := scanScheduleTemplate(r.Pool.QueryRow(ctx, `
INSERT INTO schedule_templates (name, pattern_type, workdays, start_time, end_time, created_by)
VALUES ($1, $2, $3::smallint[], $4::time, $5::time, $6::uuid)
RETURNING `+scheduleTemplateColumns,
		in.Name, in.PatternType, toInt16(in.Workdays), in.StartTime, in.EndTime, createdBy), &item)
	return item, err
}

func (r *Repo) UpdateScheduleTemplate(ctx context.Context, id string, in ScheduleTemplateInput) (ScheduleTemplate, error) {
	var item ScheduleTemplate
	err := scanScheduleTemplate(r.Pool.QueryRow(ctx, `
UPDATE schedule_templates
SET name = $2, pattern_type = $3, workdays = $4::smallint[],
    start_time = $5::time, end_time = $6::time, updated_at = now()
WHERE id = $1::uuid
RETURNING `+scheduleTemplateColumns,
		id, in.Name, in.PatternType, toInt16(in.Workdays), in.StartTime, in.EndTime), &item)
	if isNoRows(err) {
		return ScheduleTemplate{}, ErrNotFound
	}
	return item, err
}

func (r *Repo) DeleteScheduleTemplate(ctx context.Context, id string) error {
	tag, err := r.Pool.Exec(ctx, `DELETE FROM schedule_templates WHERE id = $1::uuid`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) ListScheduleTargetEmployees(ctx context.Context, targetType, targetID string) ([]string, error) {
	query := `SELECT id::text FROM employees WHERE is_active`
	args := []any{}
	switch targetType {
	case "all":
	case "department":
		query += ` AND department_id = $1::uuid`
		args = append(args, targetID)
	case "position":
		query += ` AND position_id = $1::uuid`
		args = append(args, targetID)
	case "employee":
		query += ` AND id = $1::uuid`
		args = append(args, targetID)
	default:
		return nil, ErrNotFound
	}
	query += ` ORDER BY full_name`
	rows, err := r.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
