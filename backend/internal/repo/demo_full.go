package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type DemoOAuthCredential struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	AccessToken  string `json:"access_token"`
}

type DemoResetStats struct {
	Employees     int                   `json:"employees"`
	Users         int                   `json:"users"`
	Schedules     int                   `json:"schedules"`
	Shifts        int                   `json:"shifts"`
	Marks         int                   `json:"marks"`
	Offices       int                   `json:"offices"`
	Notifications int                   `json:"notifications"`
	Delegations   int                   `json:"delegations"`
	Audit         int                   `json:"audit_records"`
	QRStatic      int                   `json:"qr_static"`
	QRPaper       int                   `json:"qr_paper"`
	OAuth         []DemoOAuthCredential `json:"oauth_clients"`
}

type demoOffice struct {
	Name string
	Lat  float64
	Lon  float64
}

type demoSchedule struct {
	Pattern   string
	StartTime string
	EndTime   string
	Workdays  []int32
}

type demoPosition struct {
	Name       string
	Department string
}

var demoOffices = []demoOffice{
	{"Главный офис", 55.030199, 82.920430},
	{"Офис на Красном проспекте", 55.040100, 82.930100},
	{"Складская база", 55.010200, 82.960300},
	{"Отдел продаж", 55.035500, 82.915100},
	{"Производственный участок", 54.990400, 82.930500},
	{"Удалённый офис в Академгородке", 54.850200, 83.090300},
}

var demoPositions = []demoPosition{
	{"Генеральный директор", "Администрация"},
	{"Офис-менеджер", "Администрация"},
	{"Ведущий разработчик", "Отдел разработки"},
	{"Программист", "Отдел разработки"},
	{"Главный бухгалтер", "Бухгалтерия"},
	{"Бухгалтер", "Бухгалтерия"},
	{"Специалист по кадрам", "Отдел кадров"},
	{"Кладовщик", "Склад"},
	{"Водитель", "Склад"},
	{"Менеджер по продажам", "Отдел продаж"},
}

var demoDepartments = []string{"Администрация", "Отдел разработки", "Бухгалтерия", "Отдел кадров", "Склад", "Отдел продаж"}

var demoSchedules = []demoSchedule{
	{"5/2", "09:00", "18:00", []int32{1, 2, 3, 4, 5}},
	{"2/2", "08:00", "20:00", []int32{1, 2}},
	{"4/2", "08:00", "17:00", []int32{1, 2, 3, 4}},
	{"1/2", "08:00", "20:00", []int32{1}},
}

var demoMaleLast = []string{
	"Иванов", "Петров", "Смирнов", "Кузнецов", "Соколов", "Попов", "Лебедев", "Козлов", "Новиков", "Морозов",
	"Волков", "Соловьёв", "Васильев", "Зайцев", "Павлов", "Семёнов", "Голубев", "Виноградов", "Богданов", "Воробьёв",
}
var demoMaleFirst = []string{
	"Иван", "Пётр", "Алексей", "Дмитрий", "Сергей", "Андрей", "Михаил", "Николай", "Владимир", "Егор",
	"Артём", "Максим", "Роман", "Павел", "Кирилл", "Денис", "Антон", "Илья", "Григорий", "Тимур",
}
var demoMaleMiddle = []string{
	"Иванович", "Петрович", "Алексеевич", "Дмитриевич", "Сергеевич", "Андреевич", "Михайлович", "Николаевич", "Владимирович", "Егорович",
	"Артёмович", "Максимович", "Романович", "Павлович", "Кириллович", "Денисович", "Антонович", "Ильич", "Григорьевич", "Тимурович",
}
var demoFemaleLast = []string{
	"Иванова", "Петрова", "Смирнова", "Кузнецова", "Соколова", "Попова", "Лебедева", "Козлова", "Новикова", "Морозова",
	"Волкова", "Соловьёва", "Васильева", "Зайцева", "Павлова", "Семёнова", "Голубева", "Виноградова", "Богданова", "Воробьёва",
}
var demoFemaleFirst = []string{
	"Анна", "Мария", "Елена", "Ольга", "Татьяна", "Наталья", "Ирина", "Светлана", "Юлия", "Екатерина",
	"Алина", "Дарья", "Ксения", "Виктория", "Полина", "Анастасия", "Вероника", "София", "Валентина", "Галина",
}
var demoFemaleMiddle = []string{
	"Ивановна", "Петровна", "Алексеевна", "Дмитриевна", "Сергеевна", "Андреевна", "Михайловна", "Николаевна", "Владимировна", "Егоровна",
	"Артёмовна", "Максимовна", "Романовна", "Павловна", "Кирилловна", "Денисовна", "Антоновна", "Ильинична", "Григорьевна", "Тимуровна",
}

func demoPerson(index int) (string, string, string) {
	pair := index / 2
	if index%2 == 0 {
		return demoMaleLast[pair%len(demoMaleLast)],
			demoMaleFirst[(pair/20+pair)%len(demoMaleFirst)],
			demoMaleMiddle[pair%len(demoMaleMiddle)]
	}
	return demoFemaleLast[pair%len(demoFemaleLast)],
		demoFemaleFirst[(pair/20+pair)%len(demoFemaleFirst)],
		demoFemaleMiddle[pair%len(demoFemaleMiddle)]
}

func demoRole(index int) string {
	switch index {
	case 0, 1, 2:
		return "operator"
	case 3:
		return "shift_lead"
	case 4:
		return "director"
	case 5:
		return "hr"
	default:
		return "employee"
	}
}

func demoLogin(fullName string, used map[string]bool) string {
	base, candidates := employeeLoginCandidates(fullName)
	for _, candidate := range candidates {
		key := strings.ToLower(candidate)
		if !used[key] {
			used[key] = true
			return candidate
		}
	}
	for suffix := 2; suffix < 10000; suffix++ {
		candidate := fmt.Sprintf("%s%d", base, suffix)
		key := strings.ToLower(candidate)
		if !used[key] {
			used[key] = true
			return candidate
		}
	}
	used[strings.ToLower(base)] = true
	return base
}

// ResetDemo полностью заменяет демонстрационные данные: сотрудников, учётные
// записи, графики, смены, отметки, QR, офисы, уведомления, делегирования,
// аудит и OAuth-клиентов. Главный администратор сохраняется, но получает новый
// хэш пароля. Текущая сессия главного администратора не отзывается.
func (r *Repo) ResetDemo(ctx context.Context, passwordHash string) (DemoResetStats, error) {
	now := time.Now().In(demoZone)
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, demoZone)
	for offset := -3; offset <= 0; offset++ {
		if err := r.EnsureMarksPartition(ctx, firstOfMonth.AddDate(0, offset, 0).Format("2006-01-02")); err != nil {
			return DemoResetStats{}, err
		}
	}

	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return DemoResetStats{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cleanup := []string{
		`DELETE FROM mark_idempotency`,
		`DELETE FROM marks`,
		`DELETE FROM shifts`,
		`DELETE FROM qr_tokens`,
		`DELETE FROM schedules`,
		`DELETE FROM notifications`,
		`DELETE FROM delegations`,
		`DELETE FROM audit_log`,
		`DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE NOT is_main_admin)`,
		`DELETE FROM users WHERE NOT is_main_admin`,
		`DELETE FROM employees`,
		`DELETE FROM departments`,
		`DELETE FROM positions`,
		`DELETE FROM offices`,
		`DELETE FROM oauth_access_tokens`,
		`DELETE FROM oauth_clients`,
		`DELETE FROM schedule_templates`,
	}
	for _, statement := range cleanup {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return DemoResetStats{}, err
		}
	}

	var adminID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM users WHERE is_main_admin ORDER BY created_at LIMIT 1`).Scan(&adminID); err != nil {
		return DemoResetStats{}, fmt.Errorf("главный администратор не найден: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE users
SET password_hash = $2, password_algo = 'argon2id', failed_attempts = 0,
    is_locked = false, auth_blocked = false, updated_at = now()
WHERE id = $1::uuid`, adminID, passwordHash); err != nil {
		return DemoResetStats{}, err
	}

	deptIDs := make(map[string]string, len(demoDepartments))
	for _, name := range demoDepartments {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO departments (name) VALUES ($1) RETURNING id::text`, name).Scan(&id); err != nil {
			return DemoResetStats{}, err
		}
		deptIDs[name] = id
	}
	posIDs := make(map[string]string, len(demoPositions))
	for _, position := range demoPositions {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO positions (name) VALUES ($1) RETURNING id::text`, position.Name).Scan(&id); err != nil {
			return DemoResetStats{}, err
		}
		posIDs[position.Name] = id
	}

	for _, schedule := range demoSchedules {
		if _, err := tx.Exec(ctx, `
INSERT INTO schedule_templates (name, pattern_type, workdays, start_time, end_time, created_by)
VALUES ($1, $2, $3::smallint[], $4::time, $5::time, $6::uuid)`,
			schedule.Pattern+" · "+schedule.StartTime+"–"+schedule.EndTime,
			schedule.Pattern, schedule.Workdays, schedule.StartTime, schedule.EndTime, adminID); err != nil {
			return DemoResetStats{}, err
		}
	}

	for _, office := range demoOffices {
		if _, err := tx.Exec(ctx, `INSERT INTO offices (name, lat, lon, radius_m) VALUES ($1, $2, $3, 300)`,
			office.Name, office.Lat, office.Lon); err != nil {
			return DemoResetStats{}, err
		}
	}
	stats := DemoResetStats{Offices: len(demoOffices), OAuth: []DemoOAuthCredential{}}

	scheduleStart := firstOfMonth.AddDate(0, -3, 0)
	scheduleStartText := scheduleStart.Format("2006-01-02")
	userIDs := make([]string, 0, 100)
	employeeIDs := make([]string, 0, 100)
	type employeeSchedule struct {
		EmployeeID string
		ScheduleID string
		Index      int
	}
	employeeSchedules := make([]employeeSchedule, 0, 100)
	usedLogins := make(map[string]bool, 100)

	for i := 0; i < 100; i++ {
		last, first, middle := demoPerson(i)
		fullName := last + " " + first + " " + middle
		number := fmt.Sprintf("н-%08d", i+1)
		position := demoPositions[i%len(demoPositions)]
		email := fmt.Sprintf("user%03d@demo.tabel.local", i+1)
		phone := fmt.Sprintf("+7913%07d", 1000000+i)
		hiredAt := now.AddDate(-(1 + i%5), -(i % 12), -(i % 27)).Format("2006-01-02")

		var employeeID string
		if err := tx.QueryRow(ctx, `
INSERT INTO employees (employee_number, full_name, last_name, first_name, middle_name,
                       email, phone, position, department_id, position_id, is_active, hired_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::uuid, $10::uuid, true, $11::date)
RETURNING id::text`,
			number, fullName, last, first, middle, email, phone, position.Name,
			deptIDs[position.Department], posIDs[position.Name], hiredAt).Scan(&employeeID); err != nil {
			return DemoResetStats{}, err
		}
		employeeIDs = append(employeeIDs, employeeID)

		login := demoLogin(fullName, usedLogins)
		role := demoRole(i)
		var userID string
		if err := tx.QueryRow(ctx, `
INSERT INTO users (employee_id, role, login, password_hash, password_algo, registered_at, last_login_at)
VALUES ($1::uuid, $2, $3, $4, 'argon2id', now(), $5)
RETURNING id::text`, employeeID, role, login, passwordHash, now.Add(-time.Duration(i%72)*time.Hour)).Scan(&userID); err != nil {
			return DemoResetStats{}, err
		}
		userIDs = append(userIDs, userID)

		schedule := demoSchedules[i%len(demoSchedules)]
		var scheduleID string
		if err := tx.QueryRow(ctx, `
INSERT INTO schedules (employee_id, pattern_type, workdays, start_time, end_time, valid_from, is_active, created_by)
VALUES ($1::uuid, $2, $3::smallint[], $4::time, $5::time, $6::date, true, $7::uuid)
RETURNING id::text`, employeeID, schedule.Pattern, schedule.Workdays, schedule.StartTime, schedule.EndTime, scheduleStartText, adminID).Scan(&scheduleID); err != nil {
			return DemoResetStats{}, err
		}
		employeeSchedules = append(employeeSchedules, employeeSchedule{EmployeeID: employeeID, ScheduleID: scheduleID, Index: i})
	}
	stats.Employees = len(employeeIDs)
	stats.Users = len(userIDs) + 1
	stats.Schedules = len(employeeSchedules)

	startDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, demoZone).AddDate(0, 0, -90)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, demoZone)
	shiftRows := make([][]any, 0, 6000)
	markRows := make([][]any, 0, 12000)
	for _, entry := range employeeSchedules {
		schedule := demoSchedules[entry.Index%len(demoSchedules)]
		startClock, _ := time.Parse("15:04", schedule.StartTime)
		endClock, _ := time.Parse("15:04", schedule.EndTime)
		dayIndex := 0
		for day := startDate; !day.After(today); day = day.AddDate(0, 0, 1) {
			if !demoWorkingDay(schedule.Pattern, day, dayIndex, entry.Index) {
				dayIndex++
				continue
			}
			started := time.Date(day.Year(), day.Month(), day.Day(), startClock.Hour(), startClock.Minute(), 0, 0, demoZone)
			plannedEnd := time.Date(day.Year(), day.Month(), day.Day(), endClock.Hour(), endClock.Minute(), 0, 0, demoZone)
			if plannedEnd.Before(started) {
				plannedEnd = plannedEnd.AddDate(0, 0, 1)
			}
			if !plannedEnd.Before(now) {
				dayIndex++
				continue
			}
			inJitter := (entry.Index*37 + dayIndex*17) % 11
			outJitter := (entry.Index*19+dayIndex*23)%31 - 12
			actualIn := started.Add(time.Duration(inJitter) * time.Minute)
			actualOut := plannedEnd.Add(time.Duration(outJitter) * time.Minute)
			overtime := actualOut.After(plannedEnd.Add(5 * time.Minute))
			shiftID := uuid.NewString()
			shiftRows = append(shiftRows, []any{
				shiftID, entry.EmployeeID, entry.ScheduleID, started, actualOut, "closed", overtime, started, actualOut,
			})

			office := demoOffices[(entry.Index+dayIndex)%len(demoOffices)]
			lat := office.Lat + float64((entry.Index+dayIndex)%17)/100000
			lon := office.Lon + float64((entry.Index*3+dayIndex)%17)/100000
			outside := (entry.Index+dayIndex)%19 == 0
			if outside {
				lat += 0.02
				lon -= 0.02
			}
			qrSource := "static"
			if (entry.Index+dayIndex)%7 == 0 {
				qrSource = "paper"
			}
			clientType := "mobile"
			if (entry.Index+dayIndex)%11 == 0 {
				clientType = "web"
			}
			var operatorID any
			var operatorNumber any
			if (entry.Index+dayIndex)%13 == 0 {
				operatorID = adminID
				operatorNumber = "ADMIN"
			}
			for _, mark := range []struct {
				kind string
				at   time.Time
			}{{"check_in", actualIn}, {"check_out", actualOut}} {
				markRows = append(markRows, []any{
					uuid.NewString(), entry.EmployeeID, operatorID, operatorNumber, mark.kind,
					mark.at.Add(-30 * time.Second), mark.at, lat, lon, 8 + (entry.Index+dayIndex)%18,
					"Android", "14", "Pixel 7", "1.0.3", outside, overtime, false,
					qrSource, false, clientType, shiftID, uuid.NewString(), mark.at, nil,
				})
			}
			dayIndex++
		}
	}

	if len(shiftRows) > 0 {
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"shifts"},
			[]string{"id", "employee_id", "schedule_id", "started_at", "ended_at", "status", "overtime", "created_at", "updated_at"},
			pgx.CopyFromRows(shiftRows)); err != nil {
			return DemoResetStats{}, err
		}
	}
	if len(markRows) > 0 {
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"marks"},
			[]string{"id", "employee_id", "operator_id", "operator_number", "event_type", "timestamp_client", "timestamp_server",
				"lat", "lon", "accuracy_m", "device_os", "device_os_version", "device_model", "app_version",
				"outside_office", "overtime", "auto_closed_previous", "qr_source", "qr_data_mismatch", "client_type",
				"shift_id", "idempotency_key", "created_at", "qr_token_id"},
			pgx.CopyFromRows(markRows)); err != nil {
			return DemoResetStats{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
UPDATE shifts s SET check_in_mark_id = m.id
FROM marks m
WHERE m.shift_id = s.id AND m.event_type = 'check_in' AND s.check_in_mark_id IS NULL`); err != nil {
		return DemoResetStats{}, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE shifts s SET check_out_mark_id = m.id
FROM marks m
WHERE m.shift_id = s.id AND m.event_type = 'check_out' AND s.check_out_mark_id IS NULL`); err != nil {
		return DemoResetStats{}, err
	}
	stats.Shifts = len(shiftRows)
	stats.Marks = len(markRows)

	notificationTypes := []string{"delegation.requested", "delegation.approved", "delegation.rejected", "delegation.revoked", "shift.auto_closed"}
	notificationTitles := []string{"Запрос на делегирование", "Делегирование подтверждено", "Делегирование отклонено", "Делегирование отозвано", "Смена закрыта автоматически"}
	for i := 0; i < 30; i++ {
		var target any
		if i%10 != 0 {
			target = userIDs[(i*7)%len(userIDs)]
		}
		notificationType := notificationTypes[i%len(notificationTypes)]
		createdAt := now.Add(-time.Duration(i)*7*time.Hour - 30*time.Minute)
		payload, _ := json.Marshal(map[string]any{"source": "demo", "number": i + 1})
		if _, err := tx.Exec(ctx, `
INSERT INTO notifications (type, title, body, payload, target_user_id, is_read, created_at)
VALUES ($1, $2, $3, $4::jsonb, $5::uuid, $6, $7)`,
			notificationType, notificationTitles[i%len(notificationTitles)],
			"Демонстрационное уведомление для проверки раздела оповещений.",
			string(payload), target, i%4 == 0, createdAt); err != nil {
			return DemoResetStats{}, err
		}
	}
	stats.Notifications = 30

	delegationStatuses := []string{"pending", "approved", "rejected", "revoked", "expired"}
	for i := 0; i < 15; i++ {
		fromID := userIDs[(i*5)%len(userIDs)]
		toID := userIDs[(i*5+13)%len(userIDs)]
		status := delegationStatuses[i%len(delegationStatuses)]
		requestedAt := now.Add(-time.Duration(i*3+1) * 24 * time.Hour)
		validUntil := now.Add(time.Duration(7-i) * 24 * time.Hour)
		if status == "expired" {
			validUntil = now.Add(-24 * time.Hour)
		}
		var decidedBy any
		var decidedAt any
		if status != "pending" {
			decidedBy = adminID
			decidedAt = requestedAt.Add(2 * time.Hour)
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO delegations (from_user_id, to_user_id, status, requested_at, valid_until, decided_by, decided_at)
VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6::uuid, $7)`,
			fromID, toID, status, requestedAt, validUntil, decidedBy, decidedAt); err != nil {
			return DemoResetStats{}, err
		}
	}
	stats.Delegations = 15

	type auditSpec struct {
		Action     string
		EntityType string
		Details    map[string]any
	}
	auditSpecs := []auditSpec{
		{"auth.login", "user", map[string]any{"login": "user001"}},
		{"employee.update", "employee", map[string]any{"employee_number": "н-00000012"}},
		{"schedule.bulk", "schedule", map[string]any{"rows_total": 25, "rows_ok": 25}},
		{"qr.admin_issue", "qr_token", map[string]any{"employee_number": "н-00000007"}},
		{"qr.paper_issue", "qr_token", map[string]any{"employee_number": "н-00000021"}},
		{"admin.profile_update", "settings", map[string]any{"company_name": "ООО «Демо-Компания»"}},
		{"office.update", "office", map[string]any{"name": "Главный офис"}},
		{"user.password_update", "user", map[string]any{"login": "user042"}},
		{"delegation.approve", "delegation", map[string]any{"days": 7}},
		{"mark.create", "mark", map[string]any{"event_type": "check_in"}},
		{"oauth.client_create", "oauth_client", map[string]any{"name": "Демо-интеграция 1С"}},
		{"oauth.client_rotate", "oauth_client", map[string]any{"name": "Демо-интеграция ERP"}},
	}
	for i := 0; i < 400; i++ {
		spec := auditSpecs[i%len(auditSpecs)]
		actor := adminID
		if i%3 != 0 && len(userIDs) > 0 {
			actor = userIDs[(i*11)%len(userIDs)]
		}
		entityID := ""
		if len(employeeIDs) > 0 {
			entityID = employeeIDs[(i*7)%len(employeeIDs)]
		}
		details, _ := json.Marshal(spec.Details)
		createdAt := now.Add(-time.Duration(i) * 5 * time.Hour)
		if _, err := tx.Exec(ctx, `
INSERT INTO audit_log (actor_user_id, action, entity_type, entity_id, details, ip, user_agent, created_at)
VALUES ($1::uuid, $2, $3, $4, $5::jsonb, $6::inet, $7, $8)`,
			actor, spec.Action, spec.EntityType, entityID, string(details),
			fmt.Sprintf("10.54.1.%d", 20+i%200), "Mozilla/5.0 (Windows NT 10.0; Win64; x64)", createdAt); err != nil {
			return DemoResetStats{}, err
		}
	}
	stats.Audit = 400

	oauthSpecs := []struct {
		ClientID     string
		ClientSecret string
		AccessToken  string
		Name         string
	}{
		{"tabel_demo_erp", "demo_erp_secret_2026", "demo_erp_token_2026", "Демо-интеграция ERP"},
		{"tabel_demo_1c", "demo_1c_secret_2026", "demo_1c_token_2026", "Демо-интеграция 1С"},
	}
	for _, spec := range oauthSpecs {
		if _, err := tx.Exec(ctx, `
INSERT INTO oauth_clients (client_id, client_secret_hash, name, secret_rotated_at, next_rotation_at, created_at, updated_at)
VALUES ($1, $2, $3, now() - interval '20 days', now() + interval '33 days', now() - interval '20 days', now())`,
			spec.ClientID, HashOAuthSecret(spec.ClientSecret), spec.Name); err != nil {
			return DemoResetStats{}, err
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO oauth_access_tokens (client_id, token_hash, expires_at, created_at)
VALUES ($1, $2, now() + interval '365 days', now())`,
			spec.ClientID, HashOAuthSecret(spec.AccessToken)); err != nil {
			return DemoResetStats{}, err
		}
		stats.OAuth = append(stats.OAuth, DemoOAuthCredential{
			ClientID: spec.ClientID, ClientSecret: spec.ClientSecret, AccessToken: spec.AccessToken,
		})
	}

	if err := tx.Commit(ctx); err != nil {
		return DemoResetStats{}, err
	}
	return stats, nil
}

func demoWorkingDay(pattern string, date time.Time, dayIndex, employeeIndex int) bool {
	switch pattern {
	case "5/2":
		weekday := int(date.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return weekday >= 1 && weekday <= 5
	case "2/2":
		return (dayIndex+employeeIndex)%4 < 2
	case "4/2":
		return (dayIndex+employeeIndex)%6 < 4
	case "1/2":
		return (dayIndex+employeeIndex)%3 == 0
	default:
		return false
	}
}
