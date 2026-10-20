package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
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

func multipartFile(t *testing.T, fields map[string]string, fieldName, filename string, content []byte) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("field %s: %v", key, err)
		}
	}
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return writer.FormDataContentType(), buf.Bytes()
}

func TestStage15E2E(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}
	t.Setenv("MOBILE_BUILDS_DIR", t.TempDir())
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
	cfg := config.Config{AppEnv: "development", JWTSecret: "test-secret", JWTAccessTTL: 15 * time.Minute, JWTRefreshTTL: time.Hour, CORSAllowedOrigins: []string{"*"}}
	server := httptest.NewServer(httpapi.NewServer(cfg, store, pool, nil).Routes())
	defer server.Close()
	client := server.Client()

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	adminLogin := "s15_admin_" + suffix
	adminHash, _ := auth.HashPassword("Admin12345")
	if _, err := pool.Exec(ctx, `
INSERT INTO users (role, login, password_hash, password_algo, registered_at)
VALUES ('admin', $1, $2, 'argon2id', now())`, adminLogin, adminHash); err != nil {
		t.Fatalf("admin: %v", err)
	}
	adminToken := login(t, client, server.URL, adminLogin, "Admin12345")

	positionName := "Должность S15 " + suffix
	status, body := request(t, client, http.MethodPost, server.URL+"/admin/positions", adminToken, map[string]any{
		"name": positionName,
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("создание должности: HTTP %d: %s", status, body)
	}
	var position struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &position); err != nil {
		t.Fatal(err)
	}

	// Создание сотрудника и пользователя с новой ролью через API.
	status, body = request(t, client, http.MethodPost, server.URL+"/admin/employees", adminToken, map[string]any{
		"employee_number": "S15-" + suffix,
		"full_name":       "Сотрудник Этапа 15",
		"position_id":     position.ID,
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

	leadLogin := "s15_lead_" + suffix
	status, body = request(t, client, http.MethodPost, server.URL+"/admin/users", adminToken, map[string]any{
		"login":    leadLogin,
		"password": "Lead12345",
		"role":     "shift_lead",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("создание руководителя смены: HTTP %d: %s", status, body)
	}

	// Неверная роль отклоняется.
	status, _ = request(t, client, http.MethodPost, server.URL+"/admin/users", adminToken, map[string]any{
		"login": "s15_bad_" + suffix, "password": "Bad12345", "role": "superuser",
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("недопустимая роль: ожидали 400, получили %d", status)
	}

	// Руководитель смены имеет доступ к отчётам.
	leadToken := login(t, client, server.URL, leadLogin, "Lead12345")
	for _, path := range []string{"/reports/summary", "/reports/marks", "/reports/shifts"} {
		status, body = request(t, client, http.MethodGet, server.URL+path, leadToken, nil, nil)
		if status != http.StatusOK {
			t.Fatalf("отчёт %s: HTTP %d: %s", path, status, body)
		}
	}
	// CSV-экспорт.
	status, body = request(t, client, http.MethodGet, server.URL+"/reports/summary?format=csv", leadToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("CSV-экспорт: HTTP %d: %s", status, body)
	}
	if !strings.Contains(string(body), "Табельный номер") {
		t.Fatalf("CSV не содержит заголовок: %s", body)
	}

	// Рядовой сотрудник доступа к отчётам не имеет.
	empLogin := "s15_emp_" + suffix
	empHash, _ := auth.HashPassword("Emp12345")
	if _, err := pool.Exec(ctx, `
INSERT INTO users (role, login, password_hash, password_algo, employee_id, registered_at)
VALUES ('employee', $1, $2, 'argon2id', $3::uuid, now())`, empLogin, empHash, employee.ID); err != nil {
		t.Fatalf("employee user: %v", err)
	}
	empToken := login(t, client, server.URL, empLogin, "Emp12345")
	status, _ = request(t, client, http.MethodGet, server.URL+"/reports/summary", empToken, nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("сотрудник и отчёты: ожидали 403, получили %d", status)
	}

	// Импорт сотрудников из CSV.
	csvData := []byte("employee_number,full_name,position\nIMP-" + suffix + ",Импорт Тестовый," + positionName + "\n")
	contentType, payload := multipartFile(t, map[string]string{}, "file", "employees.csv", csvData)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/employees/import?mode=preview", bytes.NewReader(payload))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("preview import: %v", err)
	}
	previewBody, _ := readAll(res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("preview import: HTTP %d: %s", res.StatusCode, previewBody)
	}
	if !strings.Contains(string(previewBody), `"rows_ok":1`) {
		t.Fatalf("preview import rows_ok != 1: %s", previewBody)
	}

	req, _ = http.NewRequest(http.MethodPost, server.URL+"/admin/employees/import?mode=commit", bytes.NewReader(payload))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("commit import: %v", err)
	}
	commitBody, _ := readAll(res)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(commitBody), `"rows_ok":1`) {
		t.Fatalf("commit import: HTTP %d: %s", res.StatusCode, commitBody)
	}

	// Демо-данные.
	status, body = request(t, client, http.MethodPost, server.URL+"/admin/demo/seed", adminToken, map[string]any{}, nil)
	if status != http.StatusOK || !strings.Contains(string(body), `"seeded":10`) {
		t.Fatalf("demo seed: HTTP %d: %s", status, body)
	}
	status, body = request(t, client, http.MethodGet, server.URL+"/admin/employees?query=DEMO&limit=200", adminToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("demo list: HTTP %d: %s", status, body)
	}
	var demoEmployees []struct {
		EmployeeNumber string `json:"employee_number"`
	}
	if err := json.Unmarshal(body, &demoEmployees); err != nil {
		t.Fatal(err)
	}
	demoCount := 0
	for _, item := range demoEmployees {
		if strings.HasPrefix(item.EmployeeNumber, "DEMO-") {
			demoCount++
		}
	}
	if demoCount != 10 {
		t.Fatalf("ожидали 10 демо-сотрудников, получили %d", demoCount)
	}
	status, body = request(t, client, http.MethodPost, server.URL+"/admin/demo/clear", adminToken, map[string]any{}, nil)
	if status != http.StatusOK {
		t.Fatalf("demo clear: HTTP %d: %s", status, body)
	}

	// Мобильные сборки.
	apkType, apkPayload := multipartFile(t, map[string]string{"platform": "android", "version": "1.0.0"}, "file", "tabel.apk", []byte("APKDATA"))
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/admin/mobile-builds", bytes.NewReader(apkPayload))
	req.Header.Set("Content-Type", apkType)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("upload build: %v", err)
	}
	uploadBody, _ := readAll(res)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("upload build: HTTP %d: %s", res.StatusCode, uploadBody)
	}
	var build struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(uploadBody, &build); err != nil {
		t.Fatal(err)
	}
	status, body = request(t, client, http.MethodGet, server.URL+"/mobile-builds", "", nil, nil)
	if status != http.StatusOK || !strings.Contains(string(body), build.ID) {
		t.Fatalf("публичный список сборок: HTTP %d: %s", status, body)
	}
	status, body = request(t, client, http.MethodGet, server.URL+"/mobile-builds/"+build.ID+"/download", "", nil, nil)
	if status != http.StatusOK || string(body) != "APKDATA" {
		t.Fatalf("скачивание сборки: HTTP %d: %s", status, body)
	}
	status, _ = request(t, client, http.MethodDelete, server.URL+"/admin/mobile-builds/"+build.ID, adminToken, nil, nil)
	if status != http.StatusNoContent {
		t.Fatalf("удаление сборки: ожидали 204, получили %d", status)
	}
}

func readAll(res *http.Response) ([]byte, error) {
	defer res.Body.Close()
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(res.Body)
	return buf.Bytes(), err
}
