package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// EnsureDatabase проверяет, что база данных из строки подключения существует,
// и создаёт её через служебную базу postgres, если она отсутствует.
func EnsureDatabase(ctx context.Context, databaseURL string) (bool, error) {
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return false, fmt.Errorf("разбор строки подключения: %w", err)
	}
	name := strings.TrimSpace(cfg.Database)
	if name == "" {
		return false, fmt.Errorf("в строке подключения не указано имя базы данных")
	}

	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err == nil {
		_ = conn.Close(ctx)
		return false, nil
	}
	if !isMissingDatabase(err) {
		return false, err
	}
	if name == "postgres" {
		return false, err
	}

	adminCfg := cfg.Copy()
	adminCfg.Database = "postgres"
	admin, err := pgx.ConnectConfig(ctx, adminCfg)
	if err != nil {
		return false, fmt.Errorf("подключение к служебной базе postgres: %w", err)
	}
	defer func() { _ = admin.Close(ctx) }()

	query := "CREATE DATABASE " + pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, query); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P04" {
			return false, nil
		}
		return false, fmt.Errorf("создание базы данных %q: %w", name, err)
	}
	return true, nil
}

func isMissingDatabase(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "3D000"
}
