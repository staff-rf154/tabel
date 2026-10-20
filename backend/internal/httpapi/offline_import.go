package httpapi

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"tabel/internal/repo"
)

const (
	maxOfflineImportSize = 20 << 20
	maxOfflineImportRows = 20000
)

var offlineImportHeader = []string{
	"employee_number", "event_type", "timestamp_server", "idempotency_key",
	"timestamp_client", "lat", "lon", "qr_source", "client_type", "operator_number",
}

func (s *Server) handleOfflineImport(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	if err := r.ParseMultipartForm(maxOfflineImportSize); err != nil {
		writeError(w, http.StatusBadRequest, "не удалось прочитать форму")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "файл обязателен (поле file)")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxOfflineImportSize+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "не удалось прочитать файл")
		return
	}
	if len(data) > maxOfflineImportSize {
		writeError(w, http.StatusRequestEntityTooLarge, "файл слишком большой")
		return
	}
	filename := "offline.csv"
	if header != nil && strings.TrimSpace(header.Filename) != "" {
		filename = strings.TrimSpace(header.Filename)
	}

	report := importReport{Rows: []importRow{}}
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		writeError(w, http.StatusBadRequest, "некорректный CSV: "+err.Error())
		return
	}
	if len(records) == 0 {
		writeError(w, http.StatusBadRequest, "файл пуст")
		return
	}
	if len(records)-1 > maxOfflineImportRows {
		writeError(w, http.StatusRequestEntityTooLarge, "слишком много строк")
		return
	}
	index := map[string]int{}
	for i, name := range records[0] {
		index[strings.TrimSpace(strings.ToLower(name))] = i
	}
	for _, required := range []string{"employee_number", "timestamp_server"} {
		if _, ok := index[required]; !ok {
			writeError(w, http.StatusBadRequest, "в CSV нет обязательной колонки "+required)
			return
		}
	}
	get := func(record []string, name string) string {
		pos, ok := index[name]
		if !ok || pos >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[pos])
	}

	for line := 2; line <= len(records); line++ {
		record := records[line-1]
		report.RowsTotal++
		reason, err := s.importOfflineRow(r, rp, principal.UserID, record, get)
		if err != nil {
			report.RowsError++
			message := err.Error()
			if errors.Is(err, repo.ErrNotFound) {
				message = "сотрудник не найден"
			}
			report.Rows = append(report.Rows, importRow{Line: line, Status: "error", Reason: &message})
			continue
		}
		_ = reason
		report.RowsOK++
		report.Rows = append(report.Rows, importRow{Line: line, Status: "ok", Reason: nil})
	}

	status := "ok"
	if report.RowsError > 0 {
		status = "partial"
	}
	if report.RowsOK == 0 {
		status = "error"
	}
	uploadedBy := principal.UserID
	if err := rp.RecordOfflineImport(r.Context(), &uploadedBy, filename, report.RowsTotal, report.RowsOK, report.RowsError, status, report); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сохранить отчёт импорта")
		return
	}
	if report.RowsError > 0 {
		body := "Импорт офлайн-отметок завершён с ошибками"
		_ = rp.NotifyAdmins(r.Context(), "offline_import.errors", body, &body, map[string]any{
			"filename":   filename,
			"rows_total": report.RowsTotal,
			"rows_ok":    report.RowsOK,
			"rows_error": report.RowsError,
		})
	}
	s.audit(r, rp, &principal.UserID, "offline_import", "offline_import", filename, map[string]any{
		"rows_total": report.RowsTotal,
		"rows_ok":    report.RowsOK,
		"rows_error": report.RowsError,
	})
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) importOfflineRow(r *http.Request, rp *repo.Repo, adminID string, record []string, get func([]string, string) string) (string, error) {
	ctx := r.Context()
	employeeNumber := get(record, "employee_number")
	if employeeNumber == "" {
		return "", errors.New("employee_number обязателен")
	}
	employee, err := rp.FindEmployeeByNumber(ctx, employeeNumber)
	if err != nil {
		return "", err
	}

	eventType := get(record, "event_type")
	if eventType != "" && eventType != "check_in" && eventType != "check_out" {
		return "", errors.New("event_type должен быть check_in или check_out")
	}
	rawTS := get(record, "timestamp_server")
	ts, err := time.Parse(time.RFC3339, rawTS)
	if err != nil {
		return "", errors.New("timestamp_server должен быть RFC3339")
	}
	ts = ts.UTC()

	idempotencyKey := get(record, "idempotency_key")
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	if _, err := uuid.Parse(idempotencyKey); err != nil {
		return "", errors.New("idempotency_key должен быть UUID")
	}

	clientType := get(record, "client_type")
	if clientType == "" {
		clientType = "mobile"
	}
	if clientType != "mobile" && clientType != "web" {
		return "", errors.New("client_type должен быть mobile или web")
	}
	qrSource := get(record, "qr_source")
	if qrSource == "" {
		qrSource = "static"
	}
	if qrSource != "static" && qrSource != "paper" {
		return "", errors.New("qr_source должен быть static или paper")
	}

	var lat, lon *float64
	if raw := get(record, "lat"); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || value < -90 || value > 90 {
			return "", errors.New("lat вне диапазона [-90, 90]")
		}
		lat = &value
	}
	if raw := get(record, "lon"); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || value < -180 || value > 180 {
			return "", errors.New("lon вне диапазона [-180, 180]")
		}
		lon = &value
	}
	var timestampClient *time.Time
	if raw := get(record, "timestamp_client"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return "", errors.New("timestamp_client должен быть RFC3339")
		}
		timestampClient = &value
	}

	openShift, err := rp.GetOpenShift(ctx, employee.ID)
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		return "", err
	}
	inferred := eventType
	if inferred == "" {
		if openShift != nil {
			inferred = "check_out"
		} else {
			inferred = "check_in"
		}
	}
	if inferred == "check_out" && openShift == nil {
		return "", errors.New("недопустимый переход: нет открытой смены")
	}

	evaluation, err := evaluateSchedule(ctx, rp, employee.ID, ts)
	if err != nil {
		return "", err
	}
	outside, err := outsideOffice(ctx, rp, lat, lon)
	if err != nil {
		return "", err
	}
	var autoClose *repo.AutoCloseInput
	if inferred == "check_in" && openShift != nil {
		closedAt, err := autoCloseTime(ctx, rp, openShift, ts)
		if err != nil {
			return "", err
		}
		autoClose = &repo.AutoCloseInput{At: closedAt, Overtime: openShift.Overtime}
	}

	var operatorNumber *string
	operatorUser := adminID
	if raw := get(record, "operator_number"); raw != "" {
		operatorNumber = &raw
	} else {
		operatorNumber = rp.OperatorNumber(ctx, adminID)
	}
	_, _, _, err = rp.CreateMark(ctx, employee.ID, &operatorUser, operatorNumber, qrSource, repo.MarkInput{
		EventType:       inferred,
		ClientType:      clientType,
		TimestampServer: ts,
		TimestampClient: timestampClient,
		Lat:             lat,
		Lon:             lon,
		OutsideOffice:   outside,
		Overtime:        !evaluation.Within,
		ScheduleID:      scheduleID(evaluation.Schedule),
	}, idempotencyKey, autoClose)
	if err != nil {
		return "", err
	}
	return "", nil
}
