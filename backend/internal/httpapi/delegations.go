package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"tabel/internal/repo"
)

func (s *Server) handleCreateDelegation(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	var req struct {
		ToUserID   string `json:"to_user_id"`
		ToLogin    string `json:"to_login"`
		ValidUntil string `json:"valid_until"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.ToUserID = strings.TrimSpace(req.ToUserID)
	req.ToLogin = strings.TrimSpace(req.ToLogin)
	req.ValidUntil = strings.TrimSpace(req.ValidUntil)
	if req.ToUserID == "" && req.ToLogin == "" {
		writeError(w, http.StatusBadRequest, "укажите логин сотрудника")
		return
	}
	if req.ValidUntil == "" {
		writeError(w, http.StatusBadRequest, "valid_until обязателен")
		return
	}
	if req.ToLogin != "" {
		target, err := rp.FindEmployeeUserByLogin(r.Context(), req.ToLogin)
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "сотрудник с таким логином не найден")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ошибка чтения пользователя")
			return
		}
		req.ToUserID = target.ID
	}
	validUntil, err := time.Parse(time.RFC3339, req.ValidUntil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "valid_until должен быть RFC3339")
		return
	}
	delegation, err := rp.CreateDelegation(r.Context(), principal.UserID, req.ToUserID, validUntil)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		writeError(w, http.StatusNotFound, "пользователь не найден")
		return
	case errors.Is(err, repo.ErrInvalidDelegation):
		writeError(w, http.StatusBadRequest, "делегирование возможно только на будущее время и от оператора к сотруднику")
		return
	case errors.Is(err, repo.ErrDelegationConflict):
		writeError(w, http.StatusConflict, "уже есть активное делегирование")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "не удалось создать делегирование")
		return
	}
	s.audit(r, rp, &principal.UserID, "delegation.request", "delegation", delegation.ID, map[string]any{
		"from_user_id": delegation.FromUserID,
		"to_user_id":   delegation.ToUserID,
		"valid_until":  delegation.ValidUntil,
	})
	writeJSON(w, http.StatusCreated, delegation)
}

func (s *Server) handleListDelegations(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	if _, err := rp.ExpireDelegations(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось обновить статусы делегирований")
		return
	}
	delegations, err := rp.ListDelegations(r.Context(), principal.UserID, principal.Role == "admin")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить список делегирований")
		return
	}
	if delegations == nil {
		delegations = []repo.Delegation{}
	}
	writeJSON(w, http.StatusOK, delegations)
}

func (s *Server) handleDecideDelegation(w http.ResponseWriter, r *http.Request, approve bool) {
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
	delegation, err := rp.DecideDelegation(r.Context(), id, principal.UserID, approve)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		writeError(w, http.StatusNotFound, "делегирование не найдено")
		return
	case errors.Is(err, repo.ErrDelegationState):
		writeError(w, http.StatusConflict, "делегирование не может быть изменено")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "не удалось изменить делегирование")
		return
	}
	action := "delegation.reject"
	if approve {
		action = "delegation.approve"
	}
	s.audit(r, rp, &principal.UserID, action, "delegation", delegation.ID, map[string]any{
		"from_user_id": delegation.FromUserID,
		"to_user_id":   delegation.ToUserID,
		"valid_until":  delegation.ValidUntil,
	})
	writeJSON(w, http.StatusOK, delegation)
}

func (s *Server) handleApproveDelegation(w http.ResponseWriter, r *http.Request) {
	s.handleDecideDelegation(w, r, true)
}

func (s *Server) handleRejectDelegation(w http.ResponseWriter, r *http.Request) {
	s.handleDecideDelegation(w, r, false)
}

func (s *Server) handleRevokeDelegation(w http.ResponseWriter, r *http.Request) {
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
	delegation, err := rp.RevokeDelegation(r.Context(), id, principal.UserID, principal.Role == "admin")
	switch {
	case errors.Is(err, repo.ErrNotFound):
		writeError(w, http.StatusNotFound, "делегирование не найдено")
		return
	case errors.Is(err, repo.ErrDelegationState):
		writeError(w, http.StatusConflict, "делегирование не может быть отозвано")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "не удалось отозвать делегирование")
		return
	}
	s.audit(r, rp, &principal.UserID, "delegation.revoke", "delegation", delegation.ID, map[string]any{
		"from_user_id": delegation.FromUserID,
		"to_user_id":   delegation.ToUserID,
		"valid_until":  delegation.ValidUntil,
	})
	writeJSON(w, http.StatusOK, delegation)
}
