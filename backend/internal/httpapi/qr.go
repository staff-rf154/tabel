package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"

	"tabel/internal/pdf"
	"tabel/internal/repo"
	"tabel/internal/shift"
)

const (
	maxBulkFileSize   = 25 << 20
	maxBulkRecordSize = 10 << 20
	maxBulkFiles      = 200
	maxBulkRows       = 5000
)

type qrTokenResponse struct {
	ID            string          `json:"id"`
	EmployeeID    string          `json:"employee_id"`
	Type          string          `json:"type"`
	Token         string          `json:"token"`
	Signature     string          `json:"signature"`
	ValidFrom     *string         `json:"valid_from"`
	ValidUntil    *string         `json:"valid_until"`
	Status        string          `json:"status"`
	RevokedReason *string         `json:"revoked_reason,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	QRContent     string          `json:"qr_content,omitempty"`
}

func newQRTokenResponse(token repo.QRToken) qrTokenResponse {
	response := qrTokenResponse{
		ID:            token.ID,
		EmployeeID:    token.EmployeeID,
		Type:          token.Type,
		Token:         token.Token,
		Signature:     token.Signature,
		ValidFrom:     token.ValidFrom,
		ValidUntil:    token.ValidUntil,
		Status:        token.Status,
		RevokedReason: token.RevokedReason,
	}
	if json.Valid(token.Payload) {
		response.Payload = json.RawMessage(token.Payload)
		if content, err := makeQRContent(token.Token, token.Signature, token.Payload); err == nil {
			response.QRContent = content
		}
	}
	return response
}

func makeQRContent(token, signature string, payload []byte) (string, error) {
	if !json.Valid(payload) {
		return "", errors.New("payload не является корректным JSON")
	}
	envelope := struct {
		Token     string          `json:"token"`
		Signature string          `json:"signature"`
		Payload   json.RawMessage `json:"payload"`
	}{
		Token:     token,
		Signature: signature,
		Payload:   json.RawMessage(payload),
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func effectiveQRStatus(token repo.QRToken) string {
	if token.Status != "active" {
		return token.Status
	}
	today := time.Now().In(shift.NovosibirskZone).Format("2006-01-02")
	if (token.ValidFrom != nil && today < *token.ValidFrom) || (token.ValidUntil != nil && today > *token.ValidUntil) {
		return "expired"
	}
	return "active"
}

func qrWithinWindow(token repo.QRToken, today string) bool {
	if token.ValidFrom != nil && today < *token.ValidFrom {
		return false
	}
	if token.ValidUntil != nil && today > *token.ValidUntil {
		return false
	}
	return true
}

type qrVerificationResponse struct {
	Valid          bool    `json:"valid"`
	TokenStatus    string  `json:"token_status"`
	SignatureValid bool    `json:"signature_valid"`
	EmployeeID     *string `json:"employee_id"`
	EmployeeNumber *string `json:"employee_number"`
	FullName       *string `json:"full_name"`
	QRDataMismatch bool    `json:"qr_data_mismatch"`
	Reason         *string `json:"reason,omitempty"`
}

func (s *Server) handleVerifyQR(w http.ResponseWriter, r *http.Request) {
	rawToken := strings.TrimSpace(r.PathValue("token"))
	if _, err := uuid.Parse(rawToken); err != nil {
		writeError(w, http.StatusBadRequest, "token должен быть UUID")
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}

	token, err := rp.FindQRToken(r.Context(), rawToken)
	if errors.Is(err, repo.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, qrVerificationResponse{TokenStatus: "not_found"})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения QR-токена")
		return
	}

	signatureValid, err := s.qrMgr.Verify(token.Payload, token.Signature)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка проверки подписи QR")
		return
	}
	today := time.Now().In(shift.NovosibirskZone).Format("2006-01-02")
	status := effectiveQRStatus(token)
	mismatch := qrDataMismatch(token.Payload, token)
	response := qrVerificationResponse{
		Valid:          status == "active" && token.EmployeeOK && qrWithinWindow(token, today) && signatureValid,
		TokenStatus:    status,
		SignatureValid: signatureValid,
		EmployeeID:     &token.EmployeeID,
		QRDataMismatch: mismatch,
	}
	if status == "revoked" {
		response.Reason = token.RevokedReason
	}
	if !token.EmployeeOK {
		reason := "сотрудник деактивирован или пользователь заблокирован"
		response.Reason = &reason
	}
	if employee, err := rp.GetEmployee(r.Context(), token.EmployeeID); err == nil {
		response.EmployeeNumber = &employee.EmployeeNumber
		response.FullName = &employee.FullName
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) issueServerStaticQR(ctx context.Context, rp *repo.Repo, employee repo.Employee, issuedBy, reason string) (repo.QRToken, error) {
	token := uuid.NewString()
	payload, err := buildPayload(nil, employee, token, "static", nil, nil)
	if err != nil {
		return repo.QRToken{}, err
	}
	signature, err := s.qrMgr.Sign(payload)
	if err != nil {
		return repo.QRToken{}, err
	}
	return rp.CreateStaticQR(ctx, employee.ID, token, string(payload), signature, issuedBy, reason)
}

func (s *Server) currentEmployeeQR(ctx context.Context, rp *repo.Repo, userID string) (repo.Employee, repo.QRToken, error) {
	profile, err := rp.ProfileByID(ctx, userID)
	if errors.Is(err, repo.ErrNotFound) || (err == nil && profile.EmployeeID == nil) {
		return repo.Employee{}, repo.QRToken{}, repo.ErrNotFound
	}
	if err != nil {
		return repo.Employee{}, repo.QRToken{}, err
	}
	employee, err := rp.GetEmployee(ctx, *profile.EmployeeID)
	if err != nil {
		return repo.Employee{}, repo.QRToken{}, err
	}
	token, err := rp.GetActiveStaticQR(ctx, employee.ID)
	if errors.Is(err, repo.ErrNotFound) {
		token, err = s.issueServerStaticQR(ctx, rp, employee, userID, "")
	}
	return employee, token, err
}

func (s *Server) handleMyQR(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	employee, token, err := s.currentEmployeeQR(r.Context(), rp, principal.UserID)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusForbidden, "личный QR доступен только сотруднику")
		return
	}
	if errors.Is(err, repo.ErrEmployeeInactive) {
		writeError(w, http.StatusConflict, "сотрудник деактивирован")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить QR-код")
		return
	}
	if !employee.IsActive {
		writeError(w, http.StatusConflict, "сотрудник деактивирован")
		return
	}
	writeJSON(w, http.StatusOK, newQRTokenResponse(token))
}

func (s *Server) handleRotateMyQR(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	profile, err := rp.ProfileByID(r.Context(), principal.UserID)
	if errors.Is(err, repo.ErrNotFound) || (err == nil && profile.EmployeeID == nil) {
		writeError(w, http.StatusForbidden, "личный QR доступен только сотруднику")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения профиля")
		return
	}
	employee, err := rp.GetEmployee(r.Context(), *profile.EmployeeID)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusForbidden, "личный QR доступен только сотруднику")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения сотрудника")
		return
	}
	if !employee.IsActive {
		writeError(w, http.StatusConflict, "сотрудник деактивирован")
		return
	}
	token, err := s.issueServerStaticQR(r.Context(), rp, employee, principal.UserID, "изменён сотрудником")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сменить QR-код")
		return
	}
	s.audit(r, rp, &principal.UserID, "qr.self_rotate", "qr_token", token.ID, map[string]any{
		"employee_id": employee.ID,
	})
	writeJSON(w, http.StatusCreated, newQRTokenResponse(token))
}

func (s *Server) handleMyQRImage(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	_, token, err := s.currentEmployeeQR(r.Context(), rp, principal.UserID)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusForbidden, "личный QR доступен только сотруднику")
		return
	}
	if errors.Is(err, repo.ErrEmployeeInactive) {
		writeError(w, http.StatusConflict, "сотрудник деактивирован")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить QR-код")
		return
	}
	content, err := makeQRContent(token.Token, token.Signature, token.Payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сформировать QR-код")
		return
	}
	png, err := qrcode.Encode(content, qrcode.Medium, 640)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сформировать изображение QR-кода")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(png)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}

func (s *Server) adminEmployeeQR(ctx context.Context, rp *repo.Repo, employee repo.Employee, actor string) (repo.QRToken, bool, error) {
	token, err := rp.GetActiveStaticQR(ctx, employee.ID)
	if !errors.Is(err, repo.ErrNotFound) {
		return token, false, err
	}
	token, err = s.issueServerStaticQR(ctx, rp, employee, actor, "")
	return token, err == nil, err
}

func (s *Server) handleAdminGetEmployeeQR(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	employee, err := rp.GetEmployee(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "сотрудник не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения сотрудника")
		return
	}
	principal, _ := principalFrom(r.Context())
	token, created, err := s.adminEmployeeQR(r.Context(), rp, employee, principal.UserID)
	if errors.Is(err, repo.ErrEmployeeInactive) {
		writeError(w, http.StatusConflict, "сотрудник деактивирован")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить QR-код")
		return
	}
	if created {
		s.audit(r, rp, &principal.UserID, "qr.admin_issue", "qr_token", token.ID, map[string]any{"employee_id": employee.ID})
	}
	writeJSON(w, http.StatusOK, newQRTokenResponse(token))
}

func (s *Server) handleAdminEmployeeQRImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	employee, err := rp.GetEmployee(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "сотрудник не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения сотрудника")
		return
	}
	principal, _ := principalFrom(r.Context())
	token, created, err := s.adminEmployeeQR(r.Context(), rp, employee, principal.UserID)
	if errors.Is(err, repo.ErrEmployeeInactive) {
		writeError(w, http.StatusConflict, "сотрудник деактивирован")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить QR-код")
		return
	}
	if created {
		s.audit(r, rp, &principal.UserID, "qr.admin_issue", "qr_token", token.ID, map[string]any{"employee_id": employee.ID})
	}
	content, err := makeQRContent(token.Token, token.Signature, token.Payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сформировать QR-код")
		return
	}
	png, err := qrcode.Encode(content, qrcode.Medium, 640)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сформировать изображение QR-кода")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(png)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}

func (s *Server) handleAdminRotateEmployeeQR(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	employee, err := rp.GetEmployee(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "сотрудник не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения сотрудника")
		return
	}
	if !employee.IsActive {
		writeError(w, http.StatusConflict, "сотрудник деактивирован")
		return
	}
	principal, _ := principalFrom(r.Context())
	token, err := s.issueServerStaticQR(r.Context(), rp, employee, principal.UserID, "отозван администратором")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сменить QR-код")
		return
	}
	s.audit(r, rp, &principal.UserID, "qr.admin_rotate", "qr_token", token.ID, map[string]any{
		"employee_id": employee.ID,
	})
	writeJSON(w, http.StatusCreated, newQRTokenResponse(token))
}

type qrUploadRequest struct {
	EmployeeID string         `json:"employee_id"`
	Payload    map[string]any `json:"payload"`
	Signature  string         `json:"signature"`
}

func (s *Server) handleUploadStaticQR(w http.ResponseWriter, r *http.Request) {
	var req qrUploadRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.EmployeeID = strings.TrimSpace(req.EmployeeID)
	if _, err := uuid.Parse(req.EmployeeID); err != nil {
		writeError(w, http.StatusBadRequest, "employee_id должен быть UUID")
		return
	}
	if req.Payload == nil {
		writeError(w, http.StatusBadRequest, "поле payload обязательно")
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	employee, err := rp.GetEmployee(r.Context(), req.EmployeeID)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "сотрудник не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения сотрудника")
		return
	}
	if !employee.IsActive {
		writeError(w, http.StatusConflict, "сотрудник деактивирован")
		return
	}

	delete(req.Payload, "signature")
	delete(req.Payload, "valid_from")
	delete(req.Payload, "valid_until")

	token := uuid.NewString()
	if raw, exists := req.Payload["token"]; exists {
		value, ok := raw.(string)
		if !ok {
			writeError(w, http.StatusBadRequest, "поле token в payload должно быть строкой UUID")
			return
		}
		if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
			writeError(w, http.StatusBadRequest, "поле token в payload должно быть UUID")
			return
		}
		token = strings.TrimSpace(value)
	}
	if message := validatePayloadFields(req.Payload, employee, token, "static", nil, nil); message != "" {
		writeError(w, http.StatusBadRequest, message)
		return
	}

	payload, err := buildPayload(req.Payload, employee, token, "static", nil, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "не удалось сформировать payload")
		return
	}
	signature, err := s.qrMgr.Sign(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось подписать QR-токен")
		return
	}

	principal, _ := principalFrom(r.Context())
	created, err := rp.CreateStaticQR(r.Context(), employee.ID, token, string(payload), signature, principal.UserID, "отозван администратором")
	switch {
	case errors.Is(err, repo.ErrQRTokenExists):
		writeError(w, http.StatusConflict, "QR-токен с таким значением уже существует")
		return
	case errors.Is(err, repo.ErrEmployeeInactive):
		writeError(w, http.StatusConflict, "сотрудник деактивирован")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "не удалось сохранить QR-токен")
		return
	}

	s.audit(r, rp, &principal.UserID, "qr.static_upload", "qr_token", created.ID, map[string]any{
		"employee_id": employee.ID,
	})
	writeJSON(w, http.StatusCreated, newQRTokenResponse(created))
}

type paperQRRequest struct {
	EmployeeID string `json:"employee_id"`
}

func (s *Server) handleIssuePaperQR(w http.ResponseWriter, r *http.Request) {
	var req paperQRRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.EmployeeID = strings.TrimSpace(req.EmployeeID)
	if _, err := uuid.Parse(req.EmployeeID); err != nil {
		writeError(w, http.StatusBadRequest, "employee_id должен быть UUID")
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	employee, err := rp.GetEmployee(r.Context(), req.EmployeeID)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "сотрудник не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения сотрудника")
		return
	}
	if !employee.IsActive {
		writeError(w, http.StatusConflict, "сотрудник деактивирован")
		return
	}

	principal, _ := principalFrom(r.Context())
	created, err := s.issuePaperQR(r.Context(), rp, employee, principal.UserID)
	switch {
	case errors.Is(err, repo.ErrActivePaperQRExists):
		writeError(w, http.StatusConflict, "у сотрудника уже есть активный бумажный QR-токен")
		return
	case errors.Is(err, repo.ErrEmployeeInactive):
		writeError(w, http.StatusConflict, "сотрудник деактивирован")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "не удалось выдать бумажный QR-токен")
		return
	}
	s.audit(r, rp, &principal.UserID, "qr.paper_issue", "qr_token", created.ID, map[string]any{
		"employee_id": employee.ID,
		"valid_from":  optionalString(created.ValidFrom),
		"valid_until": optionalString(created.ValidUntil),
	})
	writeJSON(w, http.StatusCreated, newQRTokenResponse(created))
}

func (s *Server) issuePaperQR(ctx context.Context, rp *repo.Repo, employee repo.Employee, userID string) (repo.QRToken, error) {
	now := time.Now().In(shift.NovosibirskZone)
	validFrom := now.Format("2006-01-02")
	validDays := s.adminProfile().PaperQRValidDays
	if validDays < 1 {
		validDays = 30
	}
	validUntil := now.AddDate(0, 0, validDays).Format("2006-01-02")
	token := uuid.NewString()
	payload, err := buildPayload(nil, employee, token, "paper", &validFrom, &validUntil)
	if err != nil {
		return repo.QRToken{}, err
	}
	signature, err := s.qrMgr.Sign(payload)
	if err != nil {
		return repo.QRToken{}, err
	}
	return rp.IssuePaperQR(ctx, employee.ID, token, string(payload), signature, validFrom, validUntil, userID)
}

func (s *Server) paperPassData(token repo.QRToken, profile adminProfileResponse, content string) pdf.PaperQRData {
	return pdf.PaperQRData{
		EmployeeName:   token.FullName,
		EmployeeNumber: token.EmployeeNum,
		Position:       optionalString(token.Position),
		Department:     optionalString(token.Department),
		CompanyName:    profile.CompanyName,
		Token:          token.Token,
		ValidFrom:      optionalString(token.ValidFrom),
		ValidUntil:     optionalString(token.ValidUntil),
		Status:         effectiveQRStatus(token),
		QRContent:      content,
		SupportPhone:   profile.SupportPhone,
		SupportEmail:   profile.SupportEmail,
		PassFooter:     profile.PassFooter,
	}
}

func (s *Server) handleRevokePaperQR(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	principal, _ := principalFrom(r.Context())
	err := rp.RevokePaperQR(r.Context(), id, principal.UserID)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		writeError(w, http.StatusNotFound, "бумажный QR-токен не найден")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "не удалось отозвать QR-токен")
		return
	}
	s.audit(r, rp, &principal.UserID, "qr.paper_revoke", "qr_token", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePaperQRPDF(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	token, err := rp.GetQRTokenByID(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) || (err == nil && token.Type != "paper") {
		writeError(w, http.StatusNotFound, "бумажный QR-токен не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения QR-токена")
		return
	}

	content, err := makeQRContent(token.Token, token.Signature, token.Payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сформировать QR-код")
		return
	}
	profile := s.adminProfile()
	document, err := pdf.PaperQR(s.paperPassData(token, profile, content))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сформировать PDF")
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="paper-qr-%s.pdf"`, token.Token))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(document)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(document)
}

func (s *Server) handlePaperPassesPDF(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EmployeeIDs []string `json:"employee_ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.EmployeeIDs) == 0 {
		writeError(w, http.StatusBadRequest, "выберите хотя бы одного сотрудника")
		return
	}
	if len(req.EmployeeIDs) > 500 {
		writeError(w, http.StatusBadRequest, "за один раз можно напечатать не более 500 пропусков")
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	principal, _ := principalFrom(r.Context())
	profile := s.adminProfile()
	passes := make([]pdf.PaperQRData, 0, len(req.EmployeeIDs))
	skipped := 0
	seen := make(map[string]bool, len(req.EmployeeIDs))
	for _, employeeID := range req.EmployeeIDs {
		employeeID = strings.TrimSpace(employeeID)
		if employeeID == "" || seen[employeeID] {
			continue
		}
		seen[employeeID] = true
		if _, err := uuid.Parse(employeeID); err != nil {
			skipped++
			continue
		}
		employee, err := rp.GetEmployee(r.Context(), employeeID)
		if err != nil || !employee.IsActive {
			skipped++
			continue
		}
		token, err := rp.GetActivePaperQR(r.Context(), employee.ID)
		if errors.Is(err, repo.ErrNotFound) {
			token, err = s.issuePaperQR(r.Context(), rp, employee, principal.UserID)
		}
		if err != nil {
			skipped++
			continue
		}
		content, err := makeQRContent(token.Token, token.Signature, token.Payload)
		if err != nil {
			skipped++
			continue
		}
		passes = append(passes, s.paperPassData(token, profile, content))
	}
	if len(passes) == 0 {
		writeError(w, http.StatusConflict, "не удалось подготовить ни одного пропуска")
		return
	}
	document, err := pdf.PaperQRSheet(passes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сформировать PDF")
		return
	}
	s.audit(r, rp, &principal.UserID, "qr.paper_print_bulk", "qr_token", "", map[string]any{
		"requested": len(req.EmployeeIDs),
		"printed":   len(passes),
		"skipped":   skipped,
	})
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="paper-passes-%s.pdf"`, time.Now().In(shift.NovosibirskZone).Format("20060102-1504")))
	w.Header().Set("X-Passes-Printed", fmt.Sprintf("%d", len(passes)))
	w.Header().Set("X-Passes-Skipped", fmt.Sprintf("%d", skipped))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(document)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(document)
}

type importRow struct {
	Line   int     `json:"line"`
	Status string  `json:"status"`
	Reason *string `json:"reason"`
}

type importReport struct {
	RowsTotal int         `json:"rows_total"`
	RowsOK    int         `json:"rows_ok"`
	RowsError int         `json:"rows_error"`
	Rows      []importRow `json:"rows"`
}

type bulkRecord struct {
	File     string
	Raw      json.RawMessage
	ParseErr error
}

type bulkItem struct {
	EmployeeID     string         `json:"employee_id"`
	EmployeeNumber string         `json:"employee_number"`
	Payload        map[string]any `json:"payload"`
}

func (s *Server) handleBulkQR(w http.ResponseWriter, r *http.Request) {
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = "preview"
	}
	if mode != "preview" && mode != "commit" {
		writeError(w, http.StatusBadRequest, "mode должен быть preview или commit")
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBulkFileSize+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "не удалось прочитать multipart-форму")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "поле file обязательно")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxBulkFileSize+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "не удалось прочитать файл")
		return
	}
	if len(data) > maxBulkFileSize {
		writeError(w, http.StatusRequestEntityTooLarge, "файл слишком большой")
		return
	}

	records, err := readBulkRecords(header.Filename, data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(records) == 0 {
		writeError(w, http.StatusBadRequest, "файл не содержит записей")
		return
	}
	if len(records) > maxBulkRows {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("слишком много записей: максимум %d", maxBulkRows))
		return
	}

	report := importReport{RowsTotal: len(records), Rows: make([]importRow, 0, len(records))}
	for index, record := range records {
		reason := ""
		if record.ParseErr != nil {
			reason = "ошибка разбора JSON: " + record.ParseErr.Error()
		} else {
			reason = s.processBulkRecord(r, rp, record.Raw, mode == "commit")
		}
		row := importRow{Line: index + 1, Status: "ok"}
		if reason != "" {
			if record.File != "" {
				reason = record.File + ": " + reason
			}
			row.Status = "error"
			row.Reason = &reason
			report.RowsError++
		} else {
			report.RowsOK++
		}
		report.Rows = append(report.Rows, row)
	}

	principal, _ := principalFrom(r.Context())
	action := "qr.bulk_preview"
	if mode == "commit" {
		action = "qr.bulk_commit"
	}
	s.audit(r, rp, &principal.UserID, action, "qr_token", "", map[string]any{
		"rows_total": report.RowsTotal,
		"rows_ok":    report.RowsOK,
		"rows_error": report.RowsError,
	})
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) processBulkRecord(r *http.Request, rp *repo.Repo, raw json.RawMessage, commit bool) string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var item bulkItem
	if err := dec.Decode(&item); err != nil {
		return "строка должна быть JSON-объектом"
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return "лишние данные после JSON-объекта"
	}

	item.EmployeeID = strings.TrimSpace(item.EmployeeID)
	item.EmployeeNumber = strings.TrimSpace(item.EmployeeNumber)
	if item.EmployeeID == "" && item.EmployeeNumber == "" {
		return "нужно указать employee_id или employee_number"
	}
	if item.Payload == nil {
		return "поле payload обязательно"
	}

	var employee repo.Employee
	if item.EmployeeID != "" {
		if _, err := uuid.Parse(item.EmployeeID); err != nil {
			return "employee_id должен быть UUID"
		}
		var err error
		employee, err = rp.GetEmployee(r.Context(), item.EmployeeID)
		if errors.Is(err, repo.ErrNotFound) {
			return "сотрудник не найден"
		}
		if err != nil {
			return "ошибка чтения сотрудника"
		}
		if item.EmployeeNumber != "" && item.EmployeeNumber != employee.EmployeeNumber {
			return "employee_id и employee_number указывают на разных сотрудников"
		}
	} else {
		var err error
		employee, err = rp.FindEmployeeByNumber(r.Context(), item.EmployeeNumber)
		if errors.Is(err, repo.ErrNotFound) {
			return "сотрудник не найден"
		}
		if err != nil {
			return "ошибка чтения сотрудника"
		}
	}
	if !employee.IsActive {
		return "сотрудник деактивирован"
	}

	delete(item.Payload, "signature")
	delete(item.Payload, "valid_from")
	delete(item.Payload, "valid_until")

	token := uuid.NewString()
	if rawToken, exists := item.Payload["token"]; exists {
		value, ok := rawToken.(string)
		if !ok {
			return "поле token в payload должно быть строкой UUID"
		}
		if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
			return "поле token в payload должно быть UUID"
		}
		token = strings.TrimSpace(value)
	}
	if message := validatePayloadFields(item.Payload, employee, token, "static", nil, nil); message != "" {
		return message
	}
	if !commit {
		return ""
	}

	payload, err := buildPayload(item.Payload, employee, token, "static", nil, nil)
	if err != nil {
		return "не удалось сформировать payload"
	}
	signature, err := s.qrMgr.Sign(payload)
	if err != nil {
		return "не удалось подписать QR-токен"
	}
	principal, _ := principalFrom(r.Context())
	_, err = rp.CreateStaticQR(r.Context(), employee.ID, token, string(payload), signature, principal.UserID, "отозван администратором")
	switch {
	case errors.Is(err, repo.ErrQRTokenExists):
		return "QR-токен с таким значением уже существует"
	case errors.Is(err, repo.ErrEmployeeInactive):
		return "сотрудник деактивирован"
	case err != nil:
		return "не удалось сохранить QR-токен"
	}
	return ""
}

func readBulkRecords(name string, data []byte) ([]bulkRecord, error) {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	if len(trimmed) == 0 {
		return nil, errors.New("файл пуст")
	}
	if strings.EqualFold(filepath.Ext(name), ".zip") || bytes.HasPrefix(trimmed, []byte("PK\x03\x04")) {
		return readBulkZip(name, data)
	}
	return parseBulkJSON("", trimmed)
}

func readBulkZip(name string, data []byte) ([]bulkRecord, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("некорректный zip-архив")
	}
	if len(reader.File) > 2000 {
		return nil, errors.New("в архиве слишком много файлов")
	}
	var records []bulkRecord
	total := 0
	files := 0
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name), ".json") {
			continue
		}
		files++
		if files > maxBulkFiles {
			return nil, fmt.Errorf("в архиве слишком много JSON-файлов: максимум %d", maxBulkFiles)
		}
		if entry.UncompressedSize64 > maxBulkRecordSize {
			return nil, fmt.Errorf("файл %s слишком большой", entry.Name)
		}
		rc, err := entry.Open()
		if err != nil {
			return nil, fmt.Errorf("не удалось открыть %s", entry.Name)
		}
		content, err := io.ReadAll(io.LimitReader(rc, maxBulkRecordSize+1))
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("не удалось прочитать %s", entry.Name)
		}
		if len(content) > maxBulkRecordSize {
			return nil, fmt.Errorf("файл %s слишком большой", entry.Name)
		}
		total += len(content)
		if total > maxBulkFileSize {
			return nil, errors.New("распакованный архив слишком большой")
		}
		parsed, err := parseBulkJSON(entry.Name, bytes.TrimSpace(bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name, err)
		}
		records = append(records, parsed...)
		if len(records) > maxBulkRows {
			return nil, fmt.Errorf("слишком много записей: максимум %d", maxBulkRows)
		}
	}
	if files == 0 {
		return nil, errors.New("в архиве нет JSON-файлов")
	}
	return records, nil
}

func parseBulkJSON(name string, data []byte) ([]bulkRecord, error) {
	if len(data) == 0 {
		return nil, errors.New("JSON пуст")
	}
	switch data[0] {
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("некорректный JSON-массив: %w", err)
		}
		return rawRecords(name, items), nil
	case '{':
		var object map[string]json.RawMessage
		if err := json.Unmarshal(data, &object); err == nil {
			if itemsRaw, exists := object["items"]; exists {
				var items []json.RawMessage
				if err := json.Unmarshal(itemsRaw, &items); err != nil {
					return nil, fmt.Errorf("поле items должно быть массивом")
				}
				return rawRecords(name, items), nil
			}
			return []bulkRecord{{File: name, Raw: json.RawMessage(data)}}, nil
		}
		return parseNDJSON(name, data), nil
	default:
		return parseNDJSON(name, data), nil
	}
}

func parseNDJSON(name string, data []byte) []bulkRecord {
	var records []bulkRecord
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var raw json.RawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			records = append(records, bulkRecord{File: name, ParseErr: err})
			continue
		}
		records = append(records, bulkRecord{File: name, Raw: raw})
	}
	return records
}

func rawRecords(name string, items []json.RawMessage) []bulkRecord {
	records := make([]bulkRecord, 0, len(items))
	for _, raw := range items {
		records = append(records, bulkRecord{File: name, Raw: raw})
	}
	return records
}

type bulkEmployeeQRRequest struct {
	EmployeeIDs []string `json:"employee_ids"`
}

type bulkEmployeeQRRow struct {
	EmployeeID     string  `json:"employee_id"`
	EmployeeNumber *string `json:"employee_number,omitempty"`
	FullName       *string `json:"full_name,omitempty"`
	Status         string  `json:"status"`
	QRID           *string `json:"qr_id,omitempty"`
	Reason         *string `json:"reason,omitempty"`
}

// handleBulkEmployeeQR выдаёт личные статические QR выбранным сотрудникам.
// Если активный QR уже есть, он не ротируется и возвращается со статусом existing.
func (s *Server) handleBulkEmployeeQR(w http.ResponseWriter, r *http.Request) {
	var req bulkEmployeeQRRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.EmployeeIDs) == 0 {
		writeError(w, http.StatusBadRequest, "employee_ids обязателен")
		return
	}
	if len(req.EmployeeIDs) > 500 {
		writeError(w, http.StatusBadRequest, "не более 500 сотрудников за запрос")
		return
	}
	seen := make(map[string]struct{}, len(req.EmployeeIDs))
	ids := make([]string, 0, len(req.EmployeeIDs))
	for _, raw := range req.EmployeeIDs {
		value := strings.TrimSpace(raw)
		if _, err := uuid.Parse(value); err != nil {
			writeError(w, http.StatusBadRequest, "employee_ids должны быть UUID")
			return
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		ids = append(ids, value)
	}

	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	principal, _ := principalFrom(r.Context())
	rows := make([]bulkEmployeeQRRow, 0, len(ids))
	createdCount, existingCount, errorCount := 0, 0, 0
	for _, id := range ids {
		row := bulkEmployeeQRRow{EmployeeID: id}
		employee, err := rp.GetEmployee(r.Context(), id)
		if errors.Is(err, repo.ErrNotFound) {
			row.Status = "error"
			reason := "сотрудник не найден"
			row.Reason = &reason
			errorCount++
			rows = append(rows, row)
			continue
		}
		if err != nil {
			row.Status = "error"
			reason := "ошибка чтения сотрудника"
			row.Reason = &reason
			errorCount++
			rows = append(rows, row)
			continue
		}
		row.EmployeeNumber = &employee.EmployeeNumber
		row.FullName = &employee.FullName
		if !employee.IsActive {
			row.Status = "error"
			reason := "сотрудник деактивирован"
			row.Reason = &reason
			errorCount++
			rows = append(rows, row)
			continue
		}
		token, err := rp.GetActiveStaticQR(r.Context(), employee.ID)
		if err == nil {
			row.Status = "existing"
			row.QRID = &token.ID
			existingCount++
			rows = append(rows, row)
			continue
		}
		if !errors.Is(err, repo.ErrNotFound) {
			row.Status = "error"
			reason := "ошибка чтения QR-кода"
			row.Reason = &reason
			errorCount++
			rows = append(rows, row)
			continue
		}
		token, err = s.issueServerStaticQR(r.Context(), rp, employee, principal.UserID, "")
		if err != nil {
			row.Status = "error"
			reason := "не удалось создать QR-код"
			row.Reason = &reason
			errorCount++
			rows = append(rows, row)
			continue
		}
		row.Status = "created"
		row.QRID = &token.ID
		createdCount++
		rows = append(rows, row)
		s.audit(r, rp, &principal.UserID, "qr.admin_issue", "qr_token", token.ID, map[string]any{
			"employee_id": employee.ID,
			"bulk":        true,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total":    len(ids),
		"created":  createdCount,
		"existing": existingCount,
		"errors":   errorCount,
		"rows":     rows,
	})
}
