package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"tabel/internal/auth"
	"tabel/internal/repo"
)

var managedRoles = map[string]bool{
	"employee":   true,
	"operator":   true,
	"admin":      true,
	"shift_lead": true,
	"director":   true,
	"hr":         true,
}

var reportingRoles = map[string]bool{
	"admin":      true,
	"shift_lead": true,
	"director":   true,
	"hr":         true,
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	var req struct {
		Login      string  `json:"login"`
		Password   string  `json:"password"`
		Role       string  `json:"role"`
		EmployeeID *string `json:"employee_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Login = strings.TrimSpace(req.Login)
	req.Role = strings.TrimSpace(req.Role)
	if !managedRoles[req.Role] {
		writeError(w, http.StatusBadRequest, "недопустимая роль")
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.EmployeeID != nil {
		value := strings.TrimSpace(*req.EmployeeID)
		if value == "" {
			req.EmployeeID = nil
		} else if _, err := uuid.Parse(value); err != nil {
			writeError(w, http.StatusBadRequest, "employee_id должен быть UUID")
			return
		} else {
			req.EmployeeID = &value
		}
	}
	if req.Role != "employee" && req.Role != "operator" && req.EmployeeID != nil {
		writeError(w, http.StatusBadRequest, "привязка к сотруднику допустима только для роли employee или operator")
		return
	}
	if req.Role == "employee" || req.Role == "operator" {
		if req.EmployeeID == nil {
			writeError(w, http.StatusBadRequest, "для этой роли выберите сотрудника")
			return
		}
		employee, err := rp.GetEmployee(r.Context(), *req.EmployeeID)
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "сотрудник не найден")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ошибка чтения сотрудника")
			return
		}
		req.Login, err = rp.GenerateEmployeeLogin(r.Context(), employee.FullName)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сформировать логин")
			return
		}
	}
	if req.Login == "" {
		writeError(w, http.StatusBadRequest, "логин обязателен")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось обработать пароль")
		return
	}
	user, err := rp.CreateUser(r.Context(), req.Login, hash, req.Role, req.EmployeeID)
	switch {
	case errors.Is(err, repo.ErrLoginTaken):
		writeError(w, http.StatusConflict, "логин уже занят")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "не удалось создать пользователя")
		return
	}
	s.audit(r, rp, &principal.UserID, "user.create", "user", user.ID, map[string]any{"login": req.Login, "role": req.Role})
	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) handleSetUserPassword(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	target, err := rp.FindAuthUserByID(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "пользователь не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения пользователя")
		return
	}
	if target.IsMainAdmin && id != principal.UserID {
		writeError(w, http.StatusConflict, "пароль главного администратора может изменить только он сам")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось обработать пароль")
		return
	}
	if err := rp.UpdatePassword(r.Context(), id, hash); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "пользователь не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, "не удалось изменить пароль")
		return
	}
	_ = rp.RevokeUserSessions(r.Context(), id)
	s.audit(r, rp, &principal.UserID, "user.password_update", "user", id, map[string]any{"login": target.Login})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if id == principal.UserID {
		writeError(w, http.StatusBadRequest, "нельзя удалить собственную учётную запись")
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	target, err := rp.FindAuthUserByID(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "пользователь не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения пользователя")
		return
	}
	if target.IsMainAdmin {
		writeError(w, http.StatusConflict, "нельзя удалить главного администратора")
		return
	}
	if err := rp.DeleteUser(r.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "пользователь не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, "не удалось удалить пользователя")
		return
	}
	s.audit(r, rp, &principal.UserID, "user.delete", "user", id, map[string]any{"login": target.Login})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUpdateUserRole(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if id == principal.UserID {
		writeError(w, http.StatusBadRequest, "нельзя изменить роль собственной учётной записи")
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	role := strings.TrimSpace(req.Role)
	if !managedRoles[role] {
		writeError(w, http.StatusBadRequest, "недопустимая роль")
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	target, err := rp.FindAuthUserByID(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "пользователь не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения пользователя")
		return
	}
	if target.IsMainAdmin {
		writeError(w, http.StatusConflict, "нельзя изменить роль главного администратора")
		return
	}
	if target.Role == "admin" && role != "admin" {
		admins, err := rp.ListUsers(r.Context(), "admin", 500, 0)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось проверить администраторов")
			return
		}
		if len(admins) <= 1 {
			writeError(w, http.StatusConflict, "нельзя снять роль с последнего администратора")
			return
		}
	}
	user, err := rp.UpdateUserRole(r.Context(), id, role)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "пользователь не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось изменить роль")
		return
	}
	_ = rp.RevokeUserSessions(r.Context(), id)
	s.audit(r, rp, &principal.UserID, "user.role_update", "user", id, map[string]any{
		"old_role": target.Role,
		"new_role": role,
	})
	writeJSON(w, http.StatusOK, user)
}
