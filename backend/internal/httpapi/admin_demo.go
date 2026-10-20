package httpapi

import (
	"net/http"
)

func (s *Server) handleSeedDemo(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	seeded, err := rp.SeedDemo(r.Context(), &principal.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось создать демо-данные")
		return
	}
	s.audit(r, rp, &principal.UserID, "demo.seed", "employee", "DEMO", map[string]any{"seeded": seeded})
	writeJSON(w, http.StatusOK, map[string]any{"seeded": seeded})
}

func (s *Server) handleClearDemo(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	removed, err := rp.ClearDemo(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось очистить демо-данные")
		return
	}
	s.audit(r, rp, &principal.UserID, "demo.clear", "employee", "DEMO", map[string]any{"removed": removed})
	writeJSON(w, http.StatusOK, map[string]any{"removed": removed})
}
