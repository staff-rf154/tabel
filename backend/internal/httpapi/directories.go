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

const maxDirectoryImportSize = 5 << 20

func (s *Server) handleListDepartments(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	items, err := rp.ListDepartments(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить отделы")
		return
	}
	if items == nil {
		items = []repo.Department{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleCreateDepartment(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	var in repo.DepartmentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "поле name обязательно")
		return
	}
	item, err := rp.CreateDepartment(r.Context(), in)
	if errors.Is(err, repo.ErrNameTaken) {
		writeError(w, http.StatusConflict, "отдел с таким названием уже существует")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось создать отдел")
		return
	}
	principal, _ := principalFrom(r.Context())
	s.audit(r, rp, &principal.UserID, "department.create", "department", item.ID, map[string]any{"name": item.Name})
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleListPositions(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	items, err := rp.ListPositions(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить должности")
		return
	}
	if items == nil {
		items = []repo.Position{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleCreatePosition(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	var in repo.PositionInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "поле name обязательно")
		return
	}
	item, err := rp.CreatePosition(r.Context(), in)
	if errors.Is(err, repo.ErrNameTaken) {
		writeError(w, http.StatusConflict, "должность с таким названием уже существует")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось создать должность")
		return
	}
	principal, _ := principalFrom(r.Context())
	s.audit(r, rp, &principal.UserID, "position.create", "position", item.ID, map[string]any{"name": item.Name})
	writeJSON(w, http.StatusCreated, item)
}

func readDirectoryCSV(w http.ResponseWriter, r *http.Request) ([]string, []string, bool) {
	if err := r.ParseMultipartForm(maxDirectoryImportSize); err != nil {
		writeError(w, http.StatusBadRequest, "не удалось прочитать форму")
		return nil, nil, false
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "файл обязателен (поле file)")
		return nil, nil, false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxDirectoryImportSize+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "не удалось прочитать файл")
		return nil, nil, false
	}
	if len(data) > maxDirectoryImportSize {
		writeError(w, http.StatusRequestEntityTooLarge, "файл слишком большой")
		return nil, nil, false
	}
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		writeError(w, http.StatusBadRequest, "некорректный CSV: "+err.Error())
		return nil, nil, false
	}
	if len(records) == 0 {
		writeError(w, http.StatusBadRequest, "файл пуст")
		return nil, nil, false
	}
	if len(records)-1 > 5000 {
		writeError(w, http.StatusRequestEntityTooLarge, "слишком много строк")
		return nil, nil, false
	}
	header := make([]string, len(records[0]))
	for i, name := range records[0] {
		header[i] = strings.ToLower(strings.TrimSpace(name))
	}
	rows := make([]string, 0, len(records)-1)
	for _, record := range records[1:] {
		name := ""
		if len(record) > 0 {
			name = strings.TrimSpace(record[0])
		}
		rows = append(rows, name)
	}
	return header, rows, true
}

func (s *Server) handleImportDepartments(w http.ResponseWriter, r *http.Request) {
	s.importDirectory(w, r, "departments")
}

func (s *Server) handleImportPositions(w http.ResponseWriter, r *http.Request) {
	s.importDirectory(w, r, "positions")
}

func (s *Server) importDirectory(w http.ResponseWriter, r *http.Request, kind string) {
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
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
	header, rows, ok := readDirectoryCSV(w, r)
	if !ok {
		return
	}
	if len(header) == 0 || strings.TrimSpace(header[0]) != "name" {
		writeError(w, http.StatusBadRequest, "в CSV нет обязательной колонки name")
		return
	}
	report := importReport{Rows: []importRow{}}
	seen := make(map[string]struct{}, len(rows))
	for index, name := range rows {
		line := index + 2
		report.RowsTotal++
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists && name != "" {
			report.RowsError++
			message := "дубликат в файле"
			report.Rows = append(report.Rows, importRow{Line: line, Status: "error", Reason: &message})
			continue
		}
		seen[key] = struct{}{}
		if name == "" {
			report.RowsError++
			message := "название обязательно"
			report.Rows = append(report.Rows, importRow{Line: line, Status: "error", Reason: &message})
			continue
		}
		if mode == "preview" {
			var found bool
			if kind == "departments" {
				_, err := rp.FindDepartmentByName(r.Context(), name)
				found = err == nil
			} else {
				_, err := rp.FindPositionByName(r.Context(), name)
				found = err == nil
			}
			if found {
				report.RowsError++
				message := "уже существует"
				report.Rows = append(report.Rows, importRow{Line: line, Status: "error", Reason: &message})
				continue
			}
			report.RowsOK++
			report.Rows = append(report.Rows, importRow{Line: line, Status: "ok", Reason: nil})
			continue
		}
		var err error
		if kind == "departments" {
			_, err = rp.CreateDepartment(r.Context(), repo.DepartmentInput{Name: name})
		} else {
			_, err = rp.CreatePosition(r.Context(), repo.PositionInput{Name: name})
		}
		if errors.Is(err, repo.ErrNameTaken) {
			report.RowsError++
			message := "уже существует"
			report.Rows = append(report.Rows, importRow{Line: line, Status: "error", Reason: &message})
			continue
		}
		if err != nil {
			report.RowsError++
			message := "не удалось сохранить"
			report.Rows = append(report.Rows, importRow{Line: line, Status: "error", Reason: &message})
			continue
		}
		report.RowsOK++
		report.Rows = append(report.Rows, importRow{Line: line, Status: "ok", Reason: nil})
	}
	if mode == "commit" {
		principal, _ := principalFrom(r.Context())
		s.audit(r, rp, &principal.UserID, kind+".import", kind, "", map[string]any{
			"rows_total": report.RowsTotal,
			"rows_ok":    report.RowsOK,
			"rows_error": report.RowsError,
		})
	}
	writeJSON(w, http.StatusOK, report)
}
