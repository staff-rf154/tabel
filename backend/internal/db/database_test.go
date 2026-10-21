package db

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestEnsureDatabaseCreatesSchema(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg, err := pgx.ParseConfig(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("tabel_schema_test_%d", time.Now().UnixNano())
	targetURL, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	targetURL.Path = "/" + name

	adminCfg := cfg.Copy()
	adminCfg.Database = "postgres"
	admin, err := pgx.ConnectConfig(ctx, adminCfg)
	if err != nil {
		t.Fatalf("подключение к postgres: %v", err)
	}
	defer func() {
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		_ = admin.Close(ctx)
	}()

	created, err := EnsureDatabase(ctx, targetURL.String())
	if err != nil {
		t.Fatalf("EnsureDatabase: %v", err)
	}
	if !created {
		t.Fatal("ожидалось создание базы данных")
	}
	created, err = EnsureDatabase(ctx, targetURL.String())
	if err != nil {
		t.Fatalf("повторный EnsureDatabase: %v", err)
	}
	if created {
		t.Fatal("повторный вызов не должен создавать базу")
	}

	if err := Migrate(ctx, targetURL.String()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	pool, err := Connect(ctx, targetURL.String())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer pool.Close()
	var employees *string
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.employees')::text").Scan(&employees); err != nil {
		t.Fatalf("проверка структуры: %v", err)
	}
	if employees == nil || *employees == "" {
		t.Fatal("таблица employees не создана")
	}
}
