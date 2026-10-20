package repo

import (
	"encoding/json"

	"github.com/google/uuid"
)

func nullUUID(value string) *string {
	if value == "" {
		return nil
	}
	if _, err := uuid.Parse(value); err != nil {
		return nil
	}
	return &value
}

func marshalJSON(value any) string {
	if value == nil {
		return "null"
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(encoded)
}
