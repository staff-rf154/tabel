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
	Department      *string   `json:"department"`
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
	Position       *string    `json:"position"`
	Department     *string    `json:"department"`
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
	Department     *string `json:"department"`
	ShiftsTotal    int     `json:"shifts_total"`
	ShiftsClosed   int     `json:"shifts_closed"`
	WorkedMinutes  int     `json:"worked_minutes"`
	OvertimeShifts int     `json:"overtime_shifts"`
	MarksTotal     int     `json:"marks_total"`
}

func (r *Repo) ReportMarks(ctx context.Context, from, to time.Time, employeeIDs []string, position string, limit int) ([]ReportMark, error) {
	limit = normalizedLimit(limit, 5000, 50000)
	rows, err := r.Pool.Query(ctx, `
SELECT m.id::text, m.employee_id::text, e.employee_number, e.full_name, e.position, d.name,
       m.event_type, m.timestamp_server, m.overtime, m.outside_office,
       m.auto_closed_previous, m.operator_number,
       to_char(sch.start_time, 'HH24:MI'), to_char(sch.end_time, 'HH24:MI')
FROM marks m
JOIN employees e ON e.id = m.employee_id
LEFT JOIN departments d ON d.id = e.department_id
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
			&item.Position, &item.Department, &item.EventType, &item.TimestampServer, &item.Overtime,
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
SELECT s.id::text, s.employee_id::text, e.employee_number, e.full_name, e.position, d.name,
       s.started_at, s.ended_at, s.status, s.overtime,
       CASE WHEN s.ended_at IS NULL THEN NULL
            ELSE EXTRACT(EPOCH FROM (s.ended_at - s.started_at))::int / 60 END
FROM shifts s
JOIN employees e ON e.id = s.employee_id
LEFT JOIN departments d ON d.id = e.department_id
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
			&item.Position, &item.Department, &item.StartedAt, &item.EndedAt, &item.Status,
			&item.Overtime, &item.WorkedMinutes); err != nil {
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
SELECT e.id::text, e.employee_number, e.full_name, e.position, d.name,
       COALESCE(s.shifts_total, 0), COALESCE(s.shifts_closed, 0),
       COALESCE(s.worked_minutes, 0), COALESCE(s.overtime_shifts, 0),
       COALESCE(m.marks_total, 0)
FROM employees e
LEFT JOIN departments d ON d.id = e.department_id
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
		if err := rows.Scan(&item.EmployeeID, &item.EmployeeNumber, &item.FullName,
			&item.Position, &item.Department, &item.ShiftsTotal, &item.ShiftsClosed, &item.WorkedMinutes, &item.OvertimeShifts,
			&item.MarksTotal); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type ReportWorkedTime struct {
	EmployeeID     string  `json:"employee_id"`
	EmployeeNumber string  `json:"employee_number"`
	FullName       string  `json:"full_name"`
	Position       *string `json:"position"`
	Department     *string `json:"department"`
	DaysWorked     int     `json:"days_worked"`
	ShiftsTotal    int     `json:"shifts_total"`
	WorkedMinutes  int     `json:"worked_minutes"`
	AverageMinutes int     `json:"average_minutes"`
	OvertimeShifts int     `json:"overtime_shifts"`
}

type ReportFirstLast struct {
	EmployeeID     string     `json:"employee_id"`
	EmployeeNumber string     `json:"employee_number"`
	FullName       string     `json:"full_name"`
	Position       *string    `json:"position"`
	Department     *string    `json:"department"`
	WorkDate       string     `json:"work_date"`
	FirstIn        time.Time  `json:"first_in"`
	LastOut        *time.Time `json:"last_out"`
	WorkedMinutes  int        `json:"worked_minutes"`
}

type ReportEntryExit struct {
	EmployeeID     string     `json:"employee_id"`
	EmployeeNumber string     `json:"employee_number"`
	FullName       string     `json:"full_name"`
	Position       *string    `json:"position"`
	Department     *string    `json:"department"`
	WorkDate       string     `json:"work_date"`
	CheckIn        time.Time  `json:"check_in"`
	CheckOut       *time.Time `json:"check_out"`
	Minutes        *int       `json:"minutes"`
	Status         string     `json:"status"`
}

type ReportDaily struct {
	EmployeeID      string     `json:"employee_id"`
	EmployeeNumber  string     `json:"employee_number"`
	FullName        string     `json:"full_name"`
	Position        *string    `json:"position"`
	Department      *string    `json:"department"`
	WorkDate        string     `json:"work_date"`
	CheckIn         time.Time  `json:"check_in"`
	CheckOut        *time.Time `json:"check_out"`
	PresenceMinutes int        `json:"presence_minutes"`
	WorkedMinutes   int        `json:"worked_minutes"`
	ShiftsTotal     int        `json:"shifts_total"`
	OvertimeShifts  int        `json:"overtime_shifts"`
}

type ReportTimesheetDay struct {
	Date          string `json:"date"`
	WorkedMinutes int    `json:"worked_minutes"`
	Code          string `json:"code"`
}

type ReportTimesheet struct {
	EmployeeID     string               `json:"employee_id"`
	EmployeeNumber string               `json:"employee_number"`
	FullName       string               `json:"full_name"`
	Position       *string              `json:"position"`
	Department     *string              `json:"department"`
	Days           []ReportTimesheetDay `json:"days"`
	DaysWorked     int                  `json:"days_worked"`
	WorkedMinutes  int                  `json:"worked_minutes"`
	OvertimeShifts int                  `json:"overtime_shifts"`
}

func (r *Repo) ReportWorkedTime(ctx context.Context, from, to time.Time, employeeIDs []string, position string) ([]ReportWorkedTime, error) {
	rows, err := r.Pool.Query(ctx, `
SELECT e.id::text, e.employee_number, e.full_name, e.position, d.name,
       count(DISTINCT (s.started_at AT TIME ZONE 'Asia/Novosibirsk')::date)::int,
       count(*)::int,
       COALESCE(sum(
           CASE WHEN s.ended_at IS NULL THEN 0
                ELSE EXTRACT(EPOCH FROM (s.ended_at - s.started_at))::int / 60 END
       ), 0)::int,
       count(*) FILTER (WHERE s.overtime)::int
FROM shifts s
JOIN employees e ON e.id = s.employee_id
LEFT JOIN departments d ON d.id = e.department_id
WHERE s.started_at >= $1 AND s.started_at < $2
  AND ($3::uuid[] IS NULL OR s.employee_id = ANY($3))
  AND ($4 = '' OR e.position ILIKE '%' || $4 || '%')
GROUP BY e.id, e.employee_number, e.full_name, e.position, d.name
ORDER BY e.full_name
LIMIT 5000`, from, to, employeeIDArray(employeeIDs), position)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ReportWorkedTime
	for rows.Next() {
		var item ReportWorkedTime
		if err := rows.Scan(&item.EmployeeID, &item.EmployeeNumber, &item.FullName, &item.Position,
			&item.Department, &item.DaysWorked, &item.ShiftsTotal, &item.WorkedMinutes,
			&item.OvertimeShifts); err != nil {
			return nil, err
		}
		if item.DaysWorked > 0 {
			item.AverageMinutes = item.WorkedMinutes / item.DaysWorked
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repo) ReportFirstLast(ctx context.Context, from, to time.Time, employeeIDs []string, position string) ([]ReportFirstLast, error) {
	rows, err := r.Pool.Query(ctx, `
SELECT e.id::text, e.employee_number, e.full_name, e.position, d.name,
       to_char((s.started_at AT TIME ZONE 'Asia/Novosibirsk')::date, 'YYYY-MM-DD'),
       min(s.started_at), max(s.ended_at),
       COALESCE(sum(
           CASE WHEN s.ended_at IS NULL THEN 0
                ELSE EXTRACT(EPOCH FROM (s.ended_at - s.started_at))::int / 60 END
       ), 0)::int
FROM shifts s
JOIN employees e ON e.id = s.employee_id
LEFT JOIN departments d ON d.id = e.department_id
WHERE s.started_at >= $1 AND s.started_at < $2
  AND ($3::uuid[] IS NULL OR s.employee_id = ANY($3))
  AND ($4 = '' OR e.position ILIKE '%' || $4 || '%')
GROUP BY e.id, e.employee_number, e.full_name, e.position, d.name,
         (s.started_at AT TIME ZONE 'Asia/Novosibirsk')::date
ORDER BY e.full_name, (s.started_at AT TIME ZONE 'Asia/Novosibirsk')::date
LIMIT 50000`, from, to, employeeIDArray(employeeIDs), position)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ReportFirstLast
	for rows.Next() {
		var item ReportFirstLast
		if err := rows.Scan(&item.EmployeeID, &item.EmployeeNumber, &item.FullName,
			&item.Position, &item.Department, &item.WorkDate, &item.FirstIn, &item.LastOut,
			&item.WorkedMinutes); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repo) ReportEntriesExits(ctx context.Context, from, to time.Time, employeeIDs []string, position string, limit int) ([]ReportEntryExit, error) {
	limit = normalizedLimit(limit, 20000, 50000)
	rows, err := r.Pool.Query(ctx, `
SELECT e.id::text, e.employee_number, e.full_name, e.position, d.name,
       to_char(s.started_at AT TIME ZONE 'Asia/Novosibirsk', 'YYYY-MM-DD'),
       s.started_at, s.ended_at,
       CASE WHEN s.ended_at IS NULL THEN NULL
            ELSE EXTRACT(EPOCH FROM (s.ended_at - s.started_at))::int / 60 END,
       s.status
FROM shifts s
JOIN employees e ON e.id = s.employee_id
LEFT JOIN departments d ON d.id = e.department_id
WHERE s.started_at >= $1 AND s.started_at < $2
  AND ($3::uuid[] IS NULL OR s.employee_id = ANY($3))
  AND ($4 = '' OR e.position ILIKE '%' || $4 || '%')
ORDER BY e.full_name, s.started_at
LIMIT $5`, from, to, employeeIDArray(employeeIDs), position, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ReportEntryExit
	for rows.Next() {
		var item ReportEntryExit
		if err := rows.Scan(&item.EmployeeID, &item.EmployeeNumber, &item.FullName,
			&item.Position, &item.Department, &item.WorkDate, &item.CheckIn,
			&item.CheckOut, &item.Minutes, &item.Status); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repo) ReportDaily(ctx context.Context, from, to time.Time, employeeIDs []string, position string) ([]ReportDaily, error) {
	rows, err := r.Pool.Query(ctx, `
SELECT e.id::text, e.employee_number, e.full_name, e.position, d.name,
       to_char((s.started_at AT TIME ZONE 'Asia/Novosibirsk')::date, 'YYYY-MM-DD'),
       min(s.started_at), max(s.ended_at),
       CASE WHEN max(s.ended_at) IS NULL THEN 0
            ELSE EXTRACT(EPOCH FROM (max(s.ended_at) - min(s.started_at)))::int / 60 END,
       COALESCE(sum(
           CASE WHEN s.ended_at IS NULL THEN 0
                ELSE EXTRACT(EPOCH FROM (s.ended_at - s.started_at))::int / 60 END
       ), 0)::int,
       count(*)::int,
       count(*) FILTER (WHERE s.overtime)::int
FROM shifts s
JOIN employees e ON e.id = s.employee_id
LEFT JOIN departments d ON d.id = e.department_id
WHERE s.started_at >= $1 AND s.started_at < $2
  AND ($3::uuid[] IS NULL OR s.employee_id = ANY($3))
  AND ($4 = '' OR e.position ILIKE '%' || $4 || '%')
GROUP BY e.id, e.employee_number, e.full_name, e.position, d.name,
         (s.started_at AT TIME ZONE 'Asia/Novosibirsk')::date
ORDER BY e.full_name, (s.started_at AT TIME ZONE 'Asia/Novosibirsk')::date
LIMIT 50000`, from, to, employeeIDArray(employeeIDs), position)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ReportDaily
	for rows.Next() {
		var item ReportDaily
		if err := rows.Scan(&item.EmployeeID, &item.EmployeeNumber, &item.FullName,
			&item.Position, &item.Department, &item.WorkDate, &item.CheckIn,
			&item.CheckOut, &item.PresenceMinutes, &item.WorkedMinutes,
			&item.ShiftsTotal, &item.OvertimeShifts); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repo) ResolveReportEmployees(ctx context.Context, values []string) ([]string, []string, error) {
	if len(values) == 0 {
		return nil, nil, nil
	}
	rows, err := r.Pool.Query(ctx, `
WITH q AS (SELECT unnest($1::text[]) AS value)
SELECT DISTINCT q.value, e.id::text
FROM q
JOIN employees e ON lower(e.employee_number) = lower(q.value)
                 OR e.full_name ILIKE '%' || q.value || '%'
ORDER BY q.value, e.id::text`, values)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	found := make(map[string]bool, len(values))
	seen := make(map[string]bool)
	var ids []string
	for rows.Next() {
		var value, id string
		if err := rows.Scan(&value, &id); err != nil {
			return nil, nil, err
		}
		found[value] = true
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	var missing []string
	for _, value := range values {
		if !found[value] {
			missing = append(missing, value)
		}
	}
	return ids, missing, nil
}

func BuildTimesheet(items []ReportDaily) []ReportTimesheet {
	order := make([]string, 0)
	byEmployee := make(map[string]*ReportTimesheet)
	for _, item := range items {
		row := byEmployee[item.EmployeeID]
		if row == nil {
			row = &ReportTimesheet{
				EmployeeID: item.EmployeeID, EmployeeNumber: item.EmployeeNumber,
				FullName: item.FullName, Position: item.Position, Department: item.Department,
				Days: []ReportTimesheetDay{},
			}
			byEmployee[item.EmployeeID] = row
			order = append(order, item.EmployeeID)
		}
		row.Days = append(row.Days, ReportTimesheetDay{
			Date: item.WorkDate, WorkedMinutes: item.WorkedMinutes,
			Code: "Я",
		})
		row.DaysWorked++
		row.WorkedMinutes += item.WorkedMinutes
		row.OvertimeShifts += item.OvertimeShifts
	}
	result := make([]ReportTimesheet, 0, len(order))
	for _, id := range order {
		result = append(result, *byEmployee[id])
	}
	return result
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
