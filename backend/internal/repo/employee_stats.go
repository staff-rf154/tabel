package repo

import (
	"context"
	"time"
)

type EmployeeMonthStats struct {
	DaysWorked     int    `json:"days_worked"`
	ShiftsTotal    int    `json:"shifts_total"`
	ShiftsClosed   int    `json:"shifts_closed"`
	WorkedMinutes  int    `json:"worked_minutes"`
	OvertimeShifts int    `json:"overtime_shifts"`
	LastCheckIns   []Mark `json:"last_check_ins"`
}

func (r *Repo) EmployeeMonthStats(ctx context.Context, employeeID string, from, to time.Time, lastLimit int) (EmployeeMonthStats, error) {
	if lastLimit <= 0 || lastLimit > 50 {
		lastLimit = 10
	}

	var stats EmployeeMonthStats
	err := r.Pool.QueryRow(ctx, `
SELECT count(*),
       count(*) FILTER (WHERE ended_at IS NOT NULL),
       count(DISTINCT (started_at AT TIME ZONE 'Asia/Novosibirsk')::date),
       COALESCE(sum(
           CASE WHEN ended_at IS NULL
                THEN GREATEST(0, EXTRACT(EPOCH FROM (LEAST(now(), $3::timestamptz) - started_at))::int / 60)
                ELSE EXTRACT(EPOCH FROM (ended_at - started_at))::int / 60
           END
       ), 0),
       count(*) FILTER (WHERE overtime)
FROM shifts
WHERE employee_id = $1::uuid
  AND started_at >= $2
  AND started_at < $3`, employeeID, from, to).Scan(
		&stats.ShiftsTotal,
		&stats.ShiftsClosed,
		&stats.DaysWorked,
		&stats.WorkedMinutes,
		&stats.OvertimeShifts,
	)
	if err != nil {
		return EmployeeMonthStats{}, err
	}

	rows, err := r.Pool.Query(ctx, `
SELECT `+markColumns+`
FROM marks
WHERE employee_id = $1::uuid
  AND event_type = 'check_in'
  AND timestamp_server >= $2
  AND timestamp_server < $3
ORDER BY timestamp_server DESC
LIMIT $4`, employeeID, from, to, lastLimit)
	if err != nil {
		return EmployeeMonthStats{}, err
	}
	defer rows.Close()

	stats.LastCheckIns = []Mark{}
	for rows.Next() {
		var mark Mark
		if err := scanMark(rows, &mark); err != nil {
			return EmployeeMonthStats{}, err
		}
		stats.LastCheckIns = append(stats.LastCheckIns, mark)
	}
	if err := rows.Err(); err != nil {
		return EmployeeMonthStats{}, err
	}
	return stats, nil
}
