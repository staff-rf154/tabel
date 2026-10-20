package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"tabel/internal/repo"
)

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	query := r.URL.Query()
	unread := strings.TrimSpace(query.Get("unread_only")) == "true" || strings.TrimSpace(query.Get("unread")) == "true"
	limit := parseLimit(query.Get("limit"), 50, 200)
	notifications, err := rp.ListNotifications(r.Context(), principal.UserID, unread, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить уведомления")
		return
	}
	if notifications == nil {
		notifications = []repo.Notification{}
	}
	writeJSON(w, http.StatusOK, notifications)
}

func (s *Server) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	if err := rp.MarkNotificationRead(r.Context(), principal.UserID, id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "уведомление не найдено")
			return
		}
		writeError(w, http.StatusInternalServerError, "не удалось отметить уведомление")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
