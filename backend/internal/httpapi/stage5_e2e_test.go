package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func oauthToken(t *testing.T, client *http.Client, baseURL, clientID, secret string) (int, []byte) {
	t.Helper()
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", secret)
	resp, err := client.PostForm(baseURL+"/oauth/token", form)
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

func TestStage5E2E(t *testing.T) {
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
	cfg := config.Config{AppEnv: "development", JWTSecret: "test-secret", JWTAccessTTL: 15 * time.Minute, JWTRefreshTTL: time.Hour, CORSAllowedOrigins: []string{"*"}}
	server := httptest.NewServer(httpapi.NewServer(cfg, store, pool, nil).Routes())
	defer server.Close()
	client := server.Client()

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	adminLogin := "s5_admin_" + suffix
	adminHash, err := auth.HashPassword("Admin12345")
	if err != nil {
		t.Fatal(err)
	}
	var adminID string
	if err := pool.QueryRow(ctx, `
INSERT INTO users (role, login, password_hash, password_algo, registered_at)
VALUES ('admin', $1, $2, 'argon2id', now())
RETURNING id::text`, adminLogin, adminHash).Scan(&adminID); err != nil {
		t.Fatalf("admin: %v", err)
	}
	adminToken := login(t, client, server.URL, adminLogin, "Admin12345")

	employeeNumber := "S5-" + suffix
	status, body := request(t, client, http.MethodPost, server.URL+"/admin/employees", adminToken, map[string]any{
		"employee_number": employeeNumber,
		"full_name":       "Сотрудник Этапа 5",
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

	var clientUUID string
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM ip_whitelist WHERE oauth_client_id = $1::uuid`, clientUUID)
		_, _ = pool.Exec(bg, `DELETE FROM oauth_access_tokens WHERE client_id IN (SELECT client_id FROM oauth_clients WHERE id = $1::uuid)`, clientUUID)
		_, _ = pool.Exec(bg, `DELETE FROM oauth_clients WHERE id = $1::uuid`, clientUUID)
		_, _ = pool.Exec(bg, `DELETE FROM offline_imports WHERE uploaded_by = $1::uuid`, adminID)
		_, _ = pool.Exec(bg, `DELETE FROM mark_idempotency WHERE employee_id = $1::uuid`, employee.ID)
		_, _ = pool.Exec(bg, `DELETE FROM marks WHERE employee_id = $1::uuid`, employee.ID)
		_, _ = pool.Exec(bg, `DELETE FROM shifts WHERE employee_id = $1::uuid`, employee.ID)
		_, _ = pool.Exec(bg, `DELETE FROM export_acks WHERE mark_id NOT IN (SELECT id FROM marks)`)
		_, _ = pool.Exec(bg, `DELETE FROM sessions WHERE user_id = $1::uuid`, adminID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id = $1::uuid`, adminID)
		_, _ = pool.Exec(bg, `DELETE FROM employees WHERE id = $1::uuid`, employee.ID)
	})

	status, body = request(t, client, http.MethodPost, server.URL+"/admin/oauth/clients", adminToken, map[string]any{"name": "Интеграция " + suffix}, nil)
	if status != http.StatusCreated {
		t.Fatalf("создание OAuth-клиента: HTTP %d: %s", status, body)
	}
	var created struct {
		ID           string `json:"id"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	clientUUID = created.ID
	if created.ClientID == "" || created.ClientSecret == "" {
		t.Fatalf("клиент без client_id/secret: %s", body)
	}

	if code, _ := oauthToken(t, client, server.URL, created.ClientID, "wrong-secret"); code != http.StatusUnauthorized {
		t.Fatalf("неверный секрет: ожидали 401, получили %d", code)
	}
	code, tokenBody := oauthToken(t, client, server.URL, created.ClientID, created.ClientSecret)
	if code != http.StatusOK {
		t.Fatalf("OAuth-токен: HTTP %d: %s", code, tokenBody)
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(tokenBody, &tokenResp); err != nil {
		t.Fatal(err)
	}
	if tokenResp.AccessToken == "" || tokenResp.TokenType != "Bearer" {
		t.Fatalf("неожиданный ответ токена: %s", tokenBody)
	}

	novosibirsk := time.FixedZone("NOV", 7*60*60)
	ts := time.Now().In(novosibirsk).Format(time.RFC3339)
	csv := fmt.Sprintf("employee_number,event_type,timestamp_server,idempotency_key,client_type\n%s,check_in,%s,%s,mobile\n", employeeNumber, ts, uuid.NewString())
	status, body = uploadScheduleCSV(t, client, server.URL+"/admin/offline-imports", adminToken, "offline.csv", csv)
	if status != http.StatusOK {
		t.Fatalf("офлайн-импорт: HTTP %d: %s", status, body)
	}
	var report importReportResponse
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if report.RowsTotal != 1 || report.RowsOK != 1 || report.RowsError != 0 {
		t.Fatalf("неожиданный отчёт импорта: %+v raw=%s", report, body)
	}

	status, body = request(t, client, http.MethodGet, server.URL+"/export/marks?limit=100", tokenResp.AccessToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("export marks: HTTP %d: %s", status, body)
	}
	var export struct {
		Items []struct {
			MarkID         string `json:"mark_id"`
			EmployeeNumber string `json:"employee_number"`
		} `json:"items"`
		NextCursor *string `json:"next_cursor"`
	}
	if err := json.Unmarshal(body, &export); err != nil {
		t.Fatal(err)
	}
	if len(export.Items) == 0 {
		t.Fatalf("экспорт вернул пустой список: %s", body)
	}
	var exportedMarkID string
	for _, item := range export.Items {
		if item.EmployeeNumber == employeeNumber {
			exportedMarkID = item.MarkID
		}
	}
	if exportedMarkID == "" {
		t.Fatalf("в экспорте нет отметки сотрудника %s: %s", employeeNumber, body)
	}

	status, body = request(t, client, http.MethodPost, server.URL+"/export/ack", tokenResp.AccessToken, map[string]any{
		"mark_ids": []string{exportedMarkID},
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("export ack: HTTP %d: %s", status, body)
	}
	var ack struct {
		Acknowledged int `json:"acknowledged"`
	}
	if err := json.Unmarshal(body, &ack); err != nil {
		t.Fatal(err)
	}
	if ack.Acknowledged != 1 {
		t.Fatalf("acknowledged=%d, ожидали 1", ack.Acknowledged)
	}

	status, body = request(t, client, http.MethodPost, server.URL+"/admin/oauth/clients/"+created.ID+"/whitelist", adminToken, map[string]any{"cidr": "10.0.0.0/8"}, nil)
	if status != http.StatusOK {
		t.Fatalf("добавление whitelist: HTTP %d: %s", status, body)
	}
	if code, _ := oauthToken(t, client, server.URL, created.ClientID, created.ClientSecret); code != http.StatusForbidden {
		t.Fatalf("IP вне whitelist: ожидали 403, получили %d", code)
	}

	status, body = request(t, client, http.MethodGet, server.URL+"/admin/oauth/clients", adminToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("список клиентов: HTTP %d: %s", status, body)
	}
}
