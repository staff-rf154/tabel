package httpapi_test

import (
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

func TestStage6E2E(t *testing.T) {
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
	adminLogin := "s6_admin_" + suffix
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

	employeeNumber := "S6-" + suffix
	status, body := request(t, client, http.MethodPost, server.URL+"/admin/employees", adminToken, map[string]any{
		"employee_number": employeeNumber,
		"full_name":       "Сотрудник Этапа 6",
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

	employeeLogin := "s6_emp_" + suffix
	operatorLogin := "s6_opr_" + suffix
	hash, err := auth.HashPassword("Pass12345")
	if err != nil {
		t.Fatal(err)
	}
	var employeeUserID, operatorUserID string
	if err := pool.QueryRow(ctx, `
INSERT INTO users (role, login, password_hash, password_algo, employee_id, registered_at)
VALUES ('employee', $1, $2, 'argon2id', $3::uuid, now())
RETURNING id::text`, employeeLogin, hash, employee.ID).Scan(&employeeUserID); err != nil {
		t.Fatalf("employee user: %v", err)
	}
	if err := pool.QueryRow(ctx, `
INSERT INTO users (role, login, password_hash, password_algo, registered_at)
VALUES ('operator', $1, $2, 'argon2id', now())
RETURNING id::text`, operatorLogin, hash).Scan(&operatorUserID); err != nil {
		t.Fatalf("operator user: %v", err)
	}

	var delegationID string
	t.Cleanup(func() {
		bg := context.Background()
		if delegationID != "" {
			_, _ = pool.Exec(bg, `DELETE FROM delegations WHERE id = $1::uuid`, delegationID)
		}
		_, _ = pool.Exec(bg, `DELETE FROM notifications WHERE target_user_id IN ($1::uuid, $2::uuid, $3::uuid)`, adminID, employeeUserID, operatorUserID)
		_, _ = pool.Exec(bg, `DELETE FROM sessions WHERE user_id IN ($1::uuid, $2::uuid, $3::uuid)`, adminID, employeeUserID, operatorUserID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id IN ($1::uuid, $2::uuid, $3::uuid)`, adminID, employeeUserID, operatorUserID)
		_, _ = pool.Exec(bg, `DELETE FROM employees WHERE id = $1::uuid`, employee.ID)
	})

	operatorToken := login(t, client, server.URL, operatorLogin, "Pass12345")
	employeeToken := login(t, client, server.URL, employeeLogin, "Pass12345")
	validUntil := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)

	status, body = request(t, client, http.MethodPost, server.URL+"/delegations", operatorToken, map[string]any{
		"to_user_id":  employeeUserID,
		"valid_until": validUntil,
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("создание делегирования: HTTP %d: %s", status, body)
	}
	var delegation struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &delegation); err != nil {
		t.Fatal(err)
	}
	delegationID = delegation.ID
	if delegation.Status != "pending" {
		t.Fatalf("статус=%q, ожидали pending", delegation.Status)
	}

	if code, _ := request(t, client, http.MethodPost, server.URL+"/delegations", operatorToken, map[string]any{
		"to_user_id":  employeeUserID,
		"valid_until": validUntil,
	}, nil); code != http.StatusConflict {
		t.Fatalf("дубликат делегирования: ожидали 409, получили %d", code)
	}

	status, body = request(t, client, http.MethodGet, server.URL+"/delegations", adminToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("список делегирований: HTTP %d: %s", status, body)
	}

	status, body = request(t, client, http.MethodPost, server.URL+"/delegations/"+delegation.ID+"/approve", adminToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("подтверждение делегирования: HTTP %d: %s", status, body)
	}
	var fromRole, toRole string
	if err := pool.QueryRow(ctx, `SELECT role FROM users WHERE id = $1::uuid`, operatorUserID).Scan(&fromRole); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT role FROM users WHERE id = $1::uuid`, employeeUserID).Scan(&toRole); err != nil {
		t.Fatal(err)
	}
	if fromRole != "employee" || toRole != "operator" {
		t.Fatalf("роли после approve: from=%s to=%s", fromRole, toRole)
	}

	status, body = request(t, client, http.MethodGet, server.URL+"/notifications", employeeToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("уведомления: HTTP %d: %s", status, body)
	}
	var notifications []struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		IsRead bool   `json:"is_read"`
	}
	if err := json.Unmarshal(body, &notifications); err != nil {
		t.Fatal(err)
	}
	if len(notifications) == 0 {
		t.Fatalf("нет уведомлений о делегировании: %s", body)
	}
	status, body = request(t, client, http.MethodPost, server.URL+"/notifications/"+notifications[0].ID+"/read", employeeToken, nil, nil)
	if status != http.StatusNoContent {
		t.Fatalf("отметка уведомления: HTTP %d: %s", status, body)
	}

	status, body = request(t, client, http.MethodPost, server.URL+"/delegations/"+delegation.ID+"/revoke", adminToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("отзыв делегирования: HTTP %d: %s", status, body)
	}
	if err := pool.QueryRow(ctx, `SELECT role FROM users WHERE id = $1::uuid`, operatorUserID).Scan(&fromRole); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT role FROM users WHERE id = $1::uuid`, employeeUserID).Scan(&toRole); err != nil {
		t.Fatal(err)
	}
	if fromRole != "operator" || toRole != "employee" {
		t.Fatalf("роли после revoke: from=%s to=%s", fromRole, toRole)
	}

}
