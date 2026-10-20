package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
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

// TestRegistrationAndMarkE2E проверяет сквозной сценарий Этапа 2 на реальной БД.
// Запуск: TEST_DATABASE_URL=postgres://... go test ./internal/httpapi -run E2E
func TestRegistrationAndMarkE2E(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("db.Connect: %v", err)
	}
	defer pool.Close()
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
	adminLogin := "e2e_admin_" + suffix
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
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sessions WHERE user_id = $1::uuid`, adminID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1::uuid`, adminID)
	})

	adminToken := login(t, client, server.URL, adminLogin, "Admin12345")

	phone := "+7999" + suffix[:7]
	status, body := request(t, client, http.MethodPost, server.URL+"/admin/employees", adminToken, map[string]any{
		"employee_number": "E2E-" + suffix,
		"full_name":       "Тестовый Сотрудник",
		"phone":           phone,
		"position":        "Тестировщик",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("создание сотрудника: HTTP %d: %s", status, body)
	}
	var employee struct {
		ID             string `json:"id"`
		EmployeeNumber string `json:"employee_number"`
	}
	if err := json.Unmarshal(body, &employee); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM mark_idempotency WHERE employee_id = $1::uuid`, employee.ID)
		_, _ = pool.Exec(bg, `DELETE FROM marks WHERE employee_id = $1::uuid`, employee.ID)
		_, _ = pool.Exec(bg, `DELETE FROM qr_tokens WHERE employee_id = $1::uuid`, employee.ID)
		_, _ = pool.Exec(bg, `DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE employee_id = $1::uuid)`, employee.ID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE employee_id = $1::uuid`, employee.ID)
		_, _ = pool.Exec(bg, `DELETE FROM employees WHERE id = $1::uuid`, employee.ID)
	})

	var qrToken string
	if err := pool.QueryRow(ctx, `
INSERT INTO qr_tokens (employee_id, type, token, payload, signature, status)
VALUES ($1::uuid, 'static', gen_random_uuid(), '{"employee_number":"test"}', '', 'active')
RETURNING token::text`, employee.ID).Scan(&qrToken); err != nil {
		t.Fatalf("создание QR-токена: %v", err)
	}

	status, body = request(t, client, http.MethodPost, server.URL+"/auth/register", "", map[string]any{
		"qr_token":  qrToken,
		"full_name": "Тестовый Сотрудник",
		"phone":     phone,
		"password":  "Pass12345",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("регистрация: HTTP %d: %s", status, body)
	}
	var profile struct {
		ID         string `json:"id"`
		Role       string `json:"role"`
		Login      string `json:"login"`
		EmployeeID string `json:"employee_id"`
	}
	if err := json.Unmarshal(body, &profile); err != nil {
		t.Fatal(err)
	}
	if profile.Role != "employee" || profile.Login != phone || profile.EmployeeID != employee.ID {
		t.Fatalf("неожиданный профиль: %+v", profile)
	}

	employeeToken := login(t, client, server.URL, phone, "Pass12345")

	status, body = request(t, client, http.MethodGet, server.URL+"/me", employeeToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /me: HTTP %d: %s", status, body)
	}

	idempotencyKey := uuid.NewString()
	markBody := map[string]any{
		"qr_token":    qrToken,
		"event_type":  "check_in",
		"client_type": "mobile",
	}
	status, body = request(t, client, http.MethodPost, server.URL+"/mark", employeeToken, markBody, map[string]string{
		"Idempotency-Key": idempotencyKey,
	})
	if status != http.StatusCreated {
		t.Fatalf("создание отметки: HTTP %d: %s", status, body)
	}
	var mark struct {
		ID        string `json:"id"`
		EventType string `json:"event_type"`
		QRSource  string `json:"qr_source"`
	}
	if err := json.Unmarshal(body, &mark); err != nil {
		t.Fatal(err)
	}
	if mark.EventType != "check_in" || mark.QRSource != "static" {
		t.Fatalf("неожиданная отметка: %+v", mark)
	}

	status, body = request(t, client, http.MethodPost, server.URL+"/mark", employeeToken, markBody, map[string]string{
		"Idempotency-Key": idempotencyKey,
	})
	if status != http.StatusOK {
		t.Fatalf("повторная отметка: HTTP %d: %s", status, body)
	}
	var repeated struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &repeated); err != nil {
		t.Fatal(err)
	}
	if repeated.ID != mark.ID {
		t.Fatalf("идемпотентность нарушена: %s != %s", repeated.ID, mark.ID)
	}

	status, body = request(t, client, http.MethodGet, server.URL+"/marks", employeeToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /marks: HTTP %d: %s", status, body)
	}
	var marks []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &marks); err != nil {
		t.Fatal(err)
	}
	if len(marks) != 1 || marks[0].ID != mark.ID {
		t.Fatalf("неожиданный список отметок: %+v", marks)
	}

	status, body = request(t, client, http.MethodGet, server.URL+"/admin/audit?entity_id="+mark.ID, adminToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /admin/audit: HTTP %d: %s", status, body)
	}
	var audit []struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(body, &audit); err != nil {
		t.Fatal(err)
	}
	if len(audit) == 0 || audit[0].Action != "mark.create" {
		t.Fatalf("в аудите нет создания отметки: %+v", audit)
	}
}

func login(t *testing.T, client *http.Client, baseURL, login, password string) string {
	t.Helper()
	status, body := request(t, client, http.MethodPost, baseURL+"/auth/login", "", map[string]any{
		"login":    login,
		"password": password,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("вход %q: HTTP %d: %s", login, status, body)
	}
	var pair struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &pair); err != nil {
		t.Fatal(err)
	}
	if pair.AccessToken == "" {
		t.Fatal("сервер не вернул access_token")
	}
	return pair.AccessToken
}

func request(t *testing.T, client *http.Client, method, url, token string, payload any, headers map[string]string) (int, []byte) {
	t.Helper()
	var reader *bytes.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, buf.Bytes()
}
