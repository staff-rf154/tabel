# ERD — схема базы данных «Табель»

Версия: 1.0 (Этап 1)
СУБД: PostgreSQL 16
Источник истины: SQL-миграции `backend/internal/db/migrations/*.sql` (goose).
Машиночитаемая схема для dbdiagram.io: [`schema.dbml`](./schema.dbml).

## Диаграмма

```mermaid
erDiagram
    employees ||--o| users : "employee_id"
    employees ||--o{ schedules : "employee_id"
    employees ||--o{ shifts : "employee_id"
    employees ||--o{ marks : "employee_id"
    employees ||--o{ qr_tokens : "employee_id"

    users ||--o{ sessions : "user_id"
    users ||--o{ marks : "operator_id"
    users ||--o{ delegations : "from_user_id"
    users ||--o{ delegations : "to_user_id"
    users ||--o{ notifications : "target_user_id"
    users ||--o{ offline_imports : "uploaded_by"

    schedules ||--o{ shifts : "schedule_id"
    shifts ||--o{ marks : "shift_id"

    oauth_clients ||--o{ ip_whitelist : "oauth_client_id"

    employees {
        uuid id PK
        text employee_number UK
        text full_name
        text email
        text phone
        text position
        boolean is_active
        date hired_at
        date fired_at
        timestamptz created_at
        timestamptz updated_at
    }

    users {
        uuid id PK
        uuid employee_id FK,UK
        text role "employee|operator|admin"
        text login UK
        text password_hash
        text password_algo
        int failed_attempts
        boolean is_locked
        boolean auth_blocked
        boolean is_main_admin
        timestamptz registered_at
        timestamptz last_login_at
        timestamptz created_at
        timestamptz updated_at
    }

    sessions {
        uuid id PK
        uuid user_id FK
        text token_hash UK
        text user_agent
        inet ip
        timestamptz expires_at
        timestamptz revoked_at
        timestamptz created_at
    }

    schedules {
        uuid id PK
        uuid employee_id FK
        text pattern_type "5/2|2/2|custom"
        smallint_array workdays
        time start_time
        time end_time
        date valid_from
        date valid_to
        boolean is_active
        uuid created_by FK
        timestamptz created_at
        timestamptz updated_at
    }

    shifts {
        uuid id PK
        uuid employee_id FK
        uuid schedule_id FK
        uuid check_in_mark_id
        uuid check_out_mark_id
        timestamptz started_at
        timestamptz ended_at
        text status "open|closed|auto_closed"
        boolean overtime
        timestamptz created_at
        timestamptz updated_at
    }

    marks {
        uuid id PK
        timestamptz timestamp_server PK "partition key"
        uuid employee_id FK
        uuid operator_id FK
        text operator_number
        text event_type "check_in|check_out"
        timestamptz timestamp_client
        double lat
        double lon
        double accuracy_m
        int location_age_sec
        text device_os
        text device_os_version
        text device_model
        text app_version
        boolean mock_location
        boolean outside_office
        boolean overtime
        boolean auto_closed_previous
        text qr_source "static|paper"
        boolean qr_data_mismatch
        text client_type "mobile|web"
        uuid shift_id FK
        uuid idempotency_key
        timestamptz created_at
    }

    mark_idempotency {
        uuid idempotency_key PK
        uuid mark_id
        uuid employee_id
        timestamptz created_at
    }

    qr_tokens {
        uuid id PK
        uuid employee_id FK
        text type "static|paper"
        uuid token UK
        jsonb payload
        text signature
        date valid_from
        date valid_until
        text status "active|revoked|expired"
        uuid issued_by FK
        timestamptz issued_at
        timestamptz revoked_at
        uuid revoked_by FK
        timestamptz created_at
        timestamptz updated_at
    }

    delegations {
        uuid id PK
        uuid from_user_id FK
        uuid to_user_id FK
        text status "pending|approved|rejected|revoked|expired"
        timestamptz requested_at
        timestamptz valid_until
        uuid decided_by FK
        timestamptz decided_at
    }

    notifications {
        uuid id PK
        text type
        text title
        text body
        jsonb payload
        uuid target_user_id FK
        boolean is_read
        timestamptz created_at
    }

    audit_log {
        bigint id PK
        uuid actor_user_id FK
        text action
        text entity_type
        text entity_id
        jsonb details
        inet ip
        text user_agent
        timestamptz created_at
    }

    oauth_clients {
        uuid id PK
        text client_id UK
        text client_secret_hash
        text name
        boolean is_active
        timestamptz secret_rotated_at
        timestamptz next_rotation_at
    }

    oauth_access_tokens {
        uuid id PK
        text client_id
        text token_hash UK
        timestamptz expires_at
    }

    ip_whitelist {
        uuid id PK
        uuid oauth_client_id FK
        cidr cidr
        text description
    }

    export_acks {
        uuid mark_id PK
        text client_id
        timestamptz acked_at
    }

    offline_imports {
        uuid id PK
        uuid uploaded_by FK
        text filename
        int rows_total
        int rows_ok
        int rows_error
        text status
        jsonb report
        timestamptz created_at
    }

    offices {
        uuid id PK
        text name
        double lat
        double lon
        int radius_m
        boolean is_active
    }
```

## Ключевые проектные решения

### Партиционирование `marks`

Таблица `marks` партиционирована по `RANGE (timestamp_server)` — по месяцам.
Первичный ключ составной: `(id, timestamp_server)` (требование PostgreSQL для
партиционированных таблиц). При создании миграция создаёт партиции на текущий и
6 следующих месяцев; функция `create_marks_partition(date)` создаёт партицию
идемпотентно и вызывается регламентно (автогенерация — Этап 12).

Индексы (наследуются партициями):
- `(employee_id, timestamp_server DESC)` — отметки сотрудника;
- `(operator_id, timestamp_server)` — отметки оператора;
- `(shift_id)`, `(idempotency_key)`.

### Идемпотентность отметок

`mark_idempotency` хранит соответствие `Idempotency-Key → mark_id` для защиты от
повторной вставки при ретраях. Дополнительно `marks.idempotency_key` фиксирует
ключ на самой отметке.

### График и смены

Один активный график на сотрудника обеспечивается частичным уникальным индексом
`uq_schedules_active_per_employee ... WHERE is_active`. Смена (`shifts`) ссылается
на график и на отметки входа/выхода; статус отражает обычное/автозакрытие.

### QR-токены

Тип `static` или `paper`. Частичные уникальные индексы гарантируют не более одного
активного токена каждого типа на сотрудника. Подпись ECDSA хранится в `signature`,
полезная нагрузка — в `payload` (JSONB).

### Делегирование и уведомления

`delegations` описывает передачу прав оператора с жизненным циклом
`pending → approved/rejected → revoked/expired`. `notifications` — внутренние
уведомления (автозакрытие, делегирование, истечение QR и т.д.).

### Экспорт и OAuth

`oauth_clients` + `oauth_access_tokens` + `ip_whitelist` — OAuth 2.0
client_credentials с ограничением по IP. `export_acks` фиксирует подтверждённую
внешней системой выгрузку (идемпотентно по `mark_id`). `offline_imports` хранит
отчёты о загрузке офлайн-данных.

### Аудит

`audit_log` — append-only журнал действий с актором, сущностью и деталями (JSONB).

### Геозона

`offices` — геозоны офисов (координаты и радиус, по умолчанию 500 м).

## Конфигурация подключения (SQLite на сервере приложений)

Настройки подключения к PostgreSQL/Redis и профиль главного администратора хранятся
в SQLite-хранилище на сервере приложений (`SETTINGS_DB_PATH`, по умолчанию
`/data/settings.db`). Секреты (пароль БД, пароль Redis) шифруются AES-GCM ключом
`SETTINGS_ENCRYPTION_KEY`. Первичная настройка выполняется через веб-интерфейс
(эндпоинты `/setup/*`).
