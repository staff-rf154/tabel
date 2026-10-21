package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"tabel/internal/auth"
	"tabel/internal/repo"
	"tabel/internal/settings"
	"tabel/internal/shift"
)

const demoPassword = "1й2ц3у"

type demoResetResponse struct {
	repo.DemoResetStats
	DemoPassword string `json:"demo_password"`
}

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

func (s *Server) handleResetDemo(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	if err := auth.ValidatePassword(demoPassword); err != nil {
		writeError(w, http.StatusInternalServerError, "демонстрационный пароль не соответствует политике")
		return
	}
	passwordHash, err := auth.HashPassword(demoPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось подготовить пароль")
		return
	}

	stats, err := rp.ResetDemo(r.Context(), passwordHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось заменить демо-данные: "+err.Error())
		return
	}
	if err := s.applyDemoCompanySettings(); err != nil {
		writeError(w, http.StatusInternalServerError, "демо-данные созданы, но не удалось сохранить информацию о компании")
		return
	}

	employees, err := rp.ListEmployees(r.Context(), "", nil, 500, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "демо-данные созданы, но не удалось прочитать сотрудников для QR")
		return
	}
	today := time.Now().In(shift.NovosibirskZone)
	validFrom := today.Format("2006-01-02")
	validUntil := today.AddDate(0, 0, 90).Format("2006-01-02")
	for _, employee := range employees {
		staticToken := uuid.NewString()
		staticPayload, err := buildPayload(nil, employee, staticToken, "static", nil, nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сформировать статический QR")
			return
		}
		signature, err := s.qrMgr.Sign(staticPayload)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось подписать статический QR")
			return
		}
		if _, err := rp.CreateStaticQR(r.Context(), employee.ID, staticToken, string(staticPayload), signature, principal.UserID, ""); err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сохранить статический QR")
			return
		}
		stats.QRStatic++

		paperToken := uuid.NewString()
		paperPayload, err := buildPayload(nil, employee, paperToken, "paper", &validFrom, &validUntil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сформировать бумажный QR")
			return
		}
		paperSignature, err := s.qrMgr.Sign(paperPayload)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось подписать бумажный QR")
			return
		}
		if _, err := rp.IssuePaperQR(r.Context(), employee.ID, paperToken, string(paperPayload), paperSignature, validFrom, validUntil, principal.UserID); err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сохранить бумажный QR")
			return
		}
		stats.QRPaper++
	}

	s.audit(r, rp, &principal.UserID, "demo.reset", "system", "DEMO", map[string]any{
		"employees": stats.Employees,
		"marks":     stats.Marks,
		"qr_static": stats.QRStatic,
		"qr_paper":  stats.QRPaper,
	})
	stats.Audit++
	writeJSON(w, http.StatusOK, demoResetResponse{DemoResetStats: stats, DemoPassword: demoPassword})
}

func (s *Server) applyDemoCompanySettings() error {
	values := []struct {
		key   string
		value string
	}{
		{settings.KeyCompanyName, "ООО «Демо-Компания»"},
		{settings.KeyCompanyAddress, "630099, г. Новосибирск, ул. Ленина, д. 10, офис 500"},
		{settings.KeyCompanyINN, "5407123456"},
		{settings.KeyWebsite, "https://demo.tabel.local"},
		{settings.KeySupportPhone, "+7 (383) 000-00-00"},
		{settings.KeySupportEmail, "support@demo.tabel.local"},
		{settings.KeyMainAdminEmail, "admin@demo.tabel.local"},
		{settings.KeyPassFooter, "ООО «Демо-Компания» · отдел кадров · +7 (383) 000-00-00"},
		{settings.KeyPaperQRValidDays, "90"},
		{settings.KeyDefaultOfficeRadius, "300"},
	}
	for _, item := range values {
		if err := s.store.SetString(item.key, item.value, false); err != nil {
			return err
		}
	}
	return nil
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
