package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"tabel/internal/repo"
	"tabel/internal/shift"
)

type scheduleTemplateRequest struct {
	Name        string `json:"name"`
	PatternType string `json:"pattern_type"`
	Workdays    []int  `json:"workdays"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
}

func scheduleTemplateInputFromRequest(w http.ResponseWriter, r *http.Request) (repo.ScheduleTemplateInput, bool) {
	var req scheduleTemplateRequest
	if !decodeJSON(w, r, &req) {
		return repo.ScheduleTemplateInput{}, false
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "укажите название графика")
		return repo.ScheduleTemplateInput{}, false
	}
	pattern, err := normalizePattern(req.PatternType)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return repo.ScheduleTemplateInput{}, false
	}
	workdays, err := normalizeWorkdays(req.Workdays, pattern)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return repo.ScheduleTemplateInput{}, false
	}
	start, err := shift.NormalizeClock(req.StartTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "время начала: "+err.Error())
		return repo.ScheduleTemplateInput{}, false
	}
	end, err := shift.NormalizeClock(req.EndTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "время окончания: "+err.Error())
		return repo.ScheduleTemplateInput{}, false
	}
	return repo.ScheduleTemplateInput{
		Name:        name,
		PatternType: pattern,
		Workdays:    workdays,
		StartTime:   start,
		EndTime:     end,
	}, true
}

func (s *Server) handleListScheduleTemplates(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	items, err := rp.ListScheduleTemplates(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить графики")
		return
	}
	if items == nil {
		items = []repo.ScheduleTemplate{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleCreateScheduleTemplate(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	in, ok := scheduleTemplateInputFromRequest(w, r)
	if !ok {
		return
	}
	in.CreatedBy = &principal.UserID
	item, err := rp.CreateScheduleTemplate(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось создать график")
		return
	}
	s.audit(r, rp, &principal.UserID, "schedule_template.create", "schedule", item.ID, map[string]any{
		"name": item.Name,
	})
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleUpdateScheduleTemplate(w http.ResponseWriter, r *http.Request) {
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
	in, ok := scheduleTemplateInputFromRequest(w, r)
	if !ok {
		return
	}
	item, err := rp.UpdateScheduleTemplate(r.Context(), id, in)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "график не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сохранить график")
		return
	}
	s.audit(r, rp, &principal.UserID, "schedule_template.update", "schedule", item.ID, map[string]any{
		"name": item.Name,
	})
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleDeleteScheduleTemplate(w http.ResponseWriter, r *http.Request) {
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
	if err := rp.DeleteScheduleTemplate(r.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "график не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, "не удалось удалить график")
		return
	}
	s.audit(r, rp, &principal.UserID, "schedule_template.delete", "schedule", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAssignScheduleTemplate(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var req struct {
		TargetType string  `json:"target_type"`
		TargetID   string  `json:"target_id"`
		ValidFrom  string  `json:"valid_from"`
		ValidTo    *string `json:"valid_to"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	targetType := strings.TrimSpace(req.TargetType)
	if targetType != "all" && targetType != "department" && targetType != "position" && targetType != "employee" {
		writeError(w, http.StatusBadRequest, "target_type должен быть all, department, position или employee")
		return
	}
	targetID := strings.TrimSpace(req.TargetID)
	if targetType != "all" {
		if _, err := uuid.Parse(targetID); err != nil {
			writeError(w, http.StatusBadRequest, "выберите получателя графика")
			return
		}
	}
	validFrom := strings.TrimSpace(req.ValidFrom)
	if validFrom == "" {
		validFrom = time.Now().In(shift.NovosibirskZone).Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", validFrom); err != nil {
		writeError(w, http.StatusBadRequest, "дата начала должна быть в формате ГГГГ-ММ-ДД")
		return
	}
	var validTo *string
	if req.ValidTo != nil {
		value := strings.TrimSpace(*req.ValidTo)
		if value != "" {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				writeError(w, http.StatusBadRequest, "дата окончания должна быть в формате ГГГГ-ММ-ДД")
				return
			}
			if value < validFrom {
				writeError(w, http.StatusBadRequest, "дата окончания не может быть раньше даты начала")
				return
			}
			validTo = &value
		}
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	template, err := rp.GetScheduleTemplate(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "график не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения графика")
		return
	}
	if targetType == "employee" {
		if _, err := rp.GetEmployee(r.Context(), targetID); errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "сотрудник не найден")
			return
		}
	}
	employeeIDs, err := rp.ListScheduleTargetEmployees(r.Context(), targetType, targetID)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "некорректный получатель графика")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось определить сотрудников")
		return
	}
	today := time.Now().In(shift.NovosibirskZone).Format("2006-01-02")
	source := "график: " + template.Name
	report := importReport{Rows: make([]importRow, 0, len(employeeIDs))}
	for index, employeeID := range employeeIDs {
		report.RowsTotal++
		row := importRow{Line: index + 1, Status: "ok"}
		input := repo.ScheduleInput{
			EmployeeID:  employeeID,
			PatternType: template.PatternType,
			Workdays:    template.Workdays,
			StartTime:   template.StartTime,
			EndTime:     template.EndTime,
			ValidFrom:   validFrom,
			ValidTo:     validTo,
			SourceFile:  &source,
			CreatedBy:   &principal.UserID,
		}
		if _, err := rp.ReplaceSchedules(r.Context(), employeeID, today, []repo.ScheduleInput{input}); err != nil {
			message := "не удалось назначить график"
			switch {
			case errors.Is(err, repo.ErrNotFound):
				message = "сотрудник не найден"
			case errors.Is(err, repo.ErrEmployeeInactive):
				message = "сотрудник деактивирован"
			}
			row.Status = "error"
			row.Reason = &message
			report.RowsError++
			report.Rows = append(report.Rows, row)
			continue
		}
		report.RowsOK++
		report.Rows = append(report.Rows, row)
	}
	s.audit(r, rp, &principal.UserID, "schedule.assign", "schedule", template.ID, map[string]any{
		"name":        template.Name,
		"target_type": targetType,
		"target_id":   targetID,
		"rows_total":  report.RowsTotal,
		"rows_ok":     report.RowsOK,
		"rows_error":  report.RowsError,
	})
	if len(employeeIDs) == 0 {
		writeError(w, http.StatusBadRequest, "не найдено ни одного активного сотрудника для назначения")
		return
	}
	writeJSON(w, http.StatusOK, report)
}
