package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"tabel/internal/repo"
)

// handle1CDepartments отдаёт справочник отделов для загрузки в 1С.
func (s *Server) handle1CDepartments(w http.ResponseWriter, r *http.Request) {
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

// handle1CPositions отдаёт справочник должностей для загрузки в 1С.
func (s *Server) handle1CPositions(w http.ResponseWriter, r *http.Request) {
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

// handle1CEmployees отдаёт постраничный список сотрудников для 1С:ЗУП/УПП.
func (s *Server) handle1CEmployees(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	query := r.URL.Query()
	limit := parseLimit(query.Get("limit"), 500, 500)
	offset := 0
	if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			writeError(w, http.StatusBadRequest, "offset должен быть неотрицательным целым")
			return
		}
		offset = value
	}
	isActive, err := optionalBool(query.Get("is_active"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "is_active должен быть true или false")
		return
	}
	items, err := rp.ListEmployees(r.Context(), strings.TrimSpace(query.Get("q")), isActive, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить сотрудников")
		return
	}
	if items == nil {
		items = []repo.Employee{}
	}
	writeJSON(w, http.StatusOK, items)
}

// handle1CSchedules отдаёт графики сотрудников для 1С.
func (s *Server) handle1CSchedules(w http.ResponseWriter, r *http.Request) {
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
	items, err := rp.ListSchedules(r.Context(), employeeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить графики")
		return
	}
	if items == nil {
		items = []repo.Schedule{}
	}
	writeJSON(w, http.StatusOK, items)
}
