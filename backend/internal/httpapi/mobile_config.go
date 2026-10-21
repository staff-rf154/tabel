package httpapi

import (
	"errors"
	"net/http"
	"time"

	"tabel/internal/repo"
)

const mobileConfigVersion = 1

type mobileNavigationItem struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Icon  string `json:"icon"`
}

type mobilePolicy struct {
	CacheUntilLogout bool `json:"cache_until_logout"`
	RefreshAfterSec  int  `json:"refresh_after_sec"`
}

type mobileBootstrap struct {
	Version       int                    `json:"version"`
	ConfigVersion int                    `json:"config_version"`
	GeneratedAt   time.Time              `json:"generated_at"`
	Role          string                 `json:"role"`
	RoleTitle     string                 `json:"role_title"`
	Shell         string                 `json:"shell"`
	Features      map[string]bool        `json:"features"`
	Navigation    []mobileNavigationItem `json:"navigation"`
	Policy        mobilePolicy           `json:"policy"`
}

func roleTitle(role string) string {
	switch role {
	case "employee":
		return "Сотрудник"
	case "operator":
		return "Оператор"
	case "admin":
		return "Администратор"
	case "shift_lead":
		return "Руководитель смены"
	case "director":
		return "Директор"
	case "hr":
		return "Отдел кадров"
	default:
		return role
	}
}

func buildMobileBootstrap(profile repo.Profile) mobileBootstrap {
	config := mobileBootstrap{
		Version:       1,
		ConfigVersion: mobileConfigVersion,
		GeneratedAt:   time.Now().UTC(),
		Role:          profile.Role,
		RoleTitle:     roleTitle(profile.Role),
		Features:      map[string]bool{},
		Policy: mobilePolicy{
			CacheUntilLogout: true,
			RefreshAfterSec:  24 * 60 * 60,
		},
	}

	switch profile.Role {
	case "employee":
		config.Shell = "employee"
		config.Features["my_qr"] = true
		config.Features["monthly_stats"] = true
		config.Features["profile"] = true
		config.Navigation = []mobileNavigationItem{
			{Key: "home", Title: "Мои отметки", Icon: "badge"},
		}
	case "operator":
		config.Shell = "operator"
		config.Features["scan"] = true
		config.Features["queue"] = true
		config.Features["delegations"] = true
		config.Features["profile"] = true
		config.Navigation = []mobileNavigationItem{
			{Key: "scan", Title: "Скан", Icon: "qr_code_scanner"},
			{Key: "queue", Title: "Очередь", Icon: "cloud_queue"},
			{Key: "delegations", Title: "Права", Icon: "swap_horiz"},
			{Key: "profile", Title: "Профиль", Icon: "person"},
		}
	case "shift_lead", "director", "hr", "admin":
		config.Shell = "management"
		config.Features["reports_summary"] = true
		config.Features["profile"] = true
		config.Navigation = []mobileNavigationItem{
			{Key: "summary", Title: "Сводка", Icon: "insights"},
			{Key: "profile", Title: "Профиль", Icon: "person"},
		}
		if profile.Role == "admin" {
			config.Features["users_directory"] = true
			config.Navigation = []mobileNavigationItem{
				{Key: "summary", Title: "Сводка", Icon: "insights"},
				{Key: "users", Title: "Пользователи", Icon: "group"},
				{Key: "profile", Title: "Профиль", Icon: "person"},
			}
		}
	default:
		config.Shell = "unsupported"
		config.Features["profile"] = true
		config.Navigation = []mobileNavigationItem{
			{Key: "profile", Title: "Профиль", Icon: "person"},
		}
	}

	return config
}

func (s *Server) handleMobileBootstrap(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, buildMobileBootstrap(profile))
}
