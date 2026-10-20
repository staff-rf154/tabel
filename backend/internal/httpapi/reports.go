package httpapi

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tabel/internal/repo"
	"tabel/internal/shift"
)

func (s *Server) reportsRole(next http.HandlerFunc) http.HandlerFunc {
	return s.authenticated(func(w http.ResponseWriter, r *http.Request) {
		p, _ := principalFrom(r.Context())
		if !reportingRoles[p.Role] {
			writeError(w, http.StatusForbidden, "недостаточно прав")
			return
		}
		next(w, r)
	})
}

func reportsMinimumFrom(now time.Time) time.Time {
	local := now.In(shift.NovosibirskZone)
	firstThisMonth := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, shift.NovosibirskZone)
	return firstThisMonth.AddDate(0, -1, 0)
}

func (s *Server) reportRange(w http.ResponseWriter, r *http.Request) (time.Time, time.Time, bool) {
	p, _ := principalFrom(r.Context())
	now := time.Now()
	minFrom := reportsMinimumFrom(now)
	from := minFrom
	to := now
	if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from должен быть RFC3339")
			return from, to, false
		}
		from = parsed
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("to")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to должен быть RFC3339")
			return from, to, false
		}
		to = parsed
	}
	if p.Role != "admin" {
		if from.Before(minFrom) {
			from = minFrom
		}
		if to.After(now) {
			to = now
		}
	}
	if !to.After(from) {
		writeError(w, http.StatusBadRequest, "to должен быть позже from")
		return from, to, false
	}
	return from, to, true
}

func reportFilters(r *http.Request) ([]string, string) {
	var ids []string
	if raw := strings.TrimSpace(r.URL.Query().Get("employee_id")); raw != "" {
		ids = append(ids, raw)
	}
	for _, part := range strings.Split(r.URL.Query().Get("employee_ids"), ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			ids = append(ids, part)
		}
	}
	position := strings.TrimSpace(r.URL.Query().Get("position"))
	return ids, position
}

func (s *Server) handleReportMarks(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	from, to, ok := s.reportRange(w, r)
	if !ok {
		return
	}
	ids, position := reportFilters(r)
	limit := parseLimit(r.URL.Query().Get("limit"), 5000, 50000)
	items, err := rp.ReportMarks(r.Context(), from, to, ids, position, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось построить отчёт")
		return
	}
	if exportCSV(r) {
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{
				item.EmployeeNumber, item.FullName, deref(item.Position), item.EventType,
				item.TimestampServer.In(shift.NovosibirskZone).Format("2006-01-02 15:04:05"),
				item.TimingStatus,
				boolText(item.Overtime), boolText(item.OutsideOffice), boolText(item.AutoClosed),
				deref(item.OperatorNumber),
			})
		}
		writeCSV(w, "marks-report.csv",
			[]string{"Табельный номер", "ФИО", "Должность", "Событие", "Время (Новосибирск)", "Статус по графику", "Переработка", "Вне офиса", "Автозакрытие", "Оператор"},
			rows)
		return
	}
	if items == nil {
		items = []repo.ReportMark{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleReportShifts(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	from, to, ok := s.reportRange(w, r)
	if !ok {
		return
	}
	ids, position := reportFilters(r)
	limit := parseLimit(r.URL.Query().Get("limit"), 5000, 50000)
	items, err := rp.ReportShifts(r.Context(), from, to, ids, position, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось построить отчёт")
		return
	}
	if exportCSV(r) {
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			ended := ""
			if item.EndedAt != nil {
				ended = item.EndedAt.In(shift.NovosibirskZone).Format("2006-01-02 15:04:05")
			}
			rows = append(rows, []string{
				item.EmployeeNumber, item.FullName,
				item.StartedAt.In(shift.NovosibirskZone).Format("2006-01-02 15:04:05"), ended,
				item.Status, boolText(item.Overtime), intText(item.WorkedMinutes),
			})
		}
		writeCSV(w, "shifts-report.csv",
			[]string{"Табельный номер", "ФИО", "Начало (Новосибирск)", "Окончание (Новосибирск)", "Статус", "Переработка", "Минут"},
			rows)
		return
	}
	if items == nil {
		items = []repo.ReportShift{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleReportSummary(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	from, to, ok := s.reportRange(w, r)
	if !ok {
		return
	}
	ids, position := reportFilters(r)
	items, err := rp.ReportSummary(r.Context(), from, to, ids, position)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось построить отчёт")
		return
	}
	if exportCSV(r) {
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{
				item.EmployeeNumber, item.FullName, deref(item.Position),
				strconv.Itoa(item.ShiftsTotal), strconv.Itoa(item.ShiftsClosed),
				strconv.Itoa(item.WorkedMinutes), strconv.Itoa(item.OvertimeShifts),
				strconv.Itoa(item.MarksTotal),
			})
		}
		writeCSV(w, "summary-report.csv",
			[]string{"Табельный номер", "ФИО", "Должность", "Смен", "Закрытых", "Минут", "Переработок", "Отметок"},
			rows)
		return
	}
	if items == nil {
		items = []repo.ReportSummary{}
	}
	writeJSON(w, http.StatusOK, items)
}

func exportCSV(r *http.Request) bool {
	value := strings.TrimSpace(r.URL.Query().Get("format"))
	return value == "csv" || value == "excel" || value == "xlsx"
}

func writeCSV(w http.ResponseWriter, filename string, header []string, rows [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	_ = writer.Write(header)
	for _, row := range rows {
		_ = writer.Write(row)
	}
	writer.Flush()
}

func boolText(value bool) string {
	if value {
		return "да"
	}
	return "нет"
}

func intText(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
