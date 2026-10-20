package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"tabel/internal/repo"
)

func (s *Server) handleListOffices(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	offices, err := rp.ListOffices(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить офисы")
		return
	}
	if offices == nil {
		offices = []repo.Office{}
	}
	writeJSON(w, http.StatusOK, offices)
}

func (s *Server) handleCreateOffice(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	var in repo.OfficeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if !validateOfficeInput(w, &in) {
		return
	}
	office, err := rp.CreateOffice(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось создать офис")
		return
	}
	principal, _ := principalFrom(r.Context())
	s.audit(r, rp, &principal.UserID, "office.create", "office", office.ID, map[string]any{
		"name":     office.Name,
		"radius_m": office.RadiusM,
	})
	writeJSON(w, http.StatusCreated, office)
}

func (s *Server) handleUpdateOffice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	var in repo.OfficeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if !validateOfficeInput(w, &in) {
		return
	}
	office, err := rp.UpdateOffice(r.Context(), id, in)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "офис не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сохранить офис")
		return
	}
	principal, _ := principalFrom(r.Context())
	s.audit(r, rp, &principal.UserID, "office.update", "office", office.ID, map[string]any{
		"name":     office.Name,
		"lat":      office.Lat,
		"lon":      office.Lon,
		"radius_m": office.RadiusM,
	})
	writeJSON(w, http.StatusOK, office)
}

func (s *Server) handleDeleteOffice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	err := rp.DeleteOffice(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "офис не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось удалить офис")
		return
	}
	principal, _ := principalFrom(r.Context())
	s.audit(r, rp, &principal.UserID, "office.delete", "office", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func validateOfficeInput(w http.ResponseWriter, in *repo.OfficeInput) bool {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "поле name обязательно")
		return false
	}
	if in.Lat < -90 || in.Lat > 90 {
		writeError(w, http.StatusBadRequest, "lat вне диапазона [-90, 90]")
		return false
	}
	if in.Lon < -180 || in.Lon > 180 {
		writeError(w, http.StatusBadRequest, "lon вне диапазона [-180, 180]")
		return false
	}
	if in.RadiusM != nil && *in.RadiusM <= 0 {
		writeError(w, http.StatusBadRequest, "radius_m должен быть положительным")
		return false
	}
	return true
}
