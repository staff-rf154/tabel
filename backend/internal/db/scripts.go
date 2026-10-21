package db

import (
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type PGSetting struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Note  string `json:"note"`
}

type SetupScript struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Content     string `json:"-"`
}

var (
	scriptsOnce sync.Once
	scriptsList []SetupScript
	scriptsErr  error
)

func PostgresRecommendations() []PGSetting {
	return []PGSetting{
		{"max_connections", "100", "Приложение ходит через PgBouncer; держите запас для обслуживания и миграций."},
		{"shared_buffers", "2GB", "Около 25% RAM сервера 8–10 ГБ. После изменения требуется перезапуск PostgreSQL."},
		{"effective_cache_size", "6GB", "Около 60–75% RAM; используется планировщиком, память не резервируется."},
		{"maintenance_work_mem", "512MB", "Для VACUUM, CREATE INDEX и восстановления дампа."},
		{"work_mem", "8MB", "На одну операцию сортировки/хэширования; при 100 соединениях верхняя оценка около 800 МБ."},
		{"wal_buffers", "16MB", "Достаточно для типовой нагрузки; при интенсивной записи можно 32MB."},
		{"min_wal_size", "1GB", "Сглаживает рост WAL и частые checkpoint."},
		{"max_wal_size", "4GB", "Компромисс между объёмом WAL и временем восстановления."},
		{"checkpoint_timeout", "15min", "Реже checkpoint при небольшой нагрузке."},
		{"checkpoint_completion_target", "0.9", "Равномерная запись checkpoint."},
		{"random_page_cost", "1.1", "Для SSD/NVMe; для HDD оставьте 4.0."},
		{"effective_io_concurrency", "200", "Для SSD/NVMe; для HDD используйте 2–4."},
		{"max_worker_processes", "8", "По числу vCPU сервера."},
		{"max_parallel_workers", "8", "Не больше числа vCPU."},
		{"max_parallel_workers_per_gather", "2", "Для OLTP не стоит выделять все ядра одному запросу."},
		{"autovacuum_naptime", "30s", "Более своевременный autovacuum для таблицы отметок."},
		{"autovacuum_max_workers", "3", "Можно увеличить при большом количестве обновлений."},
		{"idle_in_transaction_session_timeout", "60s", "Защита от зависших транзакций."},
		{"statement_timeout", "60s", "Защита от случайно тяжёлых запросов; для отчётов можно выше."},
		{"lock_timeout", "10s", "Не даёт бесконечно ждать блокировку."},
		{"track_io_timing", "on", "Нужен для диагностики I/O через pg_stat_statements."},
		{"log_min_duration_statement", "1000ms", "Логировать запросы медленнее 1 секунды."},
		{"log_checkpoints", "on", "Помогает оценить влияние checkpoint."},
		{"timezone", "UTC", "Хранить timestamptz в UTC; локальное время отображает приложение."},
	}
}

func RecommendedPostgresConf() string {
	var b strings.Builder
	b.WriteString("# Рекомендации для PostgreSQL 16, сервер 8–10 ГБ RAM / 8 vCPU, SSD.\n")
	b.WriteString("# Проверьте значения под фактическую нагрузку. shared_buffers требует перезапуска.\n\n")
	for _, item := range PostgresRecommendations() {
		b.WriteString(fmt.Sprintf("# %s\n%s = %s\n\n", item.Note, item.Name, item.Value))
	}
	return b.String()
}

func SetupScripts() ([]SetupScript, error) {
	scriptsOnce.Do(func() {
		schema, err := schemaSQL()
		if err != nil {
			scriptsErr = err
			return
		}
		scriptsList = []SetupScript{
			{
				Name:        "01_create_role_database.sql",
				Title:       "Создание роли и базы данных",
				Description: "Выполняется от суперпользователя PostgreSQL. Замените пароль и имя перед запуском.",
				Content: `-- Создание роли и базы данных Табель.
-- Выполнить от суперпользователя PostgreSQL:
--   psql -h 127.0.0.1 -U postgres -f 01_create_role_database.sql
-- Замените CHANGE_ME на стойкий пароль.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tabel') THEN
        CREATE ROLE tabel LOGIN PASSWORD 'CHANGE_ME';
    END IF;
END
$$;

SELECT 'CREATE DATABASE tabel OWNER tabel ENCODING ''UTF8'' TEMPLATE template0'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'tabel')\gexec

\connect tabel
ALTER DATABASE tabel SET timezone TO 'UTC';
`,
			},
			{
				Name:        "02_init.sql",
				Title:       "Инициализация базы",
				Description: "Часовой пояс, расширение pgcrypto и базовая проверка.",
				Content: `-- Инициализация базы Табель.
-- Выполнить после 01_create_role_database.sql:
--   psql -h 127.0.0.1 -U tabel -d tabel -f 02_init.sql

SET timezone TO 'UTC';
CREATE EXTENSION IF NOT EXISTS pgcrypto;
SELECT current_database(), current_user, now();
`,
			},
			{
				Name:        "03_schema.sql",
				Title:       "Создание всех таблиц",
				Description: "Полная схема из всех миграций приложения. Можно применять на чистой базе.",
				Content:     schema,
			},
			{
				Name:        "04_recommended_postgresql.conf",
				Title:       "Рекомендуемые параметры PostgreSQL",
				Description: "Пример настроек для PostgreSQL 16 на сервере 8–10 ГБ RAM / 8 vCPU / SSD.",
				Content:     RecommendedPostgresConf(),
			},
			{
				Name:        "README.txt",
				Title:       "Порядок установки",
				Description: "Как создать базу, применить схему и проверить подключение.",
				Content: `Табель — установка PostgreSQL вручную

1. Создайте роль и базу:
   psql -h 127.0.0.1 -U postgres -f 01_create_role_database.sql

2. Инициализируйте базу:
   psql -h 127.0.0.1 -U tabel -d tabel -f 02_init.sql

3. Создайте таблицы:
   psql -h 127.0.0.1 -U tabel -d tabel -f 03_schema.sql

4. Проверьте подключение:
   psql "postgres://tabel:ПАРОЛЬ@127.0.0.1:5432/tabel?sslmode=disable" -c "select count(*) from employees;"

5. В настройках Табеля укажите:
   - строку подключения через PgBouncer — для работы приложения;
   - прямое подключение к PostgreSQL — для миграций, дампа и восстановления;
   - параметры S3, Redis, SSL и портов.

6. Для переноса существующей базы используйте раздел
   «Перенос PostgreSQL» в настройках. Он вызывает pg_dump/pg_restore.
`,
			},
		}
	})
	if scriptsErr != nil {
		return nil, scriptsErr
	}
	out := make([]SetupScript, len(scriptsList))
	copy(out, scriptsList)
	return out, nil
}

func schemaSQL() (string, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("-- Схема базы данных Табель.\n")
	b.WriteString("-- Сгенерировано из встроенных миграций приложения.\n\n")
	b.WriteString("SET timezone TO 'UTC';\n\n")
	maxVersion := 0
	for _, name := range names {
		raw, err := fs.ReadFile(migrationsFS, "migrations/"+name)
		if err != nil {
			return "", err
		}
		up, err := upSection(string(raw))
		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		b.WriteString("-- " + name + "\n")
		b.WriteString(up)
		b.WriteString("\n")
		prefix := name
		if idx := strings.IndexByte(prefix, '_'); idx > 0 {
			prefix = prefix[:idx]
		}
		if version, err := strconv.Atoi(strings.TrimLeft(prefix, "0")); err == nil && version > maxVersion {
			maxVersion = version
		}
	}
	b.WriteString(gooseVersionSQL(maxVersion))
	return b.String(), nil
}

func gooseVersionSQL(maxVersion int) string {
	values := make([]string, 0, maxVersion+1)
	for version := 0; version <= maxVersion; version++ {
		values = append(values, fmt.Sprintf("(%d)", version))
	}
	return fmt.Sprintf(`-- Фиксируем версию схемы, чтобы приложение не применяло миграции повторно.
CREATE TABLE IF NOT EXISTS goose_db_version (
    id         integer NOT NULL GENERATED BY DEFAULT AS IDENTITY,
    version_id bigint NOT NULL,
    is_applied boolean NOT NULL,
    tstamp     timestamp without time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (id)
);

INSERT INTO goose_db_version (version_id, is_applied)
SELECT version_id, true
FROM (VALUES %s) AS versions(version_id)
WHERE NOT EXISTS (SELECT 1 FROM goose_db_version);
`, strings.Join(values, ", "))
}

func upSection(raw string) (string, error) {
	up := strings.Index(raw, "-- +goose Up")
	if up < 0 {
		return "", fmt.Errorf("не найден блок +goose Up")
	}
	body := raw[up+len("-- +goose Up"):]
	if down := strings.Index(body, "-- +goose Down"); down >= 0 {
		body = body[:down]
	}
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "-- +goose Statement") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n")), nil
}
