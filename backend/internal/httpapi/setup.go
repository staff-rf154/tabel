package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"tabel/internal/app"
	"tabel/internal/auth"
	"tabel/internal/repo"
	"tabel/internal/settings"
)

type setupStatusResponse struct {
	Installed       bool `json:"installed"`
	DBConfigured    bool `json:"db_configured"`
	DBReachable     bool `json:"db_reachable"`
	RedisConfigured bool `json:"redis_configured"`
}

type setupTestDBRequest struct {
	DatabaseURL string `json:"database_url"`
}

type setupSaveRequest struct {
	DatabaseURL    string `json:"database_url"`
	RedisAddr      string `json:"redis_addr"`
	RedisPassword  string `json:"redis_password"`
	MainAdminEmail string `json:"main_admin_email"`
	SupportPhone   string `json:"support_phone"`
}

type setupCompleteRequest struct {
	MainAdminEmail string `json:"main_admin_email"`
	SupportPhone   string `json:"support_phone"`
	AdminLogin     string `json:"admin_login"`
	AdminPassword  string `json:"admin_password"`
}

func (s *Server) resolveDatabaseURL() string {
	if v, err := s.store.GetString(settings.KeyDatabaseURL); err == nil && v != "" {
		return v
	}
	return s.cfg.DefaultDatabaseURL
}

func (s *Server) resolveRedis() (string, string) {
	addr, err := s.store.GetString(settings.KeyRedisAddr)
	if err != nil || addr == "" {
		addr = s.cfg.DefaultRedisAddr
	}
	password, err := s.store.GetString(settings.KeyRedisPassword)
	if err != nil {
		password = s.cfg.DefaultRedisPass
	}
	return addr, password
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	installed, err := s.store.IsInstalled()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать состояние установки")
		return
	}

	databaseURL := s.resolveDatabaseURL()
	redisAddr, _ := s.resolveRedis()

	reachable := false
	if pool, _ := s.connections(); pool != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		reachable = pool.Ping(ctx) == nil
		cancel()
	}

	writeJSON(w, http.StatusOK, setupStatusResponse{
		Installed:       installed,
		DBConfigured:    databaseURL != "",
		DBReachable:     reachable,
		RedisConfigured: redisAddr != "",
	})
}

func (s *Server) handleSetupTestDB(w http.ResponseWriter, r *http.Request) {
	if s.rejectIfInstalled(w) {
		return
	}

	var req setupTestDBRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.DatabaseURL) == "" {
		writeError(w, http.StatusBadRequest, "поле database_url обязательно")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := app.TestDatabase(ctx, req.DatabaseURL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSetupSave(w http.ResponseWriter, r *http.Request) {
	if s.rejectIfInstalled(w) {
		return
	}

	var req setupSaveRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.DatabaseURL) == "" {
		writeError(w, http.StatusBadRequest, "поле database_url обязательно")
		return
	}
	if strings.TrimSpace(req.RedisAddr) == "" {
		writeError(w, http.StatusBadRequest, "поле redis_addr обязательно")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	pool, rdb, err := app.Connect(ctx, req.DatabaseURL, req.RedisAddr, req.RedisPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.persistSettings(req); err != nil {
		pool.Close()
		_ = rdb.Close()
		writeError(w, http.StatusInternalServerError, "не удалось сохранить настройки: "+err.Error())
		return
	}

	if oldPool, oldRdb := s.connections(); oldPool != nil {
		oldPool.Close()
		if oldRdb != nil {
			_ = oldRdb.Close()
		}
	}
	s.SetConnections(pool, rdb)

	writeJSON(w, http.StatusOK, map[string]any{"status": "saved", "migrated": true})
}

func (s *Server) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	if s.rejectIfInstalled(w) {
		return
	}

	var req setupCompleteRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if pool, _ := s.connections(); pool == nil {
		writeError(w, http.StatusConflict, "сначала сохраните и проверьте параметры подключения")
		return
	}

	login := strings.TrimSpace(req.AdminLogin)
	if (login == "") != (req.AdminPassword == "") {
		writeError(w, http.StatusBadRequest, "поля admin_login и admin_password должны быть заполнены вместе")
		return
	}
	if req.AdminPassword != "" {
		if err := auth.ValidatePassword(req.AdminPassword); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	if err := s.persistSettings(setupSaveRequest{
		MainAdminEmail: req.MainAdminEmail,
		SupportPhone:   req.SupportPhone,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сохранить настройки: "+err.Error())
		return
	}

	if req.AdminPassword != "" {
		hash, err := auth.HashPassword(req.AdminPassword)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось хэшировать пароль администратора")
			return
		}
		pool, _ := s.connections()
		if _, err := repo.New(pool).BootstrapMainAdmin(r.Context(), login, hash); err != nil {
			if errors.Is(err, repo.ErrLoginTaken) {
				writeError(w, http.StatusConflict, "логин главного администратора уже занят")
				return
			}
			writeError(w, http.StatusInternalServerError, "не удалось создать главного администратора")
			return
		}
	}

	if err := s.store.MarkInstalled(); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось зафиксировать завершение установки")
		return
	}
	if rp, ok := s.repo(); ok {
		s.audit(r, rp, nil, "setup.complete", "system", "", nil)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "installed"})
}

func (s *Server) persistSettings(req setupSaveRequest) error {
	if req.DatabaseURL != "" {
		if err := s.store.SetString(settings.KeyDatabaseURL, req.DatabaseURL, true); err != nil {
			return err
		}
	}
	if req.RedisAddr != "" {
		if err := s.store.SetString(settings.KeyRedisAddr, req.RedisAddr, false); err != nil {
			return err
		}
	}
	if req.RedisPassword != "" {
		if err := s.store.SetString(settings.KeyRedisPassword, req.RedisPassword, true); err != nil {
			return err
		}
	}
	if req.MainAdminEmail != "" {
		if err := s.store.SetString(settings.KeyMainAdminEmail, req.MainAdminEmail, false); err != nil {
			return err
		}
	}
	if req.SupportPhone != "" {
		if err := s.store.SetString(settings.KeySupportPhone, req.SupportPhone, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) rejectIfInstalled(w http.ResponseWriter) bool {
	installed, err := s.store.IsInstalled()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать состояние установки")
		return true
	}
	if installed {
		writeError(w, http.StatusConflict, "система уже установлена")
		return true
	}
	return false
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer func() { _ = r.Body.Close() }()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		writeError(w, http.StatusBadRequest, "некорректный JSON: "+err.Error())
		return false
	}
	return true
}
