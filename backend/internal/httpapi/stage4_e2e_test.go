package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"tabel/internal/auth"
	"tabel/internal/config"
	"tabel/internal/db"
	"tabel/internal/httpapi"
	"tabel/internal/settings"
)

type stage4Mark struct {
	ID                 string    `json:"id"`
	EmployeeID         string    `json:"employee_id"`
	EventType          string    `json:"event_type"`
	TimestampServer    time.Time `json:"timestamp_server"`
	Overtime           bool      `json:"overtime"`
	OutsideOffice      bool      `json:"outside_office"`
	AutoClosedPrevious bool      `json:"auto_closed_previous"`
	ShiftID            *string   `json:"shift_id"`
}

type stage4Schedule struct {
	ID          string `json:"id"`
	EmployeeID  string `json:"employee_id"`
	PatternType string `json:"pattern_type"`
	Workdays    []int  `json:"workdays"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	ValidFrom   string `json:"valid_from"`
	IsActive    bool   `json:"is_active"`
}

func uploadScheduleCSV(t *testing.T, client *http.Client, url, token, filename, content string) (int, []byte) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, buf.Bytes()
}

func TestStage4E2E(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("db.Connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, dsn); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.db"), make([]byte, 32))
	if err != nil {
		t.Fatalf("settings.Open: %v", err)
	}
	defer func() { _ = store.Close() }()

	cfg := config.Config{
		AppEnv:             "development",
		JWTSecret:          "test-secret",
		JWTAccessTTL:       15 * time.Minute,
		JWTRefreshTTL:      time.Hour,
		CORSAllowedOrigins: []string{"*"},
	}
	server := httptest.NewServer(httpapi.NewServer(cfg, store, pool, nil).Routes())
	defer server.Close()
	client := server.Client()

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	adminLogin := "s4_admin_" + suffix
	adminHash, err := auth.HashPassword("Admin12345")
	if err != nil {
		t.Fatal(err)
	}
	var adminID string
	if err := pool.QueryRow(ctx, `
INSERT INTO users (role, login, password_hash, password_algo, registered_at)
VALUES ('admin', $1, $2, 'argon2id', now())
RETURNING id::text`, adminLogin, adminHash).Scan(&adminID); err != nil {
		t.Fatalf("создание тестового администратора: %v", err)
	}
	adminToken := login(t, client, server.URL, adminLogin, "Admin12345")

	employeeNumber := "S4-" + suffix
	status, body := request(t, client, http.MethodPost, server.URL+"/admin/employees", adminToken, map[string]any{
		"employee_number": employeeNumber,
		"full_name":       "Сотрудник Этапа 4",
		"position":        "Тестировщик",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("создание сотрудника: HTTP %d: %s", status, body)
	}
	var employee struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &employee); err != nil {
		t.Fatal(err)
	}

	var qrToken string
	if err := pool.QueryRow(ctx, `
INSERT INTO qr_tokens (employee_id, type, token, payload, signature, status)
VALUES ($1::uuid, 'static', gen_random_uuid(), '{"employee_number":"stage4"}', '', 'active')
RETURNING token::text`, employee.ID).Scan(&qrToken); err != nil {
		t.Fatalf("создание QR-токена: %v", err)
	}

	employeeLogin := "s4_employee_" + suffix
	employeeHash, err := auth.HashPassword("Pass12345")
	if err != nil {
		t.Fatal(err)
	}
	var userID string
	if err := pool.QueryRow(ctx, `
INSERT INTO users (role, login, password_hash, password_algo, employee_id, registered_at)
VALUES ('employee', $1, $2, 'argon2id', $3::uuid, now())
RETURNING id::text`, employeeLogin, employeeHash, employee.ID).Scan(&userID); err != nil {
		t.Fatalf("создание пользователя сотрудника: %v", err)
	}

	var officeID string
	t.Cleanup(func() {
		bg := context.Background()
		if officeID != "" {
			_, _ = pool.Exec(bg, `DELETE FROM offices WHERE id = $1::uuid`, officeID)
		}
		_, _ = pool.Exec(bg, `DELETE FROM notifications WHERE target_user_id = $1::uuid`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM mark_idempotency WHERE employee_id = $1::uuid`, employee.ID)
		_, _ = pool.Exec(bg, `DELETE FROM marks WHERE employee_id = $1::uuid`, employee.ID)
		_, _ = pool.Exec(bg, `DELETE FROM shifts WHERE employee_id = $1::uuid`, employee.ID)
		_, _ = pool.Exec(bg, `DELETE FROM schedules WHERE employee_id = $1::uuid`, employee.ID)
		_, _ = pool.Exec(bg, `DELETE FROM qr_tokens WHERE employee_id = $1::uuid`, employee.ID)
		_, _ = pool.Exec(bg, `DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE employee_id = $1::uuid)`, employee.ID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE employee_id = $1::uuid`, employee.ID)
		_, _ = pool.Exec(bg, `DELETE FROM sessions WHERE user_id = $1::uuid`, adminID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id = $1::uuid`, adminID)
		_, _ = pool.Exec(bg, `DELETE FROM employees WHERE id = $1::uuid`, employee.ID)
	})

	employeeToken := login(t, client, server.URL, employeeLogin, "Pass12345")
	novosibirsk := time.FixedZone("NOV", 7*60*60)
	today := time.Now().In(novosibirsk).Format("2006-01-02")

	csv := fmt.Sprintf("employee_number,pattern_type,workdays,start_time,end_time,valid_from\n%s,custom,1-7,00:00,00:00,%s\n", employeeNumber, today)
	status, body = uploadScheduleCSV(t, client, server.URL+"/admin/schedules/import?mode=preview", adminToken, "schedule.csv", csv)
	if status != http.StatusOK {
		t.Fatalf("preview графика: HTTP %d: %s", status, body)
	}
	var report importReportResponse
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if report.RowsOK != 1 || report.RowsError != 0 {
		t.Fatalf("неожиданный отчёт preview: %+v", report)
	}

	status, body = uploadScheduleCSV(t, client, server.URL+"/admin/schedules/import?mode=commit", adminToken, "schedule.csv", csv)
	if status != http.StatusOK {
		t.Fatalf("commit графика: HTTP %d: %s", status, body)
	}
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if report.RowsOK != 1 || report.RowsError != 0 {
		t.Fatalf("неожиданный отчёт commit: %+v", report)
	}

	status, body = request(t, client, http.MethodGet, server.URL+"/schedules", employeeToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /schedules: HTTP %d: %s", status, body)
	}
	var schedule stage4Schedule
	if err := json.Unmarshal(body, &schedule); err != nil {
		t.Fatal(err)
	}
	if !schedule.IsActive || schedule.PatternType != "custom" || len(schedule.Workdays) != 7 {
		t.Fatalf("неожиданный график: %+v", schedule)
	}

	status, body = request(t, client, http.MethodGet, server.URL+"/admin/schedules?employee_id="+employee.ID, adminToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /admin/schedules: HTTP %d: %s", status, body)
	}

	firstMark := createStage4Mark(t, client, server.URL, employeeToken, qrToken, "", nil)
	if firstMark.EventType != "check_in" || firstMark.ShiftID == nil {
		t.Fatalf("ожидался check_in, получено %+v", firstMark)
	}
	oldShift := *firstMark.ShiftID

	aged := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := pool.Exec(ctx, `UPDATE marks SET timestamp_server = $1 WHERE id = $2::uuid`, aged, firstMark.ID); err != nil {
		t.Fatalf("состаривание отметки: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE shifts SET started_at = $1 WHERE id = $2::uuid`, aged, oldShift); err != nil {
		t.Fatalf("состаривание смены: %v", err)
	}

	secondMark := createStage4Mark(t, client, server.URL, employeeToken, qrToken, "check_in", nil)
	if secondMark.EventType != "check_in" || secondMark.ShiftID == nil {
		t.Fatalf("ожидался новый check_in, получено %+v", secondMark)
	}
	if *secondMark.ShiftID == oldShift {
		t.Fatal("новая отметка прихода должна открыть новую смену")
	}

	var oldStatus string
	var oldEndedAt time.Time
	var syntheticID string
	if err := pool.QueryRow(ctx, `
SELECT status, ended_at, check_out_mark_id::text FROM shifts WHERE id = $1::uuid`, oldShift).
		Scan(&oldStatus, &oldEndedAt, &syntheticID); err != nil {
		t.Fatalf("чтение авто-закрытой смены: %v", err)
	}
	if oldStatus != "auto_closed" || oldEndedAt.IsZero() {
		t.Fatalf("смена не закрыта автоматически: status=%s ended=%s", oldStatus, oldEndedAt)
	}
	var syntheticEvent string
	var syntheticAuto bool
	if err := pool.QueryRow(ctx, `
SELECT event_type, auto_closed_previous FROM marks WHERE id = $1::uuid`, syntheticID).
		Scan(&syntheticEvent, &syntheticAuto); err != nil {
		t.Fatalf("чтение авто-отметки: %v", err)
	}
	if syntheticEvent != "check_out" || !syntheticAuto {
		t.Fatalf("неожиданная авто-отметка: %s/%v", syntheticEvent, syntheticAuto)
	}
	var notifications int
	if err := pool.QueryRow(ctx, `
SELECT count(*) FROM notifications WHERE type = 'shift.auto_closed' AND target_user_id = $1::uuid`, userID).
		Scan(&notifications); err != nil {
		t.Fatalf("чтение уведомлений: %v", err)
	}
	if notifications == 0 {
		t.Fatal("не создано уведомление об авто-закрытии смены")
	}

	status, body = request(t, client, http.MethodGet, server.URL+"/admin/marks?employee_id="+employee.ID+"&auto_closed_previous=true", adminToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /admin/marks: HTTP %d: %s", status, body)
	}
	var adminMarks []stage4Mark
	if err := json.Unmarshal(body, &adminMarks); err != nil {
		t.Fatal(err)
	}
	if len(adminMarks) != 1 || !adminMarks[0].AutoClosedPrevious || adminMarks[0].EventType != "check_out" {
		t.Fatalf("неожиданные авто-закрытые отметки: %+v", adminMarks)
	}

	checkOut := createStage4Mark(t, client, server.URL, employeeToken, qrToken, "", nil)
	if checkOut.EventType != "check_out" || checkOut.ShiftID == nil || *checkOut.ShiftID != *secondMark.ShiftID {
		t.Fatalf("ожидался check_out текущей смены, получено %+v", checkOut)
	}

	status, body = request(t, client, http.MethodPost, server.URL+"/mark", employeeToken, map[string]any{
		"qr_token":    qrToken,
		"event_type":  "check_out",
		"client_type": "mobile",
	}, map[string]string{"Idempotency-Key": uuid.NewString()})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("check_out без смены: HTTP %d: %s", status, body)
	}

	localNow := time.Now().In(novosibirsk)
	narrowStart := localNow.Add(2 * time.Hour)
	narrowEnd := localNow.Add(3 * time.Hour)
	narrowCSV := fmt.Sprintf("employee_number,pattern_type,workdays,start_time,end_time,valid_from\n%s,custom,1-7,%s,%s,%s\n",
		employeeNumber, narrowStart.Format("15:04"), narrowEnd.Format("15:04"), today)
	status, body = uploadScheduleCSV(t, client, server.URL+"/admin/schedules/import?mode=commit", adminToken, "narrow.csv", narrowCSV)
	if status != http.StatusOK {
		t.Fatalf("commit узкого графика: HTTP %d: %s", status, body)
	}

	farLat, farLon := 55.0, 37.0
	overtimeMark := createStage4Mark(t, client, server.URL, employeeToken, qrToken, "", map[string]any{
		"lat": farLat,
		"lon": farLon,
	})
	if !overtimeMark.Overtime {
		t.Fatalf("ожидалась переработка вне графика: %+v", overtimeMark)
	}
	if overtimeMark.OutsideOffice {
		t.Fatal("без офисов отметка не должна считаться вне офиса")
	}
	createStage4Mark(t, client, server.URL, employeeToken, qrToken, "", nil)

	status, body = request(t, client, http.MethodPost, server.URL+"/admin/offices", adminToken, map[string]any{
		"name":     "Офис " + suffix,
		"lat":      55.75,
		"lon":      37.61,
		"radius_m": 500,
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("создание офиса: HTTP %d: %s", status, body)
	}
	var office struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &office); err != nil {
		t.Fatal(err)
	}
	officeID = office.ID

	farOfficeMark := createStage4Mark(t, client, server.URL, employeeToken, qrToken, "", map[string]any{
		"lat": 55.0,
		"lon": 37.0,
	})
	if !farOfficeMark.OutsideOffice {
		t.Fatalf("ожидалась отметка вне офиса: %+v", farOfficeMark)
	}
	createStage4Mark(t, client, server.URL, employeeToken, qrToken, "", nil)

	nearOfficeMark := createStage4Mark(t, client, server.URL, employeeToken, qrToken, "", map[string]any{
		"lat": 55.7501,
		"lon": 37.6101,
	})
	if nearOfficeMark.OutsideOffice {
		t.Fatalf("отметка рядом с офисом должна быть внутри: %+v", nearOfficeMark)
	}
	createStage4Mark(t, client, server.URL, employeeToken, qrToken, "", nil)

	overlapCSV := fmt.Sprintf("employee_number,valid_from,valid_to\n%s,2026-01-01,2026-06-30\n%s,2026-06-30,2026-12-31\n", employeeNumber, employeeNumber)
	status, body = uploadScheduleCSV(t, client, server.URL+"/admin/schedules/import?mode=preview", adminToken, "overlap.csv", overlapCSV)
	if status != http.StatusOK {
		t.Fatalf("preview пересечения: HTTP %d: %s", status, body)
	}
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if report.RowsError != 1 || report.RowsOK != 1 {
		t.Fatalf("ожидалась одна ошибочная строка: %+v", report)
	}
	if report.Rows[1].Reason == nil || !strings.Contains(*report.Rows[1].Reason, "пересекается") {
		t.Fatalf("неожиданная причина: %+v", report.Rows[1])
	}
}

func createStage4Mark(t *testing.T, client *http.Client, baseURL, token, qrToken, eventType string, extra map[string]any) stage4Mark {
	t.Helper()
	payload := map[string]any{
		"qr_token":    qrToken,
		"client_type": "mobile",
	}
	if eventType != "" {
		payload["event_type"] = eventType
	}
	for key, value := range extra {
		payload[key] = value
	}
	status, body := request(t, client, http.MethodPost, baseURL+"/mark", token, payload, map[string]string{
		"Idempotency-Key": uuid.NewString(),
	})
	if status != http.StatusCreated {
		t.Fatalf("создание отметки: HTTP %d: %s", status, body)
	}
	var mark stage4Mark
	if err := json.Unmarshal(body, &mark); err != nil {
		t.Fatal(err)
	}
	return mark
}
