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

func reportFilters(r *http.Request) ([]string, []string, string) {
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
	var employees []string
	for _, part := range strings.Split(r.URL.Query().Get("employees"), ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			employees = append(employees, part)
		}
	}
	position := strings.TrimSpace(r.URL.Query().Get("position"))
	return ids, employees, position
}

func (s *Server) reportFilterIDs(w http.ResponseWriter, r *http.Request, rp *repo.Repo) ([]string, string, bool) {
	ids, employees, position := reportFilters(r)
	if len(employees) > 0 {
		resolved, missing, err := rp.ResolveReportEmployees(r.Context(), employees)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось определить сотрудников")
			return nil, "", false
		}
		if len(missing) > 0 {
			writeError(w, http.StatusBadRequest, "сотрудники не найдены: "+strings.Join(missing, ", "))
			return nil, "", false
		}
		ids = append(ids, resolved...)
	}
	seen := make(map[string]bool, len(ids))
	unique := ids[:0]
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	return unique, position, true
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
	ids, position, ok := s.reportFilterIDs(w, r, rp)
	if !ok {
		return
	}
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
				item.EmployeeNumber, item.FullName, deref(item.Position), deref(item.Department), item.EventType,
				item.TimestampServer.In(shift.NovosibirskZone).Format("2006-01-02 15:04:05"),
				item.TimingStatus,
				boolText(item.Overtime), boolText(item.OutsideOffice), boolText(item.AutoClosed),
				deref(item.OperatorNumber),
			})
		}
		writeCSV(w, "marks-report.csv",
			[]string{"Табельный номер", "ФИО", "Должность", "Отдел", "Событие", "Время (Новосибирск)", "Статус по графику", "Переработка", "Вне офиса", "Автозакрытие", "Оператор"},
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
	ids, position, ok := s.reportFilterIDs(w, r, rp)
	if !ok {
		return
	}
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
				item.EmployeeNumber, item.FullName, deref(item.Position), deref(item.Department),
				item.StartedAt.In(shift.NovosibirskZone).Format("2006-01-02 15:04:05"), ended,
				item.Status, boolText(item.Overtime), intText(item.WorkedMinutes),
			})
		}
		writeCSV(w, "shifts-report.csv",
			[]string{"Табельный номер", "ФИО", "Должность", "Отдел", "Начало (Новосибирск)", "Окончание (Новосибирск)", "Статус", "Переработка", "Минут"},
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
	ids, position, ok := s.reportFilterIDs(w, r, rp)
	if !ok {
		return
	}
	items, err := rp.ReportSummary(r.Context(), from, to, ids, position)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось построить отчёт")
		return
	}
	if exportCSV(r) {
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{
				item.EmployeeNumber, item.FullName, deref(item.Position), deref(item.Department),
				strconv.Itoa(item.ShiftsTotal), strconv.Itoa(item.ShiftsClosed),
				strconv.Itoa(item.WorkedMinutes), strconv.Itoa(item.OvertimeShifts),
				strconv.Itoa(item.MarksTotal),
			})
		}
		writeCSV(w, "summary-report.csv",
			[]string{"Табельный номер", "ФИО", "Должность", "Отдел", "Смен", "Закрытых", "Минут", "Переработок", "Отметок"},
			rows)
		return
	}
	if items == nil {
		items = []repo.ReportSummary{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleReportWorkedTime(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	from, to, ok := s.reportRange(w, r)
	if !ok {
		return
	}
	ids, position, ok := s.reportFilterIDs(w, r, rp)
	if !ok {
		return
	}
	items, err := rp.ReportWorkedTime(r.Context(), from, to, ids, position)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось построить отчёт")
		return
	}
	if exportCSV(r) {
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{
				item.EmployeeNumber, item.FullName, deref(item.Position), deref(item.Department),
				strconv.Itoa(item.DaysWorked), strconv.Itoa(item.ShiftsTotal),
				strconv.Itoa(item.WorkedMinutes), reportHours(item.WorkedMinutes),
				strconv.Itoa(item.OvertimeShifts), reportHours(item.AverageMinutes),
			})
		}
		writeCSV(w, "worked-time-report.csv",
			[]string{"Табельный номер", "ФИО", "Должность", "Отдел", "Дней", "Смен", "Минут", "Часов", "Переработок", "Среднее в день"},
			rows)
		return
	}
	if items == nil {
		items = []repo.ReportWorkedTime{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleReportFirstLast(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	from, to, ok := s.reportRange(w, r)
	if !ok {
		return
	}
	ids, position, ok := s.reportFilterIDs(w, r, rp)
	if !ok {
		return
	}
	items, err := rp.ReportFirstLast(r.Context(), from, to, ids, position)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось построить отчёт")
		return
	}
	if exportCSV(r) {
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{
				item.EmployeeNumber, item.FullName, deref(item.Position), deref(item.Department),
				item.WorkDate, reportTime(&item.FirstIn), reportTime(item.LastOut),
				reportHours(item.WorkedMinutes),
			})
		}
		writeCSV(w, "first-last-report.csv",
			[]string{"Табельный номер", "ФИО", "Должность", "Отдел", "Дата", "Первый приход", "Последний выход", "Отработано"},
			rows)
		return
	}
	if items == nil {
		items = []repo.ReportFirstLast{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleReportEntriesExits(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	from, to, ok := s.reportRange(w, r)
	if !ok {
		return
	}
	ids, position, ok := s.reportFilterIDs(w, r, rp)
	if !ok {
		return
	}
	limit := parseLimit(r.URL.Query().Get("limit"), 20000, 50000)
	items, err := rp.ReportEntriesExits(r.Context(), from, to, ids, position, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось построить отчёт")
		return
	}
	if exportCSV(r) {
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{
				item.EmployeeNumber, item.FullName, deref(item.Position), deref(item.Department),
				item.WorkDate, reportTime(&item.CheckIn), reportTime(item.CheckOut),
				intText(item.Minutes), item.Status,
			})
		}
		writeCSV(w, "entries-exits-report.csv",
			[]string{"Табельный номер", "ФИО", "Должность", "Отдел", "Дата", "Вход", "Выход", "Минут", "Статус"},
			rows)
		return
	}
	if items == nil {
		items = []repo.ReportEntryExit{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleReportDaily(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	from, to, ok := s.reportRange(w, r)
	if !ok {
		return
	}
	ids, position, ok := s.reportFilterIDs(w, r, rp)
	if !ok {
		return
	}
	items, err := rp.ReportDaily(r.Context(), from, to, ids, position)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось построить отчёт")
		return
	}
	if exportCSV(r) {
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{
				item.EmployeeNumber, item.FullName, deref(item.Position), deref(item.Department),
				item.WorkDate, reportTime(&item.CheckIn), reportTime(item.CheckOut),
				strconv.Itoa(item.PresenceMinutes), reportHours(item.WorkedMinutes),
				strconv.Itoa(item.ShiftsTotal),
			})
		}
		writeCSV(w, "daily-report.csv",
			[]string{"Табельный номер", "ФИО", "Должность", "Отдел", "Дата", "Приход", "Уход", "Присутствие (мин)", "Отработано", "Смен"},
			rows)
		return
	}
	if items == nil {
		items = []repo.ReportDaily{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleReportTimesheet(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	from, to, ok := s.reportRange(w, r)
	if !ok {
		return
	}
	ids, position, ok := s.reportFilterIDs(w, r, rp)
	if !ok {
		return
	}
	daily, err := rp.ReportDaily(r.Context(), from, to, ids, position)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось построить табель")
		return
	}
	items := repo.BuildTimesheet(daily)
	if exportCSV(r) {
		dates := reportDates(from, to)
		header := []string{"Табельный номер", "ФИО", "Должность", "Отдел"}
		header = append(header, dates...)
		header = append(header, "Дней", "Часов", "Переработок")
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			byDate := make(map[string]int, len(item.Days))
			for _, day := range item.Days {
				byDate[day.Date] = day.WorkedMinutes
			}
			row := []string{item.EmployeeNumber, item.FullName, deref(item.Position), deref(item.Department)}
			for _, date := range dates {
				row = append(row, reportHours(byDate[date]))
			}
			row = append(row, strconv.Itoa(item.DaysWorked), reportHours(item.WorkedMinutes), strconv.Itoa(item.OvertimeShifts))
			rows = append(rows, row)
		}
		writeCSV(w, "timesheet-t13.csv", header, rows)
		return
	}
	if items == nil {
		items = []repo.ReportTimesheet{}
	}
	writeJSON(w, http.StatusOK, items)
}

func reportDates(from, to time.Time) []string {
	start := from.In(shift.NovosibirskZone)
	end := to.In(shift.NovosibirskZone)
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, shift.NovosibirskZone)
	last := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, shift.NovosibirskZone)
	var dates []string
	for !day.After(last) && len(dates) < 366 {
		dates = append(dates, day.Format("2006-01-02"))
		day = day.AddDate(0, 0, 1)
	}
	return dates
}

func reportTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.In(shift.NovosibirskZone).Format("2006-01-02 15:04:05")
}

func reportHours(minutes int) string {
	if minutes == 0 {
		return ""
	}
	if minutes%60 == 0 {
		return strconv.Itoa(minutes / 60)
	}
	return strings.ReplaceAll(strconv.FormatFloat(float64(minutes)/60, 'f', 2, 64), ".", ",")
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
