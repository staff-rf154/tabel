package httpapi

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"tabel/internal/repo"
	"tabel/internal/shift"
)

const (
	maxScheduleFileSize = 10 << 20
	maxScheduleRows     = 5000
)

type scheduleCSVRow struct {
	Line           int
	EmployeeNumber string
	EmployeeID     string
	PatternType    string
	Workdays       []int
	StartTime      string
	EndTime        string
	ValidFrom      string
	ValidTo        *string
	SourceFile     string
	Err            string
}

type scheduleCSVResult struct {
	rows []scheduleCSVRow
	err  string
}

func (s *Server) handleGetMySchedule(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
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
		writeError(w, http.StatusNotFound, "график не задан")
		return
	}

	now := time.Now().In(shift.NovosibirskZone)
	date := now.Format("2006-01-02")
	schedule, err := rp.GetActiveScheduleForDate(r.Context(), *user.EmployeeID, date)
	if errors.Is(err, repo.ErrNotFound) {
		writeJSON(w, http.StatusOK, shift.DefaultSchedule(*user.EmployeeID, now))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения графика")
		return
	}
	writeJSON(w, http.StatusOK, schedule)
}

func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	employeeID := strings.TrimSpace(r.URL.Query().Get("employee_id"))
	if employeeID != "" {
		if _, err := uuid.Parse(employeeID); err != nil {
			writeError(w, http.StatusBadRequest, "employee_id должен быть UUID")
			return
		}
	}
	schedules, err := rp.ListSchedules(r.Context(), employeeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить графики")
		return
	}
	if schedules == nil {
		schedules = []repo.Schedule{}
	}
	writeJSON(w, http.StatusOK, schedules)
}

type scheduleRequest struct {
	EmployeeID  string   `json:"employee_id"`
	EmployeeIDs []string `json:"employee_ids"`
	PatternType string   `json:"pattern_type"`
	Workdays    []int    `json:"workdays"`
	StartTime   string   `json:"start_time"`
	EndTime     string   `json:"end_time"`
	ValidFrom   string   `json:"valid_from"`
	ValidTo     *string  `json:"valid_to"`
	IsActive    *bool    `json:"is_active"`
}

func normalizeWorkdays(days []int, pattern string) ([]int, error) {
	if len(days) == 0 {
		if pattern == "5/2" {
			return []int{1, 2, 3, 4, 5}, nil
		}
		return nil, errors.New("выберите дни недели")
	}
	seen := make(map[int]struct{}, len(days))
	for _, day := range days {
		if day < 1 || day > 7 {
			return nil, errors.New("день недели должен быть от 1 до 7")
		}
		seen[day] = struct{}{}
	}
	result := make([]int, 0, len(seen))
	for day := 1; day <= 7; day++ {
		if _, ok := seen[day]; ok {
			result = append(result, day)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("выберите дни недели")
	}
	return result, nil
}

func scheduleInputFromRequest(w http.ResponseWriter, r *http.Request, requireEmployee bool) (repo.ScheduleInput, *bool, bool) {
	var req scheduleRequest
	if !decodeJSON(w, r, &req) {
		return repo.ScheduleInput{}, nil, false
	}
	in := repo.ScheduleInput{}
	if requireEmployee {
		employeeID := strings.TrimSpace(req.EmployeeID)
		if _, err := uuid.Parse(employeeID); err != nil {
			writeError(w, http.StatusBadRequest, "выберите сотрудника")
			return repo.ScheduleInput{}, nil, false
		}
		in.EmployeeID = employeeID
	}

	pattern, err := normalizePattern(req.PatternType)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return repo.ScheduleInput{}, nil, false
	}
	in.PatternType = pattern
	workdays, err := normalizeWorkdays(req.Workdays, pattern)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return repo.ScheduleInput{}, nil, false
	}
	in.Workdays = workdays

	start, err := shift.NormalizeClock(req.StartTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "время начала: "+err.Error())
		return repo.ScheduleInput{}, nil, false
	}
	in.StartTime = start
	end, err := shift.NormalizeClock(req.EndTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "время окончания: "+err.Error())
		return repo.ScheduleInput{}, nil, false
	}
	in.EndTime = end

	validFrom := strings.TrimSpace(req.ValidFrom)
	if validFrom == "" {
		writeError(w, http.StatusBadRequest, "укажите дату начала действия")
		return repo.ScheduleInput{}, nil, false
	}
	if _, err := time.Parse("2006-01-02", validFrom); err != nil {
		writeError(w, http.StatusBadRequest, "дата начала должна быть в формате ГГГГ-ММ-ДД")
		return repo.ScheduleInput{}, nil, false
	}
	in.ValidFrom = validFrom
	if req.ValidTo != nil {
		value := strings.TrimSpace(*req.ValidTo)
		if value == "" {
			in.ValidTo = nil
		} else {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				writeError(w, http.StatusBadRequest, "дата окончания должна быть в формате ГГГГ-ММ-ДД")
				return repo.ScheduleInput{}, nil, false
			}
			if value < in.ValidFrom {
				writeError(w, http.StatusBadRequest, "дата окончания не может быть раньше даты начала")
				return repo.ScheduleInput{}, nil, false
			}
			in.ValidTo = &value
		}
	}
	return in, req.IsActive, true
}

func (s *Server) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	in, activePtr, ok := scheduleInputFromRequest(w, r, true)
	if !ok {
		return
	}
	active := true
	if activePtr != nil {
		active = *activePtr
	}
	principal, _ := principalFrom(r.Context())
	in.CreatedBy = &principal.UserID
	schedule, err := rp.CreateSchedule(r.Context(), in, active)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		writeError(w, http.StatusNotFound, "сотрудник не найден")
		return
	case errors.Is(err, repo.ErrEmployeeInactive):
		writeError(w, http.StatusConflict, "сотрудник деактивирован")
		return
	case errors.Is(err, repo.ErrScheduleOverlap):
		writeError(w, http.StatusConflict, "график пересекается по срокам с существующим")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "не удалось создать график")
		return
	}
	s.audit(r, rp, &principal.UserID, "schedule.create", "schedule", schedule.ID, map[string]any{
		"employee_id": schedule.EmployeeID,
		"valid_from":  schedule.ValidFrom,
		"valid_to":    schedule.ValidTo,
	})
	writeJSON(w, http.StatusCreated, schedule)
}

func (s *Server) handleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный идентификатор графика")
		return
	}
	existing, err := rp.GetScheduleByID(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "график не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать график")
		return
	}
	in, activePtr, ok := scheduleInputFromRequest(w, r, false)
	if !ok {
		return
	}
	in.EmployeeID = existing.EmployeeID
	active := existing.IsActive
	if activePtr != nil {
		active = *activePtr
	}
	principal, _ := principalFrom(r.Context())
	schedule, err := rp.UpdateSchedule(r.Context(), id, in, active)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		writeError(w, http.StatusNotFound, "график не найден")
		return
	case errors.Is(err, repo.ErrEmployeeInactive):
		writeError(w, http.StatusConflict, "сотрудник деактивирован")
		return
	case errors.Is(err, repo.ErrScheduleOverlap):
		writeError(w, http.StatusConflict, "график пересекается по срокам с существующим")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "не удалось сохранить график")
		return
	}
	s.audit(r, rp, &principal.UserID, "schedule.update", "schedule", schedule.ID, map[string]any{
		"employee_id": schedule.EmployeeID,
		"valid_from":  schedule.ValidFrom,
		"valid_to":    schedule.ValidTo,
	})
	writeJSON(w, http.StatusOK, schedule)
}

func (s *Server) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный идентификатор графика")
		return
	}
	if err := rp.DeleteSchedule(r.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "график не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, "не удалось удалить график")
		return
	}
	principal, _ := principalFrom(r.Context())
	s.audit(r, rp, &principal.UserID, "schedule.delete", "schedule", id, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleBulkSchedules(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	var req scheduleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.EmployeeIDs) == 0 {
		writeError(w, http.StatusBadRequest, "выберите хотя бы одного сотрудника")
		return
	}
	if len(req.EmployeeIDs) > 500 {
		writeError(w, http.StatusBadRequest, "за один раз можно изменить не более 500 сотрудников")
		return
	}
	body := req
	body.EmployeeIDs = nil
	raw, _ := json.Marshal(body)
	r.Body = io.NopCloser(bytes.NewReader(raw))
	in, activePtr, ok := scheduleInputFromRequest(w, r, false)
	if !ok {
		return
	}
	active := true
	if activePtr != nil {
		active = *activePtr
	}
	principal, _ := principalFrom(r.Context())
	in.CreatedBy = &principal.UserID

	seen := make(map[string]struct{}, len(req.EmployeeIDs))
	report := importReport{Rows: make([]importRow, 0, len(req.EmployeeIDs))}
	for index, rawID := range req.EmployeeIDs {
		id := strings.TrimSpace(rawID)
		report.RowsTotal++
		row := importRow{Line: index + 1, Status: "ok"}
		fail := func(message string) {
			row.Status = "error"
			row.Reason = &message
			report.RowsError++
			report.Rows = append(report.Rows, row)
		}
		if _, err := uuid.Parse(id); err != nil {
			fail("некорректный идентификатор сотрудника")
			continue
		}
		if _, exists := seen[id]; exists {
			fail("сотрудник указан повторно")
			continue
		}
		seen[id] = struct{}{}
		input := in
		input.EmployeeID = id
		if _, err := rp.CreateSchedule(r.Context(), input, active); err != nil {
			switch {
			case errors.Is(err, repo.ErrNotFound):
				fail("сотрудник не найден")
			case errors.Is(err, repo.ErrEmployeeInactive):
				fail("сотрудник деактивирован")
			case errors.Is(err, repo.ErrScheduleOverlap):
				fail("график пересекается по срокам с существующим")
			default:
				fail("не удалось сохранить график")
			}
			continue
		}
		report.RowsOK++
		report.Rows = append(report.Rows, row)
	}
	s.audit(r, rp, &principal.UserID, "schedule.bulk", "schedule", "", map[string]any{
		"rows_total": report.RowsTotal,
		"rows_ok":    report.RowsOK,
		"rows_error": report.RowsError,
	})
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleImportSchedules(w http.ResponseWriter, r *http.Request) {
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = "preview"
	}
	if mode != "preview" && mode != "commit" {
		writeError(w, http.StatusBadRequest, "mode должен быть preview или commit")
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxScheduleFileSize+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "не удалось прочитать multipart-форму")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "поле file обязательно")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxScheduleFileSize+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "не удалось прочитать файл")
		return
	}
	if len(data) > maxScheduleFileSize {
		writeError(w, http.StatusRequestEntityTooLarge, "файл слишком большой")
		return
	}
	if !strings.EqualFold(strings.TrimPrefix(filepathExt(header.Filename), "."), "csv") {
		writeError(w, http.StatusBadRequest, "ожидается CSV-файл")
		return
	}

	now := time.Now().In(shift.NovosibirskZone)
	parseResult := parseScheduleCSV(header.Filename, data, now.Format("2006-01-02"))
	if parseResult.err != "" {
		writeError(w, http.StatusBadRequest, parseResult.err)
		return
	}
	rows := parseResult.rows
	if len(rows) == 0 {
		writeError(w, http.StatusBadRequest, "файл не содержит строк")
		return
	}
	if len(rows) > maxScheduleRows {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("слишком много строк: максимум %d", maxScheduleRows))
		return
	}

	resolveScheduleRows(r.Context(), rp, rows)
	markScheduleOverlaps(rows)

	statuses := make([]string, len(rows))
	reasons := make([]string, len(rows))
	for i := range rows {
		if rows[i].Err != "" {
			statuses[i] = "error"
			reasons[i] = rows[i].Err
		}
	}

	if mode == "commit" {
		groups := make(map[string][]int)
		for i := range rows {
			if rows[i].Err == "" {
				groups[rows[i].EmployeeID] = append(groups[rows[i].EmployeeID], i)
			}
		}
		principal, _ := principalFrom(r.Context())
		for employeeID, indices := range groups {
			inputs := make([]repo.ScheduleInput, 0, len(indices))
			for _, index := range indices {
				row := rows[index]
				inputs = append(inputs, repo.ScheduleInput{
					EmployeeID:  employeeID,
					PatternType: row.PatternType,
					Workdays:    row.Workdays,
					StartTime:   row.StartTime,
					EndTime:     row.EndTime,
					ValidFrom:   row.ValidFrom,
					ValidTo:     row.ValidTo,
					SourceFile:  &row.SourceFile,
					CreatedBy:   &principal.UserID,
				})
			}
			if _, err := rp.ReplaceSchedules(r.Context(), employeeID, now.Format("2006-01-02"), inputs); err != nil {
				reason := "не удалось сохранить график"
				switch {
				case errors.Is(err, repo.ErrNotFound):
					reason = "сотрудник не найден"
				case errors.Is(err, repo.ErrEmployeeInactive):
					reason = "сотрудник деактивирован"
				}
				for _, index := range indices {
					statuses[index] = "error"
					reasons[index] = reason
				}
				continue
			}
			for _, index := range indices {
				statuses[index] = "ok"
			}
			s.audit(r, rp, &principal.UserID, "schedule.import", "employee", employeeID, map[string]any{
				"rows":        len(indices),
				"source_file": header.Filename,
			})
		}
	} else {
		for i := range rows {
			if rows[i].Err == "" {
				statuses[i] = "ok"
			}
		}
	}

	report := importReport{RowsTotal: len(rows), Rows: make([]importRow, 0, len(rows))}
	for i, row := range rows {
		entry := importRow{Line: row.Line, Status: statuses[i]}
		if entry.Status == "" {
			entry.Status = "error"
		}
		if reasons[i] != "" {
			reason := reasons[i]
			entry.Reason = &reason
			report.RowsError++
		} else {
			report.RowsOK++
		}
		report.Rows = append(report.Rows, entry)
	}
	writeJSON(w, http.StatusOK, report)
}

func parseScheduleCSV(filename string, data []byte, defaultDate string) scheduleCSVResult {
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	if len(data) == 0 {
		return scheduleCSVResult{err: "файл пуст"}
	}
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		return scheduleCSVResult{err: "не удалось прочитать заголовок CSV"}
	}
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[normalizeHeader(name)] = i
	}
	employeeIndex, ok := index["employee_number"]
	if !ok {
		return scheduleCSVResult{err: "в CSV нет обязательной колонки employee_number"}
	}
	fieldIndex := func(names ...string) int {
		for _, name := range names {
			if i, ok := index[name]; ok {
				return i
			}
		}
		return -1
	}
	patternIndex := fieldIndex("pattern_type")
	workdaysIndex := fieldIndex("workdays")
	startIndex := fieldIndex("start_time")
	endIndex := fieldIndex("end_time")
	fromIndex := fieldIndex("valid_from", "date_from")
	toIndex := fieldIndex("valid_to", "date_to")

	var rows []scheduleCSVRow
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return scheduleCSVResult{err: "некорректная строка CSV: " + err.Error()}
		}
		if len(record) == 0 {
			continue
		}
		line, _ := reader.FieldPos(0)
		row := scheduleCSVRow{
			Line:           line,
			EmployeeNumber: strings.TrimSpace(csvValue(record, employeeIndex)),
			PatternType:    "5/2",
			StartTime:      "09:00",
			EndTime:        "18:00",
			ValidFrom:      defaultDate,
			SourceFile:     filename,
		}
		if row.EmployeeNumber == "" {
			row.Err = "пустой employee_number"
		}
		if raw := strings.TrimSpace(csvValue(record, patternIndex)); raw != "" {
			pattern, err := normalizePattern(raw)
			if err != nil {
				row.Err = err.Error()
			} else {
				row.PatternType = pattern
			}
		}
		workdays, err := parseWorkdays(csvValue(record, workdaysIndex), row.PatternType)
		if err != nil {
			row.Err = err.Error()
		} else {
			row.Workdays = workdays
		}
		if raw := strings.TrimSpace(csvValue(record, startIndex)); raw != "" {
			row.StartTime = raw
		}
		if raw := strings.TrimSpace(csvValue(record, endIndex)); raw != "" {
			row.EndTime = raw
		}
		if normalized, err := shift.NormalizeClock(row.StartTime); err != nil {
			row.Err = err.Error()
		} else {
			row.StartTime = normalized
		}
		if normalized, err := shift.NormalizeClock(row.EndTime); err != nil {
			row.Err = err.Error()
		} else {
			row.EndTime = normalized
		}
		if raw := strings.TrimSpace(csvValue(record, fromIndex)); raw != "" {
			if _, err := time.Parse("2006-01-02", raw); err != nil {
				row.Err = "valid_from должен быть в формате YYYY-MM-DD"
			} else {
				row.ValidFrom = raw
			}
		}
		if raw := strings.TrimSpace(csvValue(record, toIndex)); raw != "" {
			if _, err := time.Parse("2006-01-02", raw); err != nil {
				row.Err = "valid_to должен быть в формате YYYY-MM-DD"
			} else if raw < row.ValidFrom {
				row.Err = "valid_to не может быть раньше valid_from"
			} else {
				value := raw
				row.ValidTo = &value
			}
		}
		rows = append(rows, row)
	}
	if rows == nil {
		rows = []scheduleCSVRow{}
	}
	return scheduleCSVResult{rows: rows}
}

func resolveScheduleRows(ctx context.Context, rp *repo.Repo, rows []scheduleCSVRow) {
	for i := range rows {
		if rows[i].Err != "" {
			continue
		}
		employee, err := rp.FindEmployeeByNumber(ctx, rows[i].EmployeeNumber)
		if errors.Is(err, repo.ErrNotFound) {
			rows[i].Err = "сотрудник с таким табельным номером не найден"
			continue
		}
		if err != nil {
			rows[i].Err = "ошибка чтения сотрудника"
			continue
		}
		if !employee.IsActive {
			rows[i].Err = "сотрудник деактивирован"
			continue
		}
		rows[i].EmployeeID = employee.ID
	}
}

func markScheduleOverlaps(rows []scheduleCSVRow) {
	groups := make(map[string][]int)
	for i := range rows {
		if rows[i].Err == "" {
			groups[rows[i].EmployeeNumber] = append(groups[rows[i].EmployeeNumber], i)
		}
	}
	for _, indices := range groups {
		sort.Slice(indices, func(a, b int) bool {
			if rows[indices[a]].ValidFrom == rows[indices[b]].ValidFrom {
				return rows[indices[a]].Line < rows[indices[b]].Line
			}
			return rows[indices[a]].ValidFrom < rows[indices[b]].ValidFrom
		})
		last := indices[0]
		for _, index := range indices[1:] {
			previous := rows[last]
			if previous.ValidTo == nil || rows[index].ValidFrom <= *previous.ValidTo {
				rows[index].Err = fmt.Sprintf("пересекается по срокам со строкой %d", previous.Line)
				continue
			}
			last = index
		}
	}
}

func parseWorkdays(raw, pattern string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if pattern == "5/2" {
			return []int{1, 2, 3, 4, 5}, nil
		}
		return nil, errors.New("workdays обязательны для выбранного типа графика")
	}
	replacer := strings.NewReplacer(",", " ", ";", " ", "|", " ")
	fields := strings.Fields(replacer.Replace(raw))
	days := make(map[int]struct{})
	for _, field := range fields {
		if strings.Contains(field, "-") {
			bounds := strings.SplitN(field, "-", 2)
			from, err1 := strconv.Atoi(strings.TrimSpace(bounds[0]))
			to, err2 := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err1 != nil || err2 != nil || from < 1 || to > 7 || from > to {
				return nil, fmt.Errorf("некорректный диапазон дней %q", field)
			}
			for day := from; day <= to; day++ {
				days[day] = struct{}{}
			}
			continue
		}
		day, err := strconv.Atoi(field)
		if err != nil || day < 1 || day > 7 {
			return nil, fmt.Errorf("некорректный день недели %q", field)
		}
		days[day] = struct{}{}
	}
	if len(days) == 0 {
		return nil, errors.New("не удалось разобрать workdays")
	}
	result := make([]int, 0, len(days))
	for day := 1; day <= 7; day++ {
		if _, ok := days[day]; ok {
			result = append(result, day)
		}
	}
	return result, nil
}

func normalizePattern(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "5/2":
		return "5/2", nil
	case "2/2":
		return "2/2", nil
	case "custom":
		return "custom", nil
	default:
		return "", fmt.Errorf("неизвестный тип графика %q", raw)
	}
}

func normalizeHeader(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "\ufeff")))
}

func csvValue(record []string, index int) string {
	if index < 0 || index >= len(record) {
		return ""
	}
	return record[index]
}

func filepathExt(name string) string {
	index := strings.LastIndex(name, ".")
	if index < 0 {
		return ""
	}
	return name[index:]
}
