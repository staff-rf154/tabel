package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"tabel/internal/repo"
)

func (s *Server) handleListAdminMarks(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	query := r.URL.Query()
	filter := repo.MarkFilter{}
	for _, item := range []struct {
		key  string
		dest *string
	}{
		{"employee_id", &filter.EmployeeID},
		{"operator_id", &filter.OperatorID},
		{"department_id", &filter.DepartmentID},
		{"position_id", &filter.PositionID},
	} {
		value := strings.TrimSpace(query.Get(item.key))
		if value == "" {
			continue
		}
		if _, err := uuid.Parse(value); err != nil {
			writeError(w, http.StatusBadRequest, item.key+" должен быть UUID")
			return
		}
		*item.dest = value
	}
	if raw := strings.TrimSpace(query.Get("from")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from должен быть датой-временем RFC3339")
			return
		}
		filter.From = &parsed
	}
	if raw := strings.TrimSpace(query.Get("to")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to должен быть датой-временем RFC3339")
			return
		}
		filter.To = &parsed
	}
	for _, item := range []struct {
		key  string
		dest **bool
	}{
		{"outside_office", &filter.OutsideOffice},
		{"mock_location", &filter.MockLocation},
		{"auto_closed_previous", &filter.AutoClosedPrevious},
		{"overtime", &filter.Overtime},
		{"qr_data_mismatch", &filter.QRDataMismatch},
	} {
		value, err := optionalBool(query.Get(item.key))
		if err != nil {
			writeError(w, http.StatusBadRequest, item.key+" должен быть true или false")
			return
		}
		*item.dest = value
	}
	filter.QRSource = strings.TrimSpace(query.Get("qr_source"))
	if filter.QRSource != "" && filter.QRSource != "static" && filter.QRSource != "paper" {
		writeError(w, http.StatusBadRequest, "qr_source должен быть static или paper")
		return
	}
	filter.ClientType = strings.TrimSpace(query.Get("client_type"))
	if filter.ClientType != "" && filter.ClientType != "mobile" && filter.ClientType != "web" {
		writeError(w, http.StatusBadRequest, "client_type должен быть mobile или web")
		return
	}
	filter.Query = strings.TrimSpace(query.Get("q"))
	if len([]rune(filter.Query)) > 100 {
		writeError(w, http.StatusBadRequest, "q слишком длинный")
		return
	}
	var okPagination bool
	filter.Limit, filter.Offset, okPagination = pagination(w, r)
	if !okPagination {
		return
	}

	marks, err := rp.ListMarks(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить отметки")
		return
	}
	if marks == nil {
		marks = []repo.Mark{}
	}
	writeJSON(w, http.StatusOK, marks)
}
