package httpapi

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"strings"

	"tabel/internal/repo"
)

const maxEmployeeImportSize = 10 << 20

var employeeImportHeader = []string{"employee_number", "last_name", "first_name", "middle_name", "position", "department", "email", "phone", "hired_at"}

func (s *Server) handleImportEmployees(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = "preview"
	}
	if mode != "preview" && mode != "commit" {
		writeError(w, http.StatusBadRequest, "mode должен быть preview или commit")
		return
	}
	if err := r.ParseMultipartForm(maxEmployeeImportSize); err != nil {
		writeError(w, http.StatusBadRequest, "не удалось прочитать форму")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "файл обязателен (поле file)")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxEmployeeImportSize+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "не удалось прочитать файл")
		return
	}
	if len(data) > maxEmployeeImportSize {
		writeError(w, http.StatusRequestEntityTooLarge, "файл слишком большой")
		return
	}
	filename := "employees.csv"
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
	if len(records)-1 > 5000 {
		writeError(w, http.StatusRequestEntityTooLarge, "слишком много строк")
		return
	}
	index := map[string]int{}
	for i, name := range records[0] {
		index[strings.TrimSpace(strings.ToLower(name))] = i
	}
	if _, ok := index["employee_number"]; !ok {
		writeError(w, http.StatusBadRequest, "в CSV нет обязательной колонки employee_number")
		return
	}
	_, hasFullName := index["full_name"]
	_, hasLastName := index["last_name"]
	_, hasFirstName := index["first_name"]
	if !hasFullName && (!hasLastName || !hasFirstName) {
		writeError(w, http.StatusBadRequest, "в CSV нужен full_name или колонки last_name и first_name")
		return
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
		input := repo.EmployeeInput{
			EmployeeNumber: get(record, "employee_number"),
			FullName:       get(record, "full_name"),
			LastName:       get(record, "last_name"),
			FirstName:      get(record, "first_name"),
			MiddleName:     get(record, "middle_name"),
		}
		if input.LastName != "" || input.FirstName != "" || input.MiddleName != "" {
			if input.LastName == "" || input.FirstName == "" {
				report.RowsError++
				message := "фамилия и имя обязательны"
				report.Rows = append(report.Rows, importRow{Line: line, Status: "error", Reason: &message})
				continue
			}
			input.FullName = joinNameParts(input.LastName, input.FirstName, input.MiddleName)
		} else {
			if input.FullName == "" {
				report.RowsError++
				message := "укажите ФИО или фамилию и имя"
				report.Rows = append(report.Rows, importRow{Line: line, Status: "error", Reason: &message})
				continue
			}
			input.LastName, input.FirstName, input.MiddleName = splitFullName(input.FullName)
		}
		if value := get(record, "position"); value != "" {
			position, err := rp.FindPositionByName(r.Context(), value)
			if err != nil {
				report.RowsError++
				message := "должность не найдена в справочнике: " + value
				report.Rows = append(report.Rows, importRow{Line: line, Status: "error", Reason: &message})
				continue
			}
			input.PositionID = &position.ID
		} else {
			report.RowsError++
			message := "должность обязательна: укажите название из справочника"
			report.Rows = append(report.Rows, importRow{Line: line, Status: "error", Reason: &message})
			continue
		}
		if value := get(record, "department"); value != "" {
			department, err := rp.FindDepartmentByName(r.Context(), value)
			if err != nil {
				report.RowsError++
				message := "отдел не найден: " + value
				report.Rows = append(report.Rows, importRow{Line: line, Status: "error", Reason: &message})
				continue
			}
			input.DepartmentID = &department.ID
		}
		if value := get(record, "email"); value != "" {
			input.Email = &value
		}
		if value := get(record, "phone"); value != "" {
			input.Phone = &value
		}
		if value := get(record, "hired_at"); value != "" {
			input.HiredAt = &value
		}
		if input.EmployeeNumber == "" || input.FullName == "" {
			report.RowsError++
			message := "employee_number и ФИО обязательны"
			report.Rows = append(report.Rows, importRow{Line: line, Status: "error", Reason: &message})
			continue
		}
		if mode == "preview" {
			report.RowsOK++
			report.Rows = append(report.Rows, importRow{Line: line, Status: "ok", Reason: nil})
			continue
		}
		if _, err := rp.CreateEmployee(r.Context(), input); err != nil {
			report.RowsError++
			message := "не удалось создать сотрудника"
			if errors.Is(err, repo.ErrEmployeeNumberTaken) {
				message = "табельный номер уже занят"
			}
			report.Rows = append(report.Rows, importRow{Line: line, Status: "error", Reason: &message})
			continue
		}
		report.RowsOK++
		report.Rows = append(report.Rows, importRow{Line: line, Status: "ok", Reason: nil})
	}

	if mode == "commit" {
		s.audit(r, rp, &principal.UserID, "employee.import", "employee", filename, map[string]any{
			"rows_total": report.RowsTotal,
			"rows_ok":    report.RowsOK,
			"rows_error": report.RowsError,
		})
	}
	writeJSON(w, http.StatusOK, report)
}
