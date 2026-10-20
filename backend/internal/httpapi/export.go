package httpapi

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"tabel/internal/repo"
)

func (s *Server) handleExportMarks(w http.ResponseWriter, r *http.Request) {
	principal, _ := oauthPrincipalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	query := r.URL.Query()
	var since *time.Time
	if raw := strings.TrimSpace(query.Get("since")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since должен быть RFC3339")
			return
		}
		since = &parsed
	}
	var cursorTS *time.Time
	cursorID := ""
	if raw := strings.TrimSpace(query.Get("cursor")); raw != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "некорректный cursor")
			return
		}
		parts := strings.SplitN(string(decoded), "|", 2)
		if len(parts) != 2 {
			writeError(w, http.StatusBadRequest, "некорректный cursor")
			return
		}
		parsed, err := time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			writeError(w, http.StatusBadRequest, "некорректный cursor")
			return
		}
		if _, err := uuid.Parse(parts[1]); err != nil {
			writeError(w, http.StatusBadRequest, "некорректный cursor")
			return
		}
		cursorTS = &parsed
		cursorID = parts[1]
	}
	limit := parseLimit(query.Get("limit"), 1000, 1000)

	items, err := rp.ExportMarks(r.Context(), since, cursorTS, cursorID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить отметки")
		return
	}
	if items == nil {
		items = []repo.ExportMark{}
	}
	var nextCursor *string
	if len(items) == limit && limit > 0 {
		last := items[len(items)-1]
		encoded := base64.RawURLEncoding.EncodeToString([]byte(last.TimestampServer.UTC().Format(time.RFC3339Nano) + "|" + last.MarkID))
		nextCursor = &encoded
	}
	_ = principal
	writeJSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": nextCursor,
	})
}

func (s *Server) handleExportAck(w http.ResponseWriter, r *http.Request) {
	principal, _ := oauthPrincipalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	var req struct {
		MarkIDs []string `json:"mark_ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.MarkIDs) == 0 {
		writeError(w, http.StatusBadRequest, "mark_ids обязателен")
		return
	}
	if len(req.MarkIDs) > 1000 {
		writeError(w, http.StatusBadRequest, "не более 1000 mark_ids за запрос")
		return
	}
	for _, id := range req.MarkIDs {
		if _, err := uuid.Parse(id); err != nil {
			writeError(w, http.StatusBadRequest, "mark_ids должны быть UUID")
			return
		}
	}
	acknowledged, err := rp.AckExports(r.Context(), principal.ClientID, req.MarkIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось подтвердить отметки")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"acknowledged": acknowledged})
}

func parseLimit(raw string, fallback, max int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	value := 0
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return fallback
		}
		value = value*10 + int(ch-'0')
		if value > max {
			return max
		}
	}
	if value <= 0 {
		return fallback
	}
	return value
}
