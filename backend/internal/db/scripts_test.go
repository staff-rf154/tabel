package db

import (
	"strings"
	"testing"
)

func TestSetupScriptsContainSchema(t *testing.T) {
	scripts, err := SetupScripts()
	if err != nil {
		t.Fatalf("setup scripts: %v", err)
	}
	if len(scripts) < 5 {
		t.Fatalf("expected at least 5 scripts, got %d", len(scripts))
	}
	var schema string
	for _, script := range scripts {
		if script.Name == "03_schema.sql" {
			schema = script.Content
		}
		if strings.Contains(script.Content, "-- +goose Up") || strings.Contains(script.Content, "-- +goose Down") {
			t.Fatalf("script %s contains goose markers", script.Name)
		}
	}
	if schema == "" {
		t.Fatal("schema script not found")
	}
	for _, table := range []string{"employees", "marks", "schedules", "qr_tokens"} {
		if !strings.Contains(schema, table) {
			t.Fatalf("schema does not contain %s", table)
		}
	}
}
