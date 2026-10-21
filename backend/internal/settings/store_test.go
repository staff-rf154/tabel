package settings

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestBackupCreatesReadableSnapshot(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "settings.db"), bytes.Repeat([]byte("k"), 32))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	if err := store.SetString("company_name", "ООО Тест", false); err != nil {
		t.Fatalf("set value: %v", err)
	}

	target := filepath.Join(dir, "backup.db")
	if err := store.Backup(target); err != nil {
		t.Fatalf("backup: %v", err)
	}

	db, err := sql.Open("sqlite", "file:"+target)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer func() { _ = db.Close() }()
	var value string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'company_name'`).Scan(&value); err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if value != "ООО Тест" {
		t.Fatalf("unexpected value %q", value)
	}
}
