package settings

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const (
	KeyDatabaseURL         = "database_url"
	KeyDatabaseDirectURL   = "database_direct_url"
	KeyRedisAddr           = "redis_addr"
	KeyRedisPassword       = "redis_password"
	KeyPgBouncerHost       = "pgbouncer_host"
	KeyPgBouncerPort       = "pgbouncer_port"
	KeyPgBouncerPoolMode   = "pgbouncer_pool_mode"
	KeyPgBouncerMaxClients = "pgbouncer_max_client_conn"
	KeyPgBouncerPoolSize   = "pgbouncer_default_pool_size"
	KeyPgBouncerAuthType   = "pgbouncer_auth_type"
	KeyS3Endpoint          = "s3_endpoint"
	KeyS3Region            = "s3_region"
	KeyS3Bucket            = "s3_bucket"
	KeyS3AccessKey         = "s3_access_key"
	KeyS3SecretKey         = "s3_secret_key"
	KeyS3UseSSL            = "s3_use_ssl"
	KeyS3PathStyle         = "s3_path_style"
	KeySSLCertPath         = "ssl_cert_path"
	KeySSLKeyPath          = "ssl_key_path"
	KeyHTTPPort            = "service_port_backend"
	KeyAdminPort           = "service_port_admin"
	KeyOperatorPort        = "service_port_operator"
	KeyReportsPort         = "service_port_reports"
	KeyPostgresPort        = "service_port_postgres"
	KeyPgBouncerPublicPort = "service_port_pgbouncer"
	KeyRedisPublicPort     = "service_port_redis"
	KeyS3APIPort           = "service_port_s3_api"
	KeyS3AdminPort         = "service_port_s3_admin"
	KeyHTTPSPort           = "service_port_https"
	KeyMainAdminEmail      = "main_admin_email"
	KeySupportPhone        = "support_phone"
	KeySupportEmail        = "support_email"
	KeyCompanyName         = "company_name"
	KeyCompanyAddress      = "company_address"
	KeyCompanyINN          = "company_inn"
	KeyWebsite             = "website"
	KeyPaperQRValidDays    = "paper_qr_valid_days"
	KeyDefaultOfficeRadius = "default_office_radius_m"
	KeyPassFooter          = "pass_footer"
	KeyQRPrivateKey        = "qr_private_key"
)

var ErrNotFound = errors.New("настройка не найдена")

type Store struct {
	db  *sql.DB
	key []byte
}

func Open(path string, key []byte) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("создание каталога настроек: %w", err)
		}
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("открытие базы настроек: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("проверка базы настроек: %w", err)
	}

	st := &Store{db: db, key: key}
	if err := st.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return st, nil
}

func (s *Store) init() error {
	const schema = `
CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      BLOB,
    is_secret  INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS setup_state (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    installed    INTEGER NOT NULL DEFAULT 0,
    installed_at TEXT
);

INSERT OR IGNORE INTO setup_state (id, installed) VALUES (1, 0);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("инициализация схемы настроек: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) GetString(key string) (string, error) {
	var raw []byte
	var secret int
	err := s.db.QueryRow(`SELECT value, is_secret FROM settings WHERE key = ?`, key).Scan(&raw, &secret)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}

	value := raw
	if secret == 1 {
		value, err = decrypt(s.key, raw)
		if err != nil {
			return "", fmt.Errorf("расшифровка настройки %q: %w", key, err)
		}
	}
	return string(value), nil
}

func (s *Store) SetString(key, value string, secret bool) error {
	raw := []byte(value)
	if secret {
		var err error
		if raw, err = encrypt(s.key, []byte(value)); err != nil {
			return fmt.Errorf("шифрование настройки %q: %w", key, err)
		}
	}

	_, err := s.db.Exec(`
INSERT INTO settings (key, value, is_secret, updated_at)
VALUES (?, ?, ?, datetime('now'))
ON CONFLICT(key) DO UPDATE
SET value = excluded.value, is_secret = excluded.is_secret, updated_at = excluded.updated_at`,
		key, raw, boolToInt(secret))
	return err
}

func (s *Store) Backup(path string) error {
	_, err := s.db.Exec(`VACUUM INTO ?`, path)
	return err
}

func (s *Store) IsInstalled() (bool, error) {
	var installed int
	if err := s.db.QueryRow(`SELECT installed FROM setup_state WHERE id = 1`).Scan(&installed); err != nil {
		return false, err
	}
	return installed == 1, nil
}

func (s *Store) MarkInstalled() error {
	_, err := s.db.Exec(`UPDATE setup_state SET installed = 1, installed_at = datetime('now') WHERE id = 1`)
	return err
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
