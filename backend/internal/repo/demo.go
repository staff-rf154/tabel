package repo

import (
	"context"
	"time"
)

var demoZone = time.FixedZone("NOV", 7*60*60)

type demoEmployee struct {
	Number     string
	LastName   string
	FirstName  string
	MiddleName string
	Department string
	Position   string
	Email      string
	Phone      string
	HiredAt    string
	Schedule   string
	StartTime  string
	EndTime    string
	Workdays   []int
}

var demoEmployees = []demoEmployee{
	{"DEMO-0001", "Иванов", "Иван", "Иванович", "Отдел разработки", "Ведущий разработчик", "i.ivanov@example.com", "+79000000001", "2023-02-13", "5/2", "09:00", "18:00", []int{1, 2, 3, 4, 5}},
	{"DEMO-0002", "Петрова", "Мария", "Сергеевна", "Бухгалтерия", "Бухгалтер", "m.petrova@example.com", "+79000000002", "2023-06-05", "5/2", "09:00", "18:00", []int{1, 2, 3, 4, 5}},
	{"DEMO-0003", "Сидоров", "Пётр", "Алексеевич", "Склад", "Кладовщик", "p.sidorov@example.com", "+79000000003", "2024-01-15", "5/2", "08:00", "17:00", []int{1, 2, 3, 4, 5}},
	{"DEMO-0004", "Кузнецова", "Анна", "Викторовна", "Отдел кадров", "Специалист по кадрам", "a.kuznetsova@example.com", "+79000000004", "2024-03-04", "5/2", "10:00", "19:00", []int{1, 2, 3, 4, 5}},
	{"DEMO-0005", "Смирнов", "Алексей", "Николаевич", "Отдел разработки", "Программист", "a.smirnov@example.com", "+79000000005", "2024-05-20", "5/2", "10:00", "19:00", []int{1, 2, 3, 4, 5}},
	{"DEMO-0006", "Волкова", "Елена", "Дмитриевна", "Бухгалтерия", "Главный бухгалтер", "e.volkova@example.com", "+79000000006", "2022-09-12", "5/2", "09:00", "18:00", []int{1, 2, 3, 4, 5}},
	{"DEMO-0007", "Морозов", "Дмитрий", "Сергеевич", "Склад", "Водитель", "d.morozov@example.com", "+79000000007", "2024-07-01", "5/2", "08:00", "17:00", []int{1, 2, 3, 4, 5}},
	{"DEMO-0008", "Соколова", "Ольга", "Андреевна", "Отдел продаж", "Менеджер по продажам", "o.sokolova@example.com", "+79000000008", "2023-11-13", "5/2", "09:00", "18:00", []int{1, 2, 3, 4, 5}},
	{"DEMO-0009", "Попов", "Никита", "Владимирович", "Отдел продаж", "Менеджер по продажам", "n.popov@example.com", "+79000000009", "2025-01-20", "5/2", "09:00", "18:00", []int{1, 2, 3, 4, 5}},
	{"DEMO-0010", "Лебедева", "Татьяна", "Игоревна", "Администрация", "Офис-менеджер", "t.lebedeva@example.com", "+79000000010", "2024-10-07", "5/2", "09:00", "18:00", []int{1, 2, 3, 4, 5}},
}

func (r *Repo) SeedDemo(ctx context.Context, createdBy *string) (int, error) {
	now := time.Now().In(demoZone)
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, demoZone)
	seeded := 0

	for _, month := range []time.Time{firstOfMonth.AddDate(0, -1, 0), firstOfMonth, firstOfMonth.AddDate(0, 1, 0)} {
		if err := r.EnsureMarksPartition(ctx, month.Format("2006-01-02")); err != nil {
			return seeded, err
		}
	}

	for _, demo := range demoEmployees {
		fullName := demo.LastName + " " + demo.FirstName + " " + demo.MiddleName
		var employeeID string
		if err := r.Pool.QueryRow(ctx, `
WITH dept AS (
    INSERT INTO departments (name) VALUES ($6)
    ON CONFLICT (name) DO UPDATE SET is_active = true, updated_at = now()
    RETURNING id
), pos AS (
    INSERT INTO positions (name) VALUES ($7)
    ON CONFLICT (name) DO UPDATE SET is_active = true, updated_at = now()
    RETURNING id
)
INSERT INTO employees (employee_number, full_name, last_name, first_name, middle_name,
                       email, phone, position, department_id, position_id, is_active, hired_at)
SELECT $1, $2, $3, $4, $5, $8, $9, $7, dept.id, pos.id, true, $10::date
FROM dept, pos
ON CONFLICT (employee_number) DO UPDATE
SET full_name = EXCLUDED.full_name,
    last_name = EXCLUDED.last_name,
    first_name = EXCLUDED.first_name,
    middle_name = EXCLUDED.middle_name,
    email = EXCLUDED.email,
    phone = EXCLUDED.phone,
    position = EXCLUDED.position,
    department_id = EXCLUDED.department_id,
    position_id = EXCLUDED.position_id,
    is_active = true,
    fired_at = NULL,
    hired_at = EXCLUDED.hired_at,
    updated_at = now()
RETURNING id::text`,
			demo.Number, fullName, demo.LastName, demo.FirstName, demo.MiddleName,
			demo.Department, demo.Position, demo.Email, demo.Phone, demo.HiredAt).Scan(&employeeID); err != nil {
			return seeded, err
		}

		if _, err := r.Pool.Exec(ctx, `
INSERT INTO schedules (employee_id, pattern_type, workdays, start_time, end_time, valid_from, is_active, created_by)
VALUES ($1::uuid, $2, $3, $4::time, $5::time, $6, true, $7::uuid)
ON CONFLICT (employee_id) WHERE is_active DO UPDATE
SET pattern_type = EXCLUDED.pattern_type,
    workdays = EXCLUDED.workdays,
    start_time = EXCLUDED.start_time,
    end_time = EXCLUDED.end_time,
    valid_from = EXCLUDED.valid_from,
    valid_to = NULL,
    updated_at = now()`,
			employeeID, demo.Schedule, demo.Workdays, demo.StartTime, demo.EndTime,
			firstOfMonth.Format("2006-01-02"), createdBy); err != nil {
			return seeded, err
		}

		if _, err := r.Pool.Exec(ctx, `DELETE FROM marks WHERE employee_id = $1::uuid`, employeeID); err != nil {
			return seeded, err
		}
		if _, err := r.Pool.Exec(ctx, `DELETE FROM shifts WHERE employee_id = $1::uuid`, employeeID); err != nil {
			return seeded, err
		}
		if err := r.seedDemoShifts(ctx, employeeID, now, demo); err != nil {
			return seeded, err
		}
		seeded++
	}
	return seeded, nil
}

func (r *Repo) seedDemoShifts(ctx context.Context, employeeID string, now time.Time, demo demoEmployee) error {
	startClock, err := time.Parse("15:04", demo.StartTime)
	if err != nil {
		return err
	}
	endClock, err := time.Parse("15:04", demo.EndTime)
	if err != nil {
		return err
	}
	workday := make(map[int]bool, len(demo.Workdays))
	for _, day := range demo.Workdays {
		workday[day] = true
	}

	for offset := 0; offset <= 20; offset++ {
		day := now.AddDate(0, 0, -offset)
		isoWeekday := int(day.Weekday())
		if isoWeekday == 0 {
			isoWeekday = 7
		}
		if !workday[isoWeekday] {
			continue
		}
		start := time.Date(day.Year(), day.Month(), day.Day(), startClock.Hour(), startClock.Minute(), 0, 0, demoZone)
		end := time.Date(day.Year(), day.Month(), day.Day(), endClock.Hour(), endClock.Minute(), 0, 0, demoZone)
		var shiftID string
		if err := r.Pool.QueryRow(ctx, `
INSERT INTO shifts (employee_id, started_at, ended_at, status, overtime)
VALUES ($1::uuid, $2, $3, 'closed', false)
RETURNING id::text`, employeeID, start, end).Scan(&shiftID); err != nil {
			return err
		}
		for _, mark := range []struct {
			eventType string
			at        time.Time
			lat       float64
			lon       float64
		}{
			{"check_in", start, 55.0302, 82.9204},
			{"check_out", end, 55.0307, 82.9211},
		} {
			var markID string
			if err := r.Pool.QueryRow(ctx, `
INSERT INTO marks (employee_id, event_type, timestamp_server, qr_source, client_type, shift_id, idempotency_key, lat, lon, accuracy_m)
VALUES ($1::uuid, $2, $3, 'static', 'web', $4::uuid, gen_random_uuid(), $5, $6, 12)
RETURNING id::text`, employeeID, mark.eventType, mark.at, shiftID, mark.lat, mark.lon).Scan(&markID); err != nil {
				return err
			}
			if mark.eventType == "check_in" {
				if _, err := r.Pool.Exec(ctx, `UPDATE shifts SET check_in_mark_id = $2::uuid WHERE id = $1::uuid`, shiftID, markID); err != nil {
					return err
				}
			} else {
				if _, err := r.Pool.Exec(ctx, `UPDATE shifts SET check_out_mark_id = $2::uuid WHERE id = $1::uuid`, shiftID, markID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (r *Repo) ClearDemo(ctx context.Context) (int, error) {
	var count int
	if err := r.Pool.QueryRow(ctx, `SELECT count(*) FROM employees WHERE employee_number LIKE 'DEMO-%'`).Scan(&count); err != nil {
		return 0, err
	}
	if _, err := r.Pool.Exec(ctx, `
DELETE FROM marks WHERE employee_id IN (SELECT id FROM employees WHERE employee_number LIKE 'DEMO-%')`); err != nil {
		return 0, err
	}
	if _, err := r.Pool.Exec(ctx, `
DELETE FROM shifts WHERE employee_id IN (SELECT id FROM employees WHERE employee_number LIKE 'DEMO-%')`); err != nil {
		return 0, err
	}
	if _, err := r.Pool.Exec(ctx, `
DELETE FROM mark_idempotency WHERE employee_id IN (SELECT id FROM employees WHERE employee_number LIKE 'DEMO-%')`); err != nil {
		return 0, err
	}
	if _, err := r.Pool.Exec(ctx, `
DELETE FROM users
WHERE NOT is_main_admin
  AND (login LIKE 'demo%'
       OR employee_id IN (SELECT id FROM employees WHERE employee_number LIKE 'DEMO-%'))`); err != nil {
		return 0, err
	}
	if _, err := r.Pool.Exec(ctx, `
DELETE FROM employees WHERE employee_number LIKE 'DEMO-%'`); err != nil {
		return 0, err
	}
	return count, nil
}
