package repo

import (
	"context"
	"fmt"
	"time"
)

type ReportMark struct {
	MarkID          string    `json:"mark_id"`
	EmployeeID      string    `json:"employee_id"`
	EmployeeNumber  string    `json:"employee_number"`
	FullName        string    `json:"full_name"`
	Position        *string   `json:"position"`
	EventType       string    `json:"event_type"`
	TimestampServer time.Time `json:"timestamp_server"`
	Overtime        bool      `json:"overtime"`
	OutsideOffice   bool      `json:"outside_office"`
	AutoClosed      bool      `json:"auto_closed"`
	OperatorNumber  *string   `json:"operator_number"`
	ScheduledStart  *string   `json:"scheduled_start,omitempty"`
	ScheduledEnd    *string   `json:"scheduled_end,omitempty"`
	TimingStatus    string    `json:"timing_status"`
}

type ReportShift struct {
	ShiftID        string     `json:"shift_id"`
	EmployeeID     string     `json:"employee_id"`
	EmployeeNumber string     `json:"employee_number"`
	FullName       string     `json:"full_name"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at"`
	Status         string     `json:"status"`
	Overtime       bool       `json:"overtime"`
	WorkedMinutes  *int       `json:"worked_minutes"`
}

type ReportSummary struct {
	EmployeeID     string  `json:"employee_id"`
	EmployeeNumber string  `json:"employee_number"`
	FullName       string  `json:"full_name"`
	Position       *string `json:"position"`
	ShiftsTotal    int     `json:"shifts_total"`
	ShiftsClosed   int     `json:"shifts_closed"`
	WorkedMinutes  int     `json:"worked_minutes"`
	OvertimeShifts int     `json:"overtime_shifts"`
	MarksTotal     int     `json:"marks_total"`
}

func (r *Repo) ReportMarks(ctx context.Context, from, to time.Time, employeeIDs []string, position string, limit int) ([]ReportMark, error) {
	limit = normalizedLimit(limit, 5000, 50000)
	rows, err := r.Pool.Query(ctx, `
SELECT m.id::text, m.employee_id::text, e.employee_number, e.full_name, e.position,
       m.event_type, m.timestamp_server, m.overtime, m.outside_office,
       m.auto_closed_previous, m.operator_number,
       to_char(sch.start_time, 'HH24:MI'), to_char(sch.end_time, 'HH24:MI')
FROM marks m
JOIN employees e ON e.id = m.employee_id
LEFT JOIN LATERAL (
    SELECT s.start_time, s.end_time
    FROM schedules s
    WHERE s.employee_id = m.employee_id
      AND s.valid_from <= (m.timestamp_server AT TIME ZONE 'Asia/Novosibirsk')::date
      AND (s.valid_to IS NULL OR s.valid_to >= (m.timestamp_server AT TIME ZONE 'Asia/Novosibirsk')::date)
    ORDER BY s.is_active DESC, s.valid_from DESC
    LIMIT 1
) sch ON true
WHERE m.timestamp_server >= $1 AND m.timestamp_server < $2
  AND ($3::uuid[] IS NULL OR m.employee_id = ANY($3))
  AND ($4 = '' OR e.position ILIKE '%' || $4 || '%')
ORDER BY m.timestamp_server DESC
LIMIT $5`, from, to, employeeIDArray(employeeIDs), position, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ReportMark
	for rows.Next() {
		var item ReportMark
		if err := rows.Scan(&item.MarkID, &item.EmployeeID, &item.EmployeeNumber, &item.FullName,
			&item.Position, &item.EventType, &item.TimestampServer, &item.Overtime,
			&item.OutsideOffice, &item.AutoClosed, &item.OperatorNumber,
			&item.ScheduledStart, &item.ScheduledEnd); err != nil {
			return nil, err
		}
		item.TimingStatus = markTimingStatus(item)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repo) ReportShifts(ctx context.Context, from, to time.Time, employeeIDs []string, position string, limit int) ([]ReportShift, error) {
	limit = normalizedLimit(limit, 5000, 50000)
	rows, err := r.Pool.Query(ctx, `
SELECT s.id::text, s.employee_id::text, e.employee_number, e.full_name,
       s.started_at, s.ended_at, s.status, s.overtime,
       CASE WHEN s.ended_at IS NULL THEN NULL
            ELSE EXTRACT(EPOCH FROM (s.ended_at - s.started_at))::int / 60 END
FROM shifts s
JOIN employees e ON e.id = s.employee_id
WHERE s.started_at >= $1 AND s.started_at < $2
  AND ($3::uuid[] IS NULL OR s.employee_id = ANY($3))
  AND ($4 = '' OR e.position ILIKE '%' || $4 || '%')
ORDER BY s.started_at DESC
LIMIT $5`, from, to, employeeIDArray(employeeIDs), position, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ReportShift
	for rows.Next() {
		var item ReportShift
		if err := rows.Scan(&item.ShiftID, &item.EmployeeID, &item.EmployeeNumber, &item.FullName,
			&item.StartedAt, &item.EndedAt, &item.Status, &item.Overtime, &item.WorkedMinutes); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repo) ReportSummary(ctx context.Context, from, to time.Time, employeeIDs []string, position string) ([]ReportSummary, error) {
	rows, err := r.Pool.Query(ctx, `
WITH shift_stats AS (
    SELECT employee_id,
           count(*) AS shifts_total,
           count(*) FILTER (WHERE status <> 'open') AS shifts_closed,
           COALESCE(sum(
               CASE WHEN ended_at IS NULL THEN 0
                    ELSE EXTRACT(EPOCH FROM (ended_at - started_at))::int / 60 END
           ), 0) AS worked_minutes,
           count(*) FILTER (WHERE overtime) AS overtime_shifts
    FROM shifts
    WHERE started_at >= $1 AND started_at < $2
    GROUP BY employee_id
), mark_stats AS (
    SELECT employee_id, count(*) AS marks_total
    FROM marks
    WHERE timestamp_server >= $1 AND timestamp_server < $2
    GROUP BY employee_id
)
SELECT e.id::text, e.employee_number, e.full_name, e.position,
       COALESCE(s.shifts_total, 0), COALESCE(s.shifts_closed, 0),
       COALESCE(s.worked_minutes, 0), COALESCE(s.overtime_shifts, 0),
       COALESCE(m.marks_total, 0)
FROM employees e
LEFT JOIN shift_stats s ON s.employee_id = e.id
LEFT JOIN mark_stats m ON m.employee_id = e.id
WHERE ($3::uuid[] IS NULL OR e.id = ANY($3))
  AND ($4 = '' OR e.position ILIKE '%' || $4 || '%')
  AND (s.employee_id IS NOT NULL OR m.employee_id IS NOT NULL)
ORDER BY e.full_name
LIMIT 5000`, from, to, employeeIDArray(employeeIDs), position)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ReportSummary
	for rows.Next() {
		var item ReportSummary
		if err := rows.Scan(&item.EmployeeID, &item.EmployeeNumber, &item.FullName, &item.Position,
			&item.ShiftsTotal, &item.ShiftsClosed, &item.WorkedMinutes, &item.OvertimeShifts,
			&item.MarksTotal); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func markTimingStatus(item ReportMark) string {
	if item.ScheduledStart == nil || item.ScheduledEnd == nil {
		return ""
	}
	local := item.TimestampServer.In(time.FixedZone("NOV", 7*60*60))
	parseClock := func(value string) (int, int, bool) {
		var hour, minute int
		if _, err := fmt.Sscanf(value, "%d:%d", &hour, &minute); err != nil {
			return 0, 0, false
		}
		return hour, minute, true
	}
	if item.EventType == "check_in" {
		hour, minute, ok := parseClock(*item.ScheduledStart)
		if !ok {
			return ""
		}
		expected := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, local.Location())
		return arrivalStatus(local.Sub(expected))
	}
	hour, minute, ok := parseClock(*item.ScheduledEnd)
	if !ok {
		return ""
	}
	expected := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, local.Location())
	return departureStatus(local.Sub(expected))
}

func arrivalStatus(diff time.Duration) string {
	if diff < -15*time.Minute {
		return "пришёл рано"
	}
	if diff > 15*time.Minute {
		return "пришёл поздно"
	}
	return "пришёл вовремя"
}

func departureStatus(diff time.Duration) string {
	if diff < -15*time.Minute {
		return "ушёл рано"
	}
	if diff > 15*time.Minute {
		return "ушёл поздно"
	}
	return "ушёл вовремя"
}

func employeeIDArray(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	return ids
}
