package httpapi

import (
	"errors"
	"net/http"
	"time"

	"tabel/internal/repo"
	"tabel/internal/shift"
)

func (s *Server) handleMyMonthlyStats(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
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
	if user.EmployeeID == nil {
		writeError(w, http.StatusNotFound, "сотрудник не привязан к пользователю")
		return
	}

	now := time.Now().In(shift.NovosibirskZone)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, shift.NovosibirskZone)
	monthEnd := monthStart.AddDate(0, 1, 0)
	stats, err := rp.EmployeeMonthStats(r.Context(), *user.EmployeeID, monthStart, monthEnd, 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить статистику")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"month":           monthStart.Format("2006-01"),
		"month_start":     monthStart,
		"month_end":       monthEnd,
		"days_worked":     stats.DaysWorked,
		"shifts_total":    stats.ShiftsTotal,
		"shifts_closed":   stats.ShiftsClosed,
		"worked_minutes":  stats.WorkedMinutes,
		"overtime_shifts": stats.OvertimeShifts,
		"last_check_ins":  stats.LastCheckIns,
	})
}
