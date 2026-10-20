package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"tabel/internal/repo"
)

func decodePayloadMap(raw []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("лишние данные после JSON")
		}
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("payload должен быть JSON-объектом")
	}
	return object, nil
}

func payloadString(payload map[string]any, key string) (string, bool) {
	value, ok := payload[key]
	if !ok || value == nil {
		return "", false
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v), true
	case json.Number:
		return v.String(), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(v), true
	default:
		return "", false
	}
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

// validatePayloadFields сверяет известные поля payload с данными сотрудника.
// Отсутствующие в payload поля не проверяются.
func validatePayloadFields(payload map[string]any, employee repo.Employee, token, typ string, validFrom, validUntil *string) string {
	checks := []struct {
		key   string
		value string
	}{
		{"employee_id", employee.ID},
		{"employee_number", employee.EmployeeNumber},
		{"full_name", employee.FullName},
		{"position", optionalString(employee.Position)},
		{"phone", optionalString(employee.Phone)},
		{"email", optionalString(employee.Email)},
		{"token", token},
		{"type", typ},
	}
	for _, check := range checks {
		value, ok := payloadString(payload, check.key)
		if !ok {
			continue
		}
		if value == "" && check.value == "" {
			continue
		}
		if value != check.value {
			return fmt.Sprintf("поле %s в payload не совпадает с данными сотрудника", check.key)
		}
	}
	dates := []struct {
		key   string
		value string
	}{
		{"valid_from", optionalString(validFrom)},
		{"valid_until", optionalString(validUntil)},
	}
	for _, date := range dates {
		value, ok := payloadString(payload, date.key)
		if !ok {
			continue
		}
		if value != date.value {
			return fmt.Sprintf("поле %s в payload не совпадает со сроком действия", date.key)
		}
	}
	return ""
}

func buildPayload(base map[string]any, employee repo.Employee, token, typ string, validFrom, validUntil *string) ([]byte, error) {
	payload := make(map[string]any, len(base)+8)
	for key, value := range base {
		payload[key] = value
	}
	delete(payload, "signature")
	delete(payload, "valid_from")
	delete(payload, "valid_until")

	payload["employee_id"] = employee.ID
	payload["employee_number"] = employee.EmployeeNumber
	payload["full_name"] = employee.FullName
	payload["token"] = token
	payload["type"] = typ
	if employee.Position != nil {
		payload["position"] = *employee.Position
	}
	if employee.Phone != nil {
		payload["phone"] = *employee.Phone
	}
	if employee.Email != nil {
		payload["email"] = *employee.Email
	}
	if validFrom != nil {
		payload["valid_from"] = *validFrom
	}
	if validUntil != nil {
		payload["valid_until"] = *validUntil
	}
	return json.Marshal(payload)
}

// qrDataMismatch проверяет, расходятся ли сохранённые в QR поля с текущими
// данными сотрудника (например, после переименования или смены должности).
func qrDataMismatch(raw []byte, token repo.QRToken) bool {
	payload, err := decodePayloadMap(raw)
	if err != nil {
		return true
	}
	return validatePayloadFields(payload, repo.Employee{
		ID:             token.EmployeeID,
		EmployeeNumber: token.EmployeeNum,
		FullName:       token.FullName,
		Position:       token.Position,
		Phone:          token.Phone,
		Email:          token.Email,
	}, token.Token, token.Type, token.ValidFrom, token.ValidUntil) != ""
}
