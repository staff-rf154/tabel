package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"tabel/internal/geo"
	"tabel/internal/repo"
	"tabel/internal/shift"
)

type markCreateRequest struct {
	QRToken         string     `json:"qr_token"`
	Signature       string     `json:"signature"`
	EventType       string     `json:"event_type"`
	ClientType      string     `json:"client_type"`
	TimestampClient *time.Time `json:"timestamp_client"`
	Lat             *float64   `json:"lat"`
	Lon             *float64   `json:"lon"`
	AccuracyM       *float64   `json:"accuracy_m"`
	LocationAgeSec  *int       `json:"location_age_sec"`
	DeviceOS        *string    `json:"device_os"`
	DeviceOSVersion *string    `json:"device_os_version"`
	DeviceModel     *string    `json:"device_model"`
	AppVersion      *string    `json:"app_version"`
	MockLocation    bool       `json:"mock_location"`
}

func (s *Server) handleCreateMark(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())

	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if _, err := uuid.Parse(idempotencyKey); err != nil {
		writeError(w, http.StatusBadRequest, "заголовок Idempotency-Key обязателен и должен быть UUID")
		return
	}

	var req markCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.QRToken = strings.TrimSpace(req.QRToken)
	if _, err := uuid.Parse(req.QRToken); err != nil {
		writeError(w, http.StatusBadRequest, "поле qr_token должно быть UUID")
		return
	}
	req.EventType = strings.TrimSpace(req.EventType)
	if req.EventType != "" && req.EventType != "check_in" && req.EventType != "check_out" {
		writeError(w, http.StatusBadRequest, "event_type должен быть check_in или check_out")
		return
	}
	if req.ClientType != "mobile" && req.ClientType != "web" {
		writeError(w, http.StatusBadRequest, "client_type должен быть mobile или web")
		return
	}
	if req.Lat != nil && (*req.Lat < -90 || *req.Lat > 90) {
		writeError(w, http.StatusBadRequest, "lat вне диапазона [-90, 90]")
		return
	}
	if req.Lon != nil && (*req.Lon < -180 || *req.Lon > 180) {
		writeError(w, http.StatusBadRequest, "lon вне диапазона [-180, 180]")
		return
	}

	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}

	token, err := rp.FindQRToken(r.Context(), req.QRToken)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "QR-токен не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения QR-токена")
		return
	}
	if token.Status != "active" {
		body := "Попытка использования отозванного или истёкшего QR-токена"
		_ = rp.NotifyAdmins(r.Context(), "qr.revoked_usage", body, &body, map[string]any{
			"qr_token_id": token.ID,
			"employee_id": token.EmployeeID,
			"status":      token.Status,
		})
	}
	if code, message := s.validateQRToken(token, req.Signature); code != 0 {
		writeError(w, code, message)
		return
	}

	openShift, err := rp.GetOpenShift(r.Context(), token.EmployeeID)
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "ошибка чтения смены")
		return
	}
	eventType := req.EventType
	if eventType == "" {
		if openShift != nil {
			eventType = "check_out"
		} else {
			eventType = "check_in"
		}
	}
	if eventType == "check_out" && openShift == nil {
		writeError(w, http.StatusUnprocessableEntity, "недопустимый переход: нет открытой смены")
		return
	}

	now := time.Now().UTC()
	evaluation, err := evaluateSchedule(r.Context(), rp, token.EmployeeID, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка определения графика")
		return
	}
	outside, err := outsideOffice(r.Context(), rp, req.Lat, req.Lon)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка проверки геозоны")
		return
	}

	var autoClose *repo.AutoCloseInput
	if eventType == "check_in" && openShift != nil {
		closedAt, err := autoCloseTime(r.Context(), rp, openShift, now)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ошибка определения конца смены")
			return
		}
		autoClose = &repo.AutoCloseInput{At: closedAt, Overtime: openShift.Overtime}
	}

	var operatorID, operatorNumber *string
	if principal.Role == "operator" || principal.Role == "admin" {
		operatorID = &principal.UserID
		operatorNumber = rp.OperatorNumber(r.Context(), principal.UserID)
	}

	mismatch := qrDataMismatch(token.Payload, token)
	mark, created, closure, err := rp.CreateMark(r.Context(), token.EmployeeID, operatorID, operatorNumber, token.Type, repo.MarkInput{
		EventType:       eventType,
		ClientType:      req.ClientType,
		TimestampServer: now,
		TimestampClient: req.TimestampClient,
		Lat:             req.Lat,
		Lon:             req.Lon,
		AccuracyM:       req.AccuracyM,
		LocationAgeSec:  req.LocationAgeSec,
		DeviceOS:        req.DeviceOS,
		DeviceOSVersion: req.DeviceOSVersion,
		DeviceModel:     req.DeviceModel,
		AppVersion:      req.AppVersion,
		MockLocation:    req.MockLocation,
		QRDataMismatch:  mismatch,
		OutsideOffice:   outside,
		Overtime:        !evaluation.Within,
		ScheduleID:      scheduleID(evaluation.Schedule),
	}, idempotencyKey, autoClose)
	switch {
	case errors.Is(err, repo.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, "операция с таким Idempotency-Key уже выполняется")
		return
	case errors.Is(err, repo.ErrInvalidTransition):
		writeError(w, http.StatusUnprocessableEntity, "недопустимый переход: нет открытой смены")
		return
	case errors.Is(err, repo.ErrOpenShiftExists):
		writeError(w, http.StatusConflict, "у сотрудника уже есть открытая смена, повторите запрос")
		return
	case errors.Is(err, repo.ErrEmployeeInactive):
		writeError(w, http.StatusGone, "сотрудник деактивирован")
		return
	case errors.Is(err, repo.ErrNotFound):
		writeError(w, http.StatusBadRequest, "сотрудник не найден")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "не удалось создать отметку")
		return
	}

	details := map[string]any{
		"employee_id":    mark.EmployeeID,
		"event_type":     mark.EventType,
		"created":        created,
		"overtime":       mark.Overtime,
		"outside_office": mark.OutsideOffice,
	}
	if mark.QRDataMismatch {
		body := "Данные QR-токена не совпадают с данными сотрудника"
		_ = rp.NotifyAdmins(r.Context(), "qr.data_mismatch", body, &body, map[string]any{
			"mark_id":     mark.ID,
			"employee_id": mark.EmployeeID,
		})
	}
	if closure != nil {
		details["auto_closed_shift_id"] = closure.ShiftID
		details["auto_closed_mark_id"] = closure.MarkID
		s.audit(r, rp, &principal.UserID, "shift.auto_close", "shift", closure.ShiftID, map[string]any{
			"employee_id": mark.EmployeeID,
			"mark_id":     closure.MarkID,
			"closed_at":   closure.ClosedAt,
		})
	}
	s.audit(r, rp, &principal.UserID, "mark.create", "mark", mark.ID, details)
	code := http.StatusOK
	if created {
		code = http.StatusCreated
	}
	writeJSON(w, code, mark)
}

func evaluateSchedule(ctx context.Context, rp *repo.Repo, employeeID string, ts time.Time) (shift.Evaluation, error) {
	local := ts.In(shift.NovosibirskZone)
	todayDate := local.Format("2006-01-02")
	yesterday := local.AddDate(0, 0, -1)
	yesterdayDate := yesterday.Format("2006-01-02")

	todaySchedule, err := rp.GetActiveScheduleForDate(ctx, employeeID, todayDate)
	if errors.Is(err, repo.ErrNotFound) {
		todaySchedule = shift.DefaultSchedule(employeeID, local)
	} else if err != nil {
		return shift.Evaluation{}, err
	}
	yesterdaySchedule, err := rp.GetActiveScheduleForDate(ctx, employeeID, yesterdayDate)
	if errors.Is(err, repo.ErrNotFound) {
		yesterdaySchedule = shift.DefaultSchedule(employeeID, yesterday)
	} else if err != nil {
		return shift.Evaluation{}, err
	}
	return shift.Evaluate(todaySchedule, yesterdaySchedule, ts)
}

func scheduleID(schedule repo.Schedule) *string {
	if schedule.ID == "" {
		return nil
	}
	return &schedule.ID
}

func autoCloseTime(ctx context.Context, rp *repo.Repo, openShift *repo.Shift, now time.Time) (time.Time, error) {
	schedule := shift.DefaultSchedule(openShift.EmployeeID, openShift.StartedAt.In(shift.NovosibirskZone))
	if openShift.ScheduleID != nil {
		loaded, err := rp.GetScheduleByID(ctx, *openShift.ScheduleID)
		if errors.Is(err, repo.ErrNotFound) {
			// График мог быть удалён вместе с сотрудником; используем график по умолчанию.
		} else if err != nil {
			return time.Time{}, err
		} else {
			schedule = loaded
		}
	}
	end, err := shift.WindowEnd(schedule, openShift.StartedAt)
	if err != nil {
		return time.Time{}, err
	}
	if end.After(now) {
		end = now
	}
	return end, nil
}

func outsideOffice(ctx context.Context, rp *repo.Repo, lat, lon *float64) (bool, error) {
	if lat == nil || lon == nil {
		return false, nil
	}
	offices, err := rp.ListOffices(ctx, true)
	if err != nil {
		return false, err
	}
	if len(offices) == 0 {
		return false, nil
	}
	for _, office := range offices {
		if geo.DistanceMeters(*lat, *lon, office.Lat, office.Lon) <= float64(office.RadiusM) {
			return false, nil
		}
	}
	return true, nil
}

func (s *Server) handleListMarks(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}

	var from, to *time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from должен быть датой-временем RFC3339")
			return
		}
		from = &parsed
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("to")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to должен быть датой-временем RFC3339")
			return
		}
		to = &parsed
	}
	if from != nil && to != nil && from.After(*to) {
		writeError(w, http.StatusBadRequest, "from не может быть позже to")
		return
	}

	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "limit должен быть числом")
			return
		}
		limit = parsed
	}

	user, err := rp.FindAuthUserByID(r.Context(), principal.UserID)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "пользователь не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения пользователя")
		return
	}
	if user.EmployeeID == nil {
		writeJSON(w, http.StatusOK, []repo.Mark{})
		return
	}

	marks, err := rp.ListMarksByEmployee(r.Context(), *user.EmployeeID, from, to, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить отметки")
		return
	}
	if marks == nil {
		marks = []repo.Mark{}
	}
	writeJSON(w, http.StatusOK, marks)
}
