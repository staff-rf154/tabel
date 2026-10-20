package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"tabel/internal/auth"
	"tabel/internal/repo"
)

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type registerRequest struct {
	QRToken   string `json:"qr_token"`
	Signature string `json:"signature"`
	FullName  string `json:"full_name"`
	Phone     string `json:"phone"`
	Password  string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type tokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Login = strings.TrimSpace(req.Login)
	if req.Login == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "поля login и password обязательны")
		return
	}

	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}

	user, err := rp.FindAuthUserByLogin(r.Context(), req.Login)
	if errors.Is(err, repo.ErrNotFound) {
		s.audit(r, rp, nil, "auth.login_failed", "user", "", map[string]any{"login": req.Login, "reason": "not_found"})
		writeError(w, http.StatusUnauthorized, "неверный логин или пароль")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения пользователя")
		return
	}

	if user.AuthBlocked || user.IsLocked {
		s.audit(r, rp, &user.ID, "auth.login_blocked", "user", user.ID, nil)
		s.writeBlocked(w, "авторизация запрещена")
		return
	}

	valid, err := auth.VerifyPassword(user.PasswordHash, req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка проверки пароля")
		return
	}
	if !valid {
		attempts, locked, recordErr := rp.RecordLoginFailure(r.Context(), user.ID)
		s.audit(r, rp, &user.ID, "auth.login_failed", "user", user.ID, map[string]any{
			"attempts": attempts,
			"locked":   locked,
		})
		if recordErr != nil {
			writeError(w, http.StatusInternalServerError, "ошибка учёта попытки входа")
			return
		}
		if locked {
			s.writeBlocked(w, "учётная запись заблокирована после 5 неудачных попыток входа")
			return
		}
		writeError(w, http.StatusUnauthorized, "неверный логин или пароль")
		return
	}

	if err := rp.RecordLoginSuccess(r.Context(), user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка обновления данных входа")
		return
	}

	pair, err := s.issueSession(r, rp, user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось создать сессию")
		return
	}
	s.audit(r, rp, &user.ID, "auth.login", "user", user.ID, nil)
	writeJSON(w, http.StatusOK, pair)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.RefreshToken) == "" {
		writeError(w, http.StatusBadRequest, "поле refresh_token обязательно")
		return
	}

	claims, err := s.tokens.ParseRefresh(req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "refresh-токен недействителен")
		return
	}

	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}

	session, user, err := rp.SessionByTokenHash(r.Context(), auth.HashToken(req.RefreshToken))
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "refresh-токен недействителен")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения сессии")
		return
	}
	if session.ID != claims.SID || user.ID != claims.Sub || session.RevokedAt != nil || time.Now().After(session.ExpiresAt) {
		writeError(w, http.StatusUnauthorized, "refresh-токен недействителен")
		return
	}
	if user.AuthBlocked || user.IsLocked {
		s.writeBlocked(w, "авторизация запрещена")
		return
	}

	if err := rp.RevokeSession(r.Context(), session.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось обновить сессию")
		return
	}
	pair, err := s.issueSession(r, rp, user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось создать сессию")
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	if err := rp.RevokeSession(r.Context(), principal.SessionID); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось завершить сессию")
		return
	}
	s.audit(r, rp, &principal.UserID, "auth.logout", "user", principal.UserID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.QRToken = strings.TrimSpace(req.QRToken)
	req.FullName = strings.TrimSpace(req.FullName)
	req.Phone = strings.TrimSpace(req.Phone)

	if _, err := uuid.Parse(req.QRToken); err != nil {
		writeError(w, http.StatusBadRequest, "поле qr_token должно быть UUID")
		return
	}
	if req.FullName == "" || req.Phone == "" {
		writeError(w, http.StatusBadRequest, "поля full_name и phone обязательны")
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

	token, err := rp.FindQRToken(r.Context(), req.QRToken)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "QR-токен не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения QR-токена")
		return
	}
	if code, message := s.validateQRToken(token, req.Signature); code != 0 {
		writeError(w, code, message)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка хэширования пароля")
		return
	}

	login, err := rp.GenerateEmployeeLogin(r.Context(), req.FullName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сформировать логин")
		return
	}
	profile, err := rp.RegisterEmployee(r.Context(), token.EmployeeID, login, hash, req.FullName, req.Phone)
	switch {
	case errors.Is(err, repo.ErrAlreadyRegistered):
		writeError(w, http.StatusConflict, "учётная запись уже зарегистрирована")
		return
	case errors.Is(err, repo.ErrLoginTaken):
		writeError(w, http.StatusConflict, "не удалось подобрать уникальный логин")
		return
	case errors.Is(err, repo.ErrNotFound):
		writeError(w, http.StatusGone, "сотрудник не найден или деактивирован")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "не удалось создать учётную запись")
		return
	}

	s.audit(r, rp, &profile.ID, "auth.register", "employee", token.EmployeeID, map[string]any{
		"qr_token_id": token.ID,
		"qr_type":     token.Type,
	})
	writeJSON(w, http.StatusCreated, profile)
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	var req changePasswordRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := auth.ValidatePassword(req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	user, err := rp.FindAuthUserByID(r.Context(), principal.UserID)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "пользователь не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения пользователя")
		return
	}
	valid, err := auth.VerifyPassword(user.PasswordHash, req.CurrentPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка проверки пароля")
		return
	}
	if !valid {
		writeError(w, http.StatusBadRequest, "неверный текущий пароль")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка хэширования пароля")
		return
	}
	if err := rp.UpdatePassword(r.Context(), user.ID, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось изменить пароль")
		return
	}
	if err := rp.RevokeUserSessions(r.Context(), user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "пароль изменён, но не удалось завершить сессии")
		return
	}
	s.audit(r, rp, &user.ID, "auth.password_changed", "user", user.ID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	profile, err := rp.ProfileByID(r.Context(), principal.UserID)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "пользователь не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения профиля")
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) issueSession(r *http.Request, rp *repo.Repo, user repo.AuthUser) (tokenPair, error) {
	sessionID := uuid.NewString()
	access, refresh, expiresIn, err := s.tokens.Issue(user.ID, user.Role, sessionID)
	if err != nil {
		return tokenPair{}, err
	}
	_, err = rp.CreateSession(
		r.Context(),
		sessionID,
		user.ID,
		auth.HashToken(refresh),
		userAgent(r),
		clientIP(r),
		time.Now().Add(s.tokens.RefreshTTL()),
	)
	if err != nil {
		return tokenPair{}, err
	}
	return tokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
	}, nil
}

func (s *Server) audit(r *http.Request, rp *repo.Repo, actor *string, action, entityType, entityID string, details any) {
	_ = rp.WriteAudit(r.Context(), repo.AuditInput{
		ActorUserID: actor,
		Action:      action,
		EntityType:  entityType,
		EntityID:    entityID,
		Details:     details,
		IP:          clientIP(r),
		UserAgent:   userAgent(r),
	})
}
