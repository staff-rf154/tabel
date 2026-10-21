package httpapi

import (
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"tabel/internal/repo"
	"tabel/internal/settings"
)

func (s *Server) handleListEmployees(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	isActive, err := optionalBool(r.URL.Query().Get("is_active"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "is_active должен быть true или false")
		return
	}
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}

	employees, err := rp.ListEmployees(r.Context(), query, isActive, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить список сотрудников")
		return
	}
	if employees == nil {
		employees = []repo.Employee{}
	}
	writeJSON(w, http.StatusOK, employees)
}

func (s *Server) handleCreateEmployee(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	var in repo.EmployeeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if message := validateEmployeeInput(&in); message != "" {
		writeError(w, http.StatusBadRequest, message)
		return
	}

	employee, err := rp.CreateEmployee(r.Context(), in)
	switch {
	case errors.Is(err, repo.ErrEmployeeNumberTaken):
		writeError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, repo.ErrReferenceNotFound):
		writeError(w, http.StatusBadRequest, "отдел или должность не найдены в справочнике")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "не удалось создать сотрудника")
		return
	}
	principal, _ := principalFrom(r.Context())
	s.audit(r, rp, &principal.UserID, "employee.create", "employee", employee.ID, map[string]any{
		"employee_number": employee.EmployeeNumber,
		"full_name":       employee.FullName,
		"position":        employee.Position,
		"department_id":   employee.DepartmentID,
		"is_active":       employee.IsActive,
	})
	writeJSON(w, http.StatusCreated, employee)
}

func (s *Server) handleGetEmployee(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	id, ok := pathUUID(w, r)
	if !ok {
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
	writeJSON(w, http.StatusOK, employee)
}

func (s *Server) handleUpdateEmployee(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	var in repo.EmployeeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if message := validateEmployeeInput(&in); message != "" {
		writeError(w, http.StatusBadRequest, message)
		return
	}

	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	employee, err := rp.UpdateEmployee(r.Context(), id, in)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		writeError(w, http.StatusNotFound, "сотрудник не найден")
		return
	case errors.Is(err, repo.ErrEmployeeNumberTaken):
		writeError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, repo.ErrReferenceNotFound):
		writeError(w, http.StatusBadRequest, "отдел или должность не найдены в справочнике")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "не удалось обновить сотрудника")
		return
	}
	principal, _ := principalFrom(r.Context())
	s.audit(r, rp, &principal.UserID, "employee.update", "employee", employee.ID, map[string]any{
		"employee_number": employee.EmployeeNumber,
		"full_name":       employee.FullName,
		"position":        employee.Position,
		"department_id":   employee.DepartmentID,
		"is_active":       employee.IsActive,
	})
	writeJSON(w, http.StatusOK, employee)
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	if role != "" && !managedRoles[role] {
		writeError(w, http.StatusBadRequest, "недопустимая роль")
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(search)) < 3 {
		search = ""
	}
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	users, err := rp.ListUsers(r.Context(), role, search, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить список пользователей")
		return
	}
	if users == nil {
		users = []repo.User{}
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) handleBlockUser(w http.ResponseWriter, r *http.Request) {
	s.setUserAccess(w, r, true)
}

func (s *Server) handleUnblockUser(w http.ResponseWriter, r *http.Request) {
	s.setUserAccess(w, r, false)
}

func (s *Server) setUserAccess(w http.ResponseWriter, r *http.Request, blocked bool) {
	principal, _ := principalFrom(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if id == principal.UserID {
		writeError(w, http.StatusBadRequest, "нельзя изменить блокировку собственной учётной записи")
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
		writeError(w, http.StatusConflict, "нельзя заблокировать главного администратора")
		return
	}
	err = rp.SetAuthBlocked(r.Context(), id, blocked)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		writeError(w, http.StatusNotFound, "пользователь не найден")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "не удалось изменить блокировку")
		return
	}
	action := "user.unblock"
	if blocked {
		action = "user.block"
	}
	s.audit(r, rp, &principal.UserID, action, "user", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResetRegistration(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
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
		writeError(w, http.StatusConflict, "нельзя сбросить регистрацию главного администратора")
		return
	}
	err = rp.SetRegistrationReset(r.Context(), id, true)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		writeError(w, http.StatusNotFound, "сотрудник не найден")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "не удалось сбросить регистрацию")
		return
	}
	s.audit(r, rp, &principal.UserID, "user.reset_registration", "user", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

type adminProfileResponse struct {
	MainAdminEmail       string `json:"main_admin_email"`
	SupportPhone         string `json:"support_phone"`
	SupportEmail         string `json:"support_email"`
	CompanyName          string `json:"company_name"`
	CompanyAddress       string `json:"company_address"`
	CompanyINN           string `json:"company_inn"`
	Website              string `json:"website"`
	PaperQRValidDays     int    `json:"paper_qr_valid_days"`
	DefaultOfficeRadiusM int    `json:"default_office_radius_m"`
	PassFooter           string `json:"pass_footer"`
}

type adminProfileRequest struct {
	MainAdminEmail       *string `json:"main_admin_email"`
	SupportPhone         *string `json:"support_phone"`
	SupportEmail         *string `json:"support_email"`
	CompanyName          *string `json:"company_name"`
	CompanyAddress       *string `json:"company_address"`
	CompanyINN           *string `json:"company_inn"`
	Website              *string `json:"website"`
	PaperQRValidDays     *int    `json:"paper_qr_valid_days"`
	DefaultOfficeRadiusM *int    `json:"default_office_radius_m"`
	PassFooter           *string `json:"pass_footer"`
}

func (s *Server) handleGetAdminProfile(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.adminProfile())
}

func (s *Server) handleUpdateAdminProfile(w http.ResponseWriter, r *http.Request) {
	var req adminProfileRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.MainAdminEmail != nil {
		value := strings.TrimSpace(*req.MainAdminEmail)
		if value != "" {
			addr, err := mail.ParseAddress(value)
			if err != nil || addr.Address != value {
				writeError(w, http.StatusBadRequest, "main_admin_email должен быть корректным email")
				return
			}
		}
		if err := s.store.SetString(settings.KeyMainAdminEmail, value, false); err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сохранить email администратора")
			return
		}
	}
	if req.SupportEmail != nil {
		value := strings.TrimSpace(*req.SupportEmail)
		if value != "" {
			addr, err := mail.ParseAddress(value)
			if err != nil || addr.Address != value {
				writeError(w, http.StatusBadRequest, "support_email должен быть корректным email")
				return
			}
		}
		if err := s.store.SetString(settings.KeySupportEmail, value, false); err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сохранить email поддержки")
			return
		}
	}
	if req.SupportPhone != nil {
		value := strings.TrimSpace(*req.SupportPhone)
		if err := s.store.SetString(settings.KeySupportPhone, value, false); err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сохранить телефон поддержки")
			return
		}
	}
	textFields := []struct {
		value *string
		key   string
		limit int
		label string
	}{
		{req.CompanyName, settings.KeyCompanyName, 200, "company_name"},
		{req.CompanyAddress, settings.KeyCompanyAddress, 300, "company_address"},
		{req.CompanyINN, settings.KeyCompanyINN, 12, "company_inn"},
		{req.Website, settings.KeyWebsite, 200, "website"},
		{req.PassFooter, settings.KeyPassFooter, 200, "pass_footer"},
	}
	for _, field := range textFields {
		if field.value == nil {
			continue
		}
		value := strings.TrimSpace(*field.value)
		if len(value) > field.limit {
			writeError(w, http.StatusBadRequest, field.label+" превышает допустимую длину")
			return
		}
		if err := s.store.SetString(field.key, value, false); err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сохранить настройку "+field.label)
			return
		}
	}
	if req.PaperQRValidDays != nil {
		if *req.PaperQRValidDays < 1 || *req.PaperQRValidDays > 365 {
			writeError(w, http.StatusBadRequest, "paper_qr_valid_days должен быть от 1 до 365")
			return
		}
		if err := s.store.SetString(settings.KeyPaperQRValidDays, strconv.Itoa(*req.PaperQRValidDays), false); err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сохранить срок действия пропуска")
			return
		}
	}
	if req.DefaultOfficeRadiusM != nil {
		if *req.DefaultOfficeRadiusM < 10 || *req.DefaultOfficeRadiusM > 10000 {
			writeError(w, http.StatusBadRequest, "default_office_radius_m должен быть от 10 до 10000")
			return
		}
		if err := s.store.SetString(settings.KeyDefaultOfficeRadius, strconv.Itoa(*req.DefaultOfficeRadiusM), false); err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сохранить радиус офиса по умолчанию")
			return
		}
	}
	if rp, ok := s.repo(); ok {
		principal, _ := principalFrom(r.Context())
		s.audit(r, rp, &principal.UserID, "admin.profile_update", "settings", "", nil)
	}
	writeJSON(w, http.StatusOK, s.adminProfile())
}

func (s *Server) adminProfile() adminProfileResponse {
	email, _ := s.store.GetString(settings.KeyMainAdminEmail)
	phone, _ := s.store.GetString(settings.KeySupportPhone)
	supportEmail, _ := s.store.GetString(settings.KeySupportEmail)
	company, _ := s.store.GetString(settings.KeyCompanyName)
	address, _ := s.store.GetString(settings.KeyCompanyAddress)
	inn, _ := s.store.GetString(settings.KeyCompanyINN)
	website, _ := s.store.GetString(settings.KeyWebsite)
	footer, _ := s.store.GetString(settings.KeyPassFooter)
	return adminProfileResponse{
		MainAdminEmail:       email,
		SupportPhone:         phone,
		SupportEmail:         supportEmail,
		CompanyName:          company,
		CompanyAddress:       address,
		CompanyINN:           inn,
		Website:              website,
		PaperQRValidDays:     s.settingInt(settings.KeyPaperQRValidDays, 30),
		DefaultOfficeRadiusM: s.settingInt(settings.KeyDefaultOfficeRadius, 150),
		PassFooter:           footer,
	}
}

func (s *Server) settingInt(key string, fallback int) int {
	raw, err := s.store.GetString(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	query := r.URL.Query()
	filter := repo.AuditFilter{
		Action:      strings.TrimSpace(query.Get("action")),
		EntityType:  strings.TrimSpace(query.Get("entity_type")),
		EntityID:    strings.TrimSpace(query.Get("entity_id")),
		ActorUserID: strings.TrimSpace(query.Get("actor_user_id")),
		IP:          strings.TrimSpace(query.Get("ip")),
	}
	if filter.ActorUserID != "" {
		if _, err := uuid.Parse(filter.ActorUserID); err != nil {
			writeError(w, http.StatusBadRequest, "actor_user_id должен быть UUID")
			return
		}
	}
	var err error
	if raw := strings.TrimSpace(query.Get("from")); raw != "" {
		filter.From, err = parseRFC3339(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from должен быть датой-временем RFC3339")
			return
		}
	}
	if raw := strings.TrimSpace(query.Get("to")); raw != "" {
		filter.To, err = parseRFC3339(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to должен быть датой-временем RFC3339")
			return
		}
	}
	filter.Limit, filter.Offset, ok = pagination(w, r)
	if !ok {
		return
	}

	entries, err := rp.ListAudit(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить журнал аудита")
		return
	}
	if entries == nil {
		entries = []repo.AuditEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

func pathUUID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "id должен быть UUID")
		return "", false
	}
	return id, true
}

func validateEmployeeInput(in *repo.EmployeeInput) string {
	in.EmployeeNumber = strings.TrimSpace(in.EmployeeNumber)
	in.FullName = strings.TrimSpace(in.FullName)
	in.LastName = strings.TrimSpace(in.LastName)
	in.FirstName = strings.TrimSpace(in.FirstName)
	in.MiddleName = strings.TrimSpace(in.MiddleName)
	if in.EmployeeNumber == "" {
		return "поле employee_number обязательно"
	}
	if in.LastName != "" || in.FirstName != "" || in.MiddleName != "" {
		if in.LastName == "" || in.FirstName == "" {
			return "фамилия и имя обязательны"
		}
		in.FullName = joinNameParts(in.LastName, in.FirstName, in.MiddleName)
	} else {
		if in.FullName == "" {
			return "укажите фамилию, имя и отчество (или full_name)"
		}
		in.LastName, in.FirstName, in.MiddleName = splitFullName(in.FullName)
	}
	in.Email = normalizeOptional(in.Email)
	in.Phone = normalizeOptional(in.Phone)
	in.DepartmentID = normalizeOptional(in.DepartmentID)
	in.PositionID = normalizeOptional(in.PositionID)
	if in.PositionID != nil {
		if _, err := uuid.Parse(*in.PositionID); err != nil {
			return "position_id должен быть UUID"
		}
	}
	if in.DepartmentID != nil {
		if _, err := uuid.Parse(*in.DepartmentID); err != nil {
			return "department_id должен быть UUID"
		}
	}
	in.HiredAt = normalizeDate(in.HiredAt)
	in.FiredAt = normalizeDate(in.FiredAt)
	if in.HiredAt != nil {
		if _, err := time.Parse("2006-01-02", *in.HiredAt); err != nil {
			return "hired_at должен быть в формате YYYY-MM-DD"
		}
	}
	if in.FiredAt != nil {
		if _, err := time.Parse("2006-01-02", *in.FiredAt); err != nil {
			return "fired_at должен быть в формате YYYY-MM-DD"
		}
	}
	return ""
}

func joinNameParts(parts ...string) string {
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return strings.Join(result, " ")
}

func splitFullName(fullName string) (string, string, string) {
	parts := strings.Fields(fullName)
	switch len(parts) {
	case 0:
		return "", "", ""
	case 1:
		return parts[0], "", ""
	case 2:
		return parts[0], parts[1], ""
	default:
		return parts[0], parts[1], strings.Join(parts[2:], " ")
	}
}

func normalizeOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeDate(value *string) *string {
	trimmed := normalizeOptional(value)
	if trimmed == nil {
		return nil
	}
	return trimmed
}

func parseRFC3339(raw string) (*time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func optionalBool(raw string) (*bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func pagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "limit должен быть неотрицательным числом")
			return 0, 0, false
		}
		limit = parsed
	}
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "offset должен быть неотрицательным числом")
			return 0, 0, false
		}
		offset = parsed
	}
	return limit, offset, true
}
