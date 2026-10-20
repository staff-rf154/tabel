package httpapi_test

import (
	"archive/zip"
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

type qrTokenResponse struct {
	ID         string         `json:"id"`
	EmployeeID string         `json:"employee_id"`
	Type       string         `json:"type"`
	Token      string         `json:"token"`
	Signature  string         `json:"signature"`
	ValidFrom  *string        `json:"valid_from"`
	ValidUntil *string        `json:"valid_until"`
	Status     string         `json:"status"`
	Payload    map[string]any `json:"payload"`
	QRContent  string         `json:"qr_content"`
}

type qrVerificationResponse struct {
	Valid          bool   `json:"valid"`
	TokenStatus    string `json:"token_status"`
	SignatureValid bool   `json:"signature_valid"`
	EmployeeID     string `json:"employee_id"`
	QRDataMismatch bool   `json:"qr_data_mismatch"`
}

type importReportResponse struct {
	RowsTotal int `json:"rows_total"`
	RowsOK    int `json:"rows_ok"`
	RowsError int `json:"rows_error"`
	Rows      []struct {
		Line   int     `json:"line"`
		Status string  `json:"status"`
		Reason *string `json:"reason"`
	} `json:"rows"`
}

func TestQRStage3E2E(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	adminLogin := "qr_admin_" + suffix
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

	employees := []string{}
	t.Cleanup(func() {
		bg := context.Background()
		for _, employeeID := range employees {
			_, _ = pool.Exec(bg, `DELETE FROM mark_idempotency WHERE employee_id = $1::uuid`, employeeID)
			_, _ = pool.Exec(bg, `DELETE FROM marks WHERE employee_id = $1::uuid`, employeeID)
			_, _ = pool.Exec(bg, `DELETE FROM qr_tokens WHERE employee_id = $1::uuid`, employeeID)
			_, _ = pool.Exec(bg, `DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE employee_id = $1::uuid)`, employeeID)
			_, _ = pool.Exec(bg, `DELETE FROM users WHERE employee_id = $1::uuid`, employeeID)
			_, _ = pool.Exec(bg, `DELETE FROM employees WHERE id = $1::uuid`, employeeID)
		}
		_, _ = pool.Exec(bg, `DELETE FROM sessions WHERE user_id = $1::uuid`, adminID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id = $1::uuid`, adminID)
	})

	employeeA, numberA := createEmployee(t, client, server.URL, adminToken, "A", suffix+"a")
	employeeB, numberB := createEmployee(t, client, server.URL, adminToken, "B", suffix+"b")
	employees = append(employees, employeeA, employeeB)

	tokenA := uuid.NewString()
	status, body := request(t, client, http.MethodPost, server.URL+"/admin/qr/static", adminToken, map[string]any{
		"employee_id": employeeA,
		"payload": map[string]any{
			"token":           tokenA,
			"employee_number": numberA,
			"full_name":       "Сотрудник A",
			"position":        "Инженер",
		},
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("загрузка статического QR: HTTP %d: %s", status, body)
	}
	var staticQR qrTokenResponse
	if err := json.Unmarshal(body, &staticQR); err != nil {
		t.Fatal(err)
	}
	if staticQR.Token != tokenA || staticQR.Signature == "" || staticQR.QRContent == "" || staticQR.Status != "active" {
		t.Fatalf("неожиданный статический QR: %+v", staticQR)
	}
	if staticQR.Payload["employee_id"] != employeeA || staticQR.Payload["type"] != "static" {
		t.Fatalf("payload не дополнен данными сотрудника: %+v", staticQR.Payload)
	}

	status, body = request(t, client, http.MethodGet, server.URL+"/qr/"+tokenA, adminToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("проверка QR: HTTP %d: %s", status, body)
	}
	var verification qrVerificationResponse
	if err := json.Unmarshal(body, &verification); err != nil {
		t.Fatal(err)
	}
	if !verification.Valid || !verification.SignatureValid || verification.TokenStatus != "active" || verification.EmployeeID != employeeA || verification.QRDataMismatch {
		t.Fatalf("неожиданный результат проверки: %+v", verification)
	}

	phoneA := "+7999" + suffix[:7]
	status, body = request(t, client, http.MethodPost, server.URL+"/auth/register", "", map[string]any{
		"qr_token":  tokenA,
		"signature": staticQR.Signature,
		"full_name": "Сотрудник A",
		"phone":     phoneA,
		"password":  "Pass12345",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("регистрация: HTTP %d: %s", status, body)
	}
	employeeAToken := login(t, client, server.URL, phoneA, "Pass12345")

	markBody := map[string]any{
		"qr_token":    tokenA,
		"event_type":  "check_in",
		"client_type": "mobile",
	}
	status, body = request(t, client, http.MethodPost, server.URL+"/mark", employeeAToken, markBody, map[string]string{
		"Idempotency-Key": uuid.NewString(),
	})
	if status != http.StatusCreated {
		t.Fatalf("отметка по статическому QR: HTTP %d: %s", status, body)
	}
	var mark struct {
		QRSource       string `json:"qr_source"`
		QRDataMismatch bool   `json:"qr_data_mismatch"`
	}
	if err := json.Unmarshal(body, &mark); err != nil {
		t.Fatal(err)
	}
	if mark.QRSource != "static" || mark.QRDataMismatch {
		t.Fatalf("неожиданная отметка: %+v", mark)
	}

	status, body = request(t, client, http.MethodPatch, server.URL+"/admin/employees/"+employeeA, adminToken, map[string]any{
		"employee_number": numberA,
		"full_name":       "Сотрудник A (обновлён)",
		"phone":           phoneA,
		"position":        "Инженер",
		"is_active":       true,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("обновление сотрудника: HTTP %d: %s", status, body)
	}

	status, body = request(t, client, http.MethodGet, server.URL+"/qr/"+tokenA, adminToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("проверка QR после изменения: HTTP %d: %s", status, body)
	}
	if err := json.Unmarshal(body, &verification); err != nil {
		t.Fatal(err)
	}
	if !verification.QRDataMismatch || !verification.SignatureValid {
		t.Fatalf("ожидалось расхождение данных при валидной подписи: %+v", verification)
	}

	status, body = request(t, client, http.MethodPost, server.URL+"/mark", employeeAToken, map[string]any{
		"qr_token":    tokenA,
		"event_type":  "check_out",
		"client_type": "mobile",
	}, map[string]string{"Idempotency-Key": uuid.NewString()})
	if status != http.StatusCreated {
		t.Fatalf("отметка с расхождением: HTTP %d: %s", status, body)
	}
	if err := json.Unmarshal(body, &mark); err != nil {
		t.Fatal(err)
	}
	if !mark.QRDataMismatch {
		t.Fatalf("отметка должна содержать qr_data_mismatch: %+v", mark)
	}

	status, body = request(t, client, http.MethodPost, server.URL+"/admin/qr/paper", adminToken, map[string]any{
		"employee_id": employeeB,
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("выдача бумажного QR: HTTP %d: %s", status, body)
	}
	var paperQR qrTokenResponse
	if err := json.Unmarshal(body, &paperQR); err != nil {
		t.Fatal(err)
	}
	if paperQR.Type != "paper" || paperQR.ValidFrom == nil || paperQR.ValidUntil == nil || paperQR.Status != "active" {
		t.Fatalf("неожиданный бумажный QR: %+v", paperQR)
	}
	from, _ := time.Parse("2006-01-02", *paperQR.ValidFrom)
	until, _ := time.Parse("2006-01-02", *paperQR.ValidUntil)
	if until.Before(from.AddDate(0, 1, 0)) {
		t.Fatalf("срок действия меньше месяца: %s - %s", *paperQR.ValidFrom, *paperQR.ValidUntil)
	}

	status, body = request(t, client, http.MethodPost, server.URL+"/admin/qr/paper", adminToken, map[string]any{
		"employee_id": employeeB,
	}, nil)
	if status != http.StatusConflict {
		t.Fatalf("повторная выдача бумажного QR: HTTP %d: %s", status, body)
	}

	status, body = request(t, client, http.MethodGet, server.URL+"/admin/qr/paper/"+paperQR.ID+"/pdf", adminToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("PDF бумажного QR: HTTP %d: %s", status, body)
	}
	if !bytes.HasPrefix(body, []byte("%PDF-1.")) {
		t.Fatalf("ответ не является PDF: %q", body[:minInt(16, len(body))])
	}

	paperMarkBody := map[string]any{
		"qr_token":    paperQR.Token,
		"event_type":  "check_in",
		"client_type": "web",
	}
	status, body = request(t, client, http.MethodPost, server.URL+"/mark", adminToken, paperMarkBody, map[string]string{
		"Idempotency-Key": uuid.NewString(),
	})
	if status != http.StatusCreated {
		t.Fatalf("отметка по бумажному QR: HTTP %d: %s", status, body)
	}
	if err := json.Unmarshal(body, &mark); err != nil {
		t.Fatal(err)
	}
	if mark.QRSource != "paper" || mark.QRDataMismatch {
		t.Fatalf("неожиданная отметка по бумажному QR: %+v", mark)
	}

	status, body = request(t, client, http.MethodPost, server.URL+"/admin/qr/paper/"+paperQR.ID+"/revoke", adminToken, nil, nil)
	if status != http.StatusNoContent {
		t.Fatalf("отзыв бумажного QR: HTTP %d: %s", status, body)
	}
	status, body = request(t, client, http.MethodGet, server.URL+"/qr/"+paperQR.Token, adminToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("проверка отозванного QR: HTTP %d: %s", status, body)
	}
	if err := json.Unmarshal(body, &verification); err != nil {
		t.Fatal(err)
	}
	if verification.Valid || verification.TokenStatus != "revoked" {
		t.Fatalf("отозванный токен должен быть невалидным: %+v", verification)
	}

	status, body = request(t, client, http.MethodPost, server.URL+"/admin/qr/paper", adminToken, map[string]any{
		"employee_id": employeeB,
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("повторная выдача бумажного QR после отзыва: HTTP %d: %s", status, body)
	}
	var expiringPaper qrTokenResponse
	if err := json.Unmarshal(body, &expiringPaper); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
UPDATE qr_tokens SET valid_from = current_date - 40, valid_until = current_date - 10
WHERE id = $1::uuid`, expiringPaper.ID); err != nil {
		t.Fatalf("сдвиг срока действия: %v", err)
	}
	status, body = request(t, client, http.MethodPost, server.URL+"/admin/qr/paper", adminToken, map[string]any{
		"employee_id": employeeB,
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("выдача бумажного QR после истечения: HTTP %d: %s", status, body)
	}
	status, body = request(t, client, http.MethodGet, server.URL+"/qr/"+expiringPaper.Token, adminToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("проверка истёкшего QR: HTTP %d: %s", status, body)
	}
	if err := json.Unmarshal(body, &verification); err != nil {
		t.Fatal(err)
	}
	if verification.Valid || verification.TokenStatus != "expired" {
		t.Fatalf("истёкший токен должен быть невалидным: %+v", verification)
	}

	bulkToken := uuid.NewString()
	bulkPayload := []map[string]any{
		{
			"employee_id": employeeB,
			"payload": map[string]any{
				"token":           bulkToken,
				"employee_number": numberB,
				"full_name":       "Сотрудник B",
				"position":        "Инженер",
			},
		},
		{
			"employee_id": uuid.NewString(),
			"payload":     map[string]any{"full_name": "Неизвестный"},
		},
	}
	bulkJSON, err := json.Marshal(bulkPayload)
	if err != nil {
		t.Fatal(err)
	}

	status, body = multipartFileRequest(t, client, server.URL+"/admin/qr/bulk?mode=preview", adminToken, "bulk.json", bulkJSON)
	if status != http.StatusOK {
		t.Fatalf("предпросмотр импорта: HTTP %d: %s", status, body)
	}
	var report importReportResponse
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if report.RowsTotal != 2 || report.RowsOK != 1 || report.RowsError != 1 {
		t.Fatalf("неожиданный отчёт предпросмотра: %+v", report)
	}

	status, body = multipartFileRequest(t, client, server.URL+"/admin/qr/bulk?mode=commit", adminToken, "bulk.json", bulkJSON)
	if status != http.StatusOK {
		t.Fatalf("импорт: HTTP %d: %s", status, body)
	}
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if report.RowsTotal != 2 || report.RowsOK != 1 || report.RowsError != 1 {
		t.Fatalf("неожиданный отчёт импорта: %+v", report)
	}
	status, body = request(t, client, http.MethodGet, server.URL+"/qr/"+bulkToken, adminToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("проверка импортированного QR: HTTP %d: %s", status, body)
	}
	if err := json.Unmarshal(body, &verification); err != nil {
		t.Fatal(err)
	}
	if !verification.Valid || !verification.SignatureValid || verification.QRDataMismatch {
		t.Fatalf("импортированный QR невалиден: %+v", verification)
	}

	zipData := makeZip(t, "employees.json", []byte(`[{"employee_id":"`+employeeA+`","payload":{"full_name":"Сотрудник A (обновлён)","position":"Инженер"}}]`))
	status, body = multipartFileRequest(t, client, server.URL+"/admin/qr/bulk?mode=preview", adminToken, "bulk.zip", zipData)
	if status != http.StatusOK {
		t.Fatalf("предпросмотр zip: HTTP %d: %s", status, body)
	}
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if report.RowsTotal != 1 || report.RowsOK != 1 {
		t.Fatalf("неожиданный отчёт zip: %+v", report)
	}

	status, body = request(t, client, http.MethodGet, server.URL+"/qr/"+uuid.NewString(), adminToken, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("неизвестный QR: HTTP %d: %s", status, body)
	}
}

func createEmployee(t *testing.T, client *http.Client, baseURL, token, suffix, numberSuffix string) (string, string) {
	t.Helper()
	number := "QR-" + numberSuffix
	status, body := request(t, client, http.MethodPost, baseURL+"/admin/employees", token, map[string]any{
		"employee_number": number,
		"full_name":       "Сотрудник " + suffix,
		"phone":           "+7999" + suffix + "00",
		"position":        "Инженер",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("создание сотрудника %s: HTTP %d: %s", suffix, status, body)
	}
	var employee struct {
		ID             string `json:"id"`
		EmployeeNumber string `json:"employee_number"`
	}
	if err := json.Unmarshal(body, &employee); err != nil {
		t.Fatal(err)
	}
	return employee.ID, employee.EmployeeNumber
}

func multipartFileRequest(t *testing.T, client *http.Client, url, token, filename string, data []byte) (int, []byte) {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, &buffer)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, buf.Bytes()
}

func makeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
