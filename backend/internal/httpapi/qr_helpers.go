package httpapi

import (
	"net/http"
	"strings"
	"time"

	"tabel/internal/repo"
	"tabel/internal/shift"
)

// validateQRToken проверяет статус, срок действия и подпись QR-токена.
// Возвращает 0 при успехе либо HTTP-код и текст ошибки.
func (s *Server) validateQRToken(token repo.QRToken, requestSignature string) (int, string) {
	if token.Status == "revoked" {
		if token.RevokedReason != nil && strings.TrimSpace(*token.RevokedReason) != "" {
			return http.StatusGone, "QR-токен отозван: " + strings.TrimSpace(*token.RevokedReason)
		}
		return http.StatusGone, "QR-токен отозван"
	}
	if token.Status != "active" {
		return http.StatusGone, "QR-токен истёк"
	}
	if !token.EmployeeOK {
		return http.StatusGone, "сотрудник деактивирован или пользователь заблокирован"
	}
	today := time.Now().In(shift.NovosibirskZone).Format("2006-01-02")
	if (token.ValidFrom != nil && today < *token.ValidFrom) || (token.ValidUntil != nil && today > *token.ValidUntil) {
		return http.StatusGone, "QR-токен истёк"
	}

	signature := strings.TrimSpace(requestSignature)
	if signature == "" {
		signature = token.Signature
	}
	if signature != "" {
		valid, err := s.qrMgr.Verify(token.Payload, signature)
		if err != nil {
			return http.StatusInternalServerError, "ошибка проверки подписи QR"
		}
		if !valid {
			return http.StatusBadRequest, "неверная подпись QR-токена"
		}
		return 0, ""
	}
	if s.cfg.AppEnv == "production" {
		return http.StatusBadRequest, "QR-токен не содержит подписи"
	}
	return 0, ""
}
