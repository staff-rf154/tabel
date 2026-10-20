# Табель — QR-учёт рабочего времени

Система табелирования сотрудников: серверная часть (REST API), веб-панель администратора,
веб-интерфейс оператора и мобильное приложение (Flutter).

## Стек

| Компонент | Технология |
|-----------|------------|
| Backend | Go 1.24, стандартный `net/http`, pgx, go-redis |
| БД | PostgreSQL 16 + PgBouncer |
| Кэш | Redis 7 |
| Хранилище | Garage (S3-совместимое) |
| Веб-панели | React 18 + TypeScript + Vite |
| Контейнеризация | Docker Compose |

## Структура

```
backend/       серверная часть (Go)
web-admin/     веб-панель администратора
web-operator/  веб-интерфейс оператора
reports-web/   веб-приложение отчётов (руководитель смены / директор / кадры)
templates/     шаблоны массовой загрузки данных (CSV/JSON)
mobile/        мобильное приложение (Flutter) — появится на этапе 7
infra/         конфигурации инфраструктуры
docs/          OpenAPI-спецификация, ERD и документация
```

## Быстрый старт

1. Скопируйте окружение, задайте пароли и ключ шифрования настроек:

   ```
   cp .env.example .env
   KEY=$(openssl rand -hex 32)
   sed -i "s/CHANGE_ME_64_HEX_CHARS/$KEY/" .env
   ```

2. Создайте конфигурацию Garage (с секретами):

   ```
   RPC=$(openssl rand -hex 32)
   TOKEN=$(openssl rand -hex 24)
   sed -e "s/CHANGE_ME_64_HEX_CHARS/$RPC/" -e "s/CHANGE_ME_ADMIN_TOKEN/$TOKEN/" \
     infra/garage/garage.toml.example > infra/garage/garage.toml
   ```

3. Запустите стек:

   ```
   docker compose up -d --build
   ```

4. Проверьте сервисы:

   - Backend health: `http://localhost:8080/health`
   - Backend ready: `http://localhost:8080/ready`
   - Админ-панель: `http://localhost:8081`
   - Интерфейс оператора: `http://localhost:8082`
   - S3 API (Garage): `http://localhost:9000`
   - Garage admin API: `http://localhost:9001`

5. Первичная установка — откройте админ-панель `http://localhost:8081`, укажите
   параметры подключения к PostgreSQL и Redis и завершите установку.

## Первичная установка

Настройки подключения к БД/Redis и профиль главного администратора хранятся в SQLite
на сервере приложений (том `settings_data`, файл `SETTINGS_DB_PATH`). Секреты шифруются
AES-GCM ключом `SETTINGS_ENCRYPTION_KEY`.

Первичная установка выполняется через веб-панель администратора (эндпоинты `/setup/*`,
доступны через прокси `/api/*`):

| Эндпоинт | Назначение |
|----------|------------|
| `GET  /setup/status` | Состояние установки и доступность БД/Redis |
| `POST /setup/test-db` | Проверка подключения к PostgreSQL |
| `POST /setup/save` | Сохранение параметров и применение миграций (goose) |
| `POST /setup/complete` | Завершение установки |

После завершения установки эндпоинты `/setup/save`, `/setup/test-db` и `/setup/complete`
возвращают `409 Conflict`.

При завершении установки можно сразу создать главного администратора: передайте
`admin_login` и `admin_password` в `POST /setup/complete` (поля необязательные).
Если они не заданы, администратор создаётся на этапе деплоя.

Миграции БД выполняются автоматически при старте backend (встроены через `embed.FS`,
движок [goose](https://github.com/pressly/goose)); исходники — `backend/internal/db/migrations`.

## Аутентификация и API (Этапы 2–4)

- Пароли хранятся в виде **Argon2id**; политика: не менее 5 символов, буква и цифра.
- Access/refresh **JWT** (HS256), refresh-токены хранятся в БД в виде SHA-256 хэша и
  ротируются при обновлении. Время жизни: `JWT_ACCESS_TTL` (15m), `JWT_REFRESH_TTL` (720h).
- Роли: `employee`, `operator`, `admin`; административные маршруты `/admin/*` доступны только `admin`.
- После 5 неудачных попыток входа учётная запись блокируется (`423 Locked` с телефоном поддержки).
  Разблокировка — `POST /admin/users/{id}/unblock`.
- Регистрация сотрудника — по QR-токену (`POST /auth/register`, логин = телефон).
  Сброс регистрации администратором: `POST /admin/users/{id}/reset-registration`.
- Отметка `POST /mark` требует заголовок `Idempotency-Key` (UUID); повторная доставка
  возвращает ранее созданную отметку, одинаковые события в пределах 2 минут дедуплицируются.

| Эндпоинт | Назначение |
|----------|------------|
| `POST /auth/login` | Вход, выдача пары токенов |
| `POST /auth/refresh` | Ротация refresh-токена |
| `POST /auth/logout` | Отзыв текущей сессии |
| `POST /auth/register` | Регистрация по QR-токену |
| `POST /auth/password` | Смена собственного пароля |
| `GET  /me` | Профиль текущего пользователя |
| `POST /mark` | Создание отметки |
| `GET  /marks` | Отметки текущего сотрудника |
| `GET/POST /admin/employees`, `GET/PATCH /admin/employees/{id}` | Справочник сотрудников |
| `GET /admin/users`, `POST /admin/users/{id}/block`, `/unblock`, `/reset-registration` | Пользователи |
| `GET/PUT /admin/profile` | Профиль главного администратора |
| `GET /admin/audit` | Журнал аудита |
| `GET /schedules` | Активный график сотрудника (или 5/2 по умолчанию) |
| `GET /admin/schedules` | Графики сотрудников |
| `POST /admin/schedules/import?mode=preview\|commit` | Импорт графиков из CSV |
| `GET/POST /admin/offices` | Геозоны офисов |
| `GET /admin/marks` | Отметки с фильтрами |
| `GET /qr/{token}` | Проверка QR: статус, подпись, сверка полей |
| `POST /admin/qr/static` | Загрузка статического QR (подпись на сервере) |
| `POST /admin/qr/bulk?mode=preview\|commit` | Массовая загрузка QR (JSON/NDJSON/ZIP) |
| `POST /admin/qr/paper` | Выдача бумажного QR на 1 месяц |
| `POST /admin/qr/paper/{id}/revoke` | Отзыв бумажного QR |
| `GET /admin/qr/paper/{id}/pdf` | Печатная форма PDF/A |

Полная схема — `docs/openapi.yaml`.

### QR-токены (Этап 3)

- Ключ ECDSA P-256 создаётся при старте backend и хранится зашифрованным в SQLite
  (`qr_private_key`); приватный ключ не покидает сервер. Подписывается канонический
  JSON без поля `signature`.
- Сервер возвращает `payload` и готовую строку `qr_content`:
  `{"token":"<uuid>","signature":"<base64>","payload":{...}}`.
- Статический QR: `POST /admin/qr/static` с `employee_id` и `payload`. Сервер дополняет
  payload табельными данными, подписывает и при наличии предыдущего активного
  статического токена отзывает его (ротация).
- Массовая загрузка: `POST /admin/qr/bulk?mode=preview` только валидирует строки,
  `mode=commit` создаёт/ротирует статические QR. Поддерживаются JSON-массив, объект
  `{"items":[...]}`, NDJSON и ZIP с JSON-файлами; отчёт содержит строку, статус и причину.
  В записи нужны `payload` и одно из полей `employee_id` / `employee_number`.
- Бумажный QR: один активный на сотрудника, срок — 1 месяц включительно по
  Asia/Novosibirsk; отзыв идемпотентен, просроченный токен освобождает место для нового. PDF содержит QR, данные сотрудника, срок,
  инструкцию и XMP-метаданные PDF/A-1b (встроенные Unicode-шрифты DejaVu).
- `GET /qr/{token}` проверяет статус, срок, подпись и расхождение полей с БД.
  При расхождении `POST /mark` сохраняет отметку с `qr_data_mismatch = true`.

Пример массовой загрузки:

```json
[
  {
    "employee_id": "00000000-0000-0000-0000-000000000001",
    "payload": {
      "employee_number": "E-0001",
      "full_name": "Иванов Иван Иванович",
      "position": "Инженер"
    }
  }
]
```

### Графики, смены и геозоны (Этап 4)

- Если график не задан, действует график по умолчанию: 5/2, 09:00–18:00 (Asia/Novosibirsk).
- CSV-импорт (`POST /admin/schedules/import?mode=preview|commit`): обязательна колонка
  `employee_number`; поддерживаются `pattern_type` (`5/2`, `2/2`, `custom`),
  `workdays` (`1-5`, `1,3,5`), `start_time`, `end_time`, `valid_from`/`date_from`,
  `valid_to`/`date_to`. Значения по умолчанию: 5/2, дни 1–5, 09:00–18:00,
  `valid_from` — текущая дата Новосибирска. Строки одного сотрудника не должны пересекаться
  по срокам. `preview` возвращает построчный отчёт, `commit` заменяет активный набор
  графиков; активируется запись, действующая сегодня, иначе ближайшая будущая,
  иначе последняя прошедшая.
- Смена открывается отметкой `check_in` и закрывается `check_out`. Поле `event_type`
  в `POST /mark` необязательно: при открытой смене определяется `check_out`, иначе
  `check_in`. Явный `check_in` при открытой смене закрывает предыдущую смену
  автоматически: время закрытия — плановое окончание смены, но не позже текущего
  момента; создаётся синтетическая отметка `check_out` с `auto_closed_previous=true`
  и уведомление `shift.auto_closed`. `check_out` без открытой смены — `422`.
- Переработка: `overtime=true`, если момент отметки вне рабочего окна графика,
  включая выходные и ночные смены, переходящие через полночь.
- Геозоны: `outside_office=true`, если координаты отметки не попадают ни в один
  активный офис (`radius_m`, по умолчанию 500 м). Без координат или при отсутствии
  офисов флаг остаётся `false`. Администрирование — `GET/POST /admin/offices`.

Пример CSV графика:

```csv
employee_number,pattern_type,workdays,start_time,end_time,valid_from,valid_to
E-0001,custom,1-5,09:00,18:00,2026-01-01,
E-0001,2/2,1-2,20:00,08:00,2026-07-01,2026-12-31
```

### Выгрузка, OAuth, офлайн-импорт (Этап 5)

- OAuth 2.0 `client_credentials`: `POST /oauth/token` (form-urlencoded) с
  `grant_type=client_credentials`, `client_id`, `client_secret`. Ответ — `access_token`,
  `token_type=Bearer`, `expires_in=3600`. Секреты хранятся только в виде SHA-256 хэша.
- IP-whitelist: пока у клиента нет записей — доступ разрешён; после добавления хотя бы
  одного CIDR требуется попадание (иначе `403`). IP определяется по `X-Real-IP` →
  `X-Forwarded-For` → `RemoteAddr`.
- Управление (админ): `GET/POST /admin/oauth/clients`,
  `POST /admin/oauth/clients/{id}/rotate` (новый секрет + отзыв токенов),
  `GET/POST /admin/oauth/clients/{id}/whitelist`.
- Экспорт: `GET /export/marks?since=&limit=&cursor=` (Bearer OAuth), keyset-пагинация,
  `next_cursor` — base64 от `<timestamp_server>|<mark_id>`. `POST /export/ack` с
  `mark_ids` (до 1000), идемпотентно (повторный `ack` не считается).
- Ротация секрета — раз в 33 дня; уведомление админам за 5 дней (планировщик).
- Офлайн-импорт: `POST /admin/offline-imports` (multipart `file`). Колонки:
  `employee_number`, `timestamp_server` (обязательные), `event_type`,
  `idempotency_key`, `timestamp_client`, `lat`, `lon`, `qr_source`, `client_type`,
  `operator_number`. Дубли отсекаются по `idempotency_key`. Лимиты 20 МБ / 20000 строк.

```csv
employee_number,event_type,timestamp_server,idempotency_key,client_type
E-0001,check_in,2026-01-15T09:03:00+07:00,6f1c2b3a-9d4e-4a11-8c2f-2b6e5d7a9c10,mobile
```

### Интеграция с 1С:УПП/ЗУП

Для внешних систем используется тот же OAuth 2.0 `client_credentials`, что и для
выгрузки. Все ответы — JSON; время в RFC3339, бизнес-часовой пояс — `Asia/Novosibirsk`
(UTC+7).

- `GET /integration/1c/departments` — отделы (`id`, `name`, `is_active`).
- `GET /integration/1c/positions` — должности.
- `GET /integration/1c/employees?q=&is_active=&limit=&offset=` — сотрудники с
  `employee_number`, частями ФИО, `department_id`, `position_id`, датами приёма/увольнения.
- `GET /integration/1c/schedules?employee_id=` — графики; без `employee_id` — до 1000 записей.
- `GET /integration/1c/marks?since=&cursor=&limit=` — отметки, аналог `/export/marks`.
  Подтверждение — `POST /export/ack`.
- `GET /export/marks` остаётся основным контрактом для 1С:УПП; для 1С:ЗУП удобнее
  справочники и графики из `/integration/1c/*`.

Подробные описания полей и ограничений — в `docs/openapi.yaml`, тег `integration-1c`.
Сопоставление сотрудников рекомендуется по `employee_number`, подразделений и
должностей — по UUID из справочников.

### Делегирование и уведомления (Этап 6)

- Передача прав оператора: `POST /delegations` (`to_user_id`, `valid_until`) от
  оператора к сотруднику; `POST /delegations/{id}/approve|reject` — только админ;
  `POST /delegations/{id}/revoke` — админ или участник. При подтверждении передающий
  сразу теряет права (`employee`), принимающий получает `operator`; по истечении срока
  или отзыву роли возвращаются.
- Уведомления: `GET /notifications?unread=&limit=`, `POST /notifications/{id}/read`.
  Типы: автозакрытие смены, делегирование, истечение бумажного QR (за 5 дней),
  использование отозванного QR, расхождение полей, ротация секретов, ошибки
  офлайн-импорта.
- Планировщик раз в час: истечение бумажных QR, завершение делегирований, напоминания.

### Тесты

```
cd backend
go test ./...
```

Сквозные тесты Этапов 2–4 выполняются на реальной БД:

```
TEST_DATABASE_URL='postgres://tabel:...@localhost:5432/tabel?sslmode=disable' \
  go test ./internal/httpapi -run E2E -v
```

`TestRegistrationAndMarkE2E` проверяет регистрацию и отметки, `TestQRStage3E2E` —
статический и бумажный QR, PDF, массовую загрузку (JSON/ZIP), отзыв и `qr_data_mismatch`,
`TestStage4E2E` — импорт графиков, смены, авто-закрытие, переработку, геозоны и
недопустимые переходы, `TestStage5E2E` — OAuth, whitelist, экспорт/ack и офлайн-импорт,
`TestStage6E2E` — делегирование и уведомления.
В окружении `development` допускаются QR-токены без подписи (для отладки); в `production`
подпись обязательна.

## Роли, отчёты и импорт (Этап 15)

Дополнительно к `employee`/`operator`/`admin` доступны роли `shift_lead`
(руководитель смены), `director` (директор) и `hr` (отдел кадров). Они видят все
отметки и смены за предыдущий и текущий месяц и работают в отдельном приложении
`reports-web` (порт `8083`). Для не-администраторов диапазон отчёта автоматически
ограничивается предыдущим и текущим месяцем.

Отчёты:
- `GET /reports/shifts` — смены (сотрудник, начало/окончание, статус, переработка).
- `GET /reports/marks` — отметки (событие, время, переработка, вне офиса).
- `GET /reports/summary` — сводка по сотрудникам (смены, минуты, переработки).
- Фильтры: `from`, `to` (RFC3339), `employee_id`, `employee_ids` (через запятую),
  `position`; добавьте `format=csv` для выгрузки в таблицу (Excel).

Управление из панели администратора:
- `POST /admin/users` — создание пользователя с любой ролью.
- `POST /admin/employees/import?mode=preview|commit` — импорт сотрудников из CSV.
- `POST /admin/demo/seed` и `POST /admin/demo/clear` — загрузка/очистка демо-базы
  (10 сотрудников `DEMO-0001…DEMO-0010`).
- `POST /admin/qr/employees/bulk` — массовое создание личных QR выбранным сотрудникам
  (до 500 UUID); существующие активные QR не ротируются.
- В разделе «Отметки» геоданные отображаются на встроенной офлайн-карте
  Новосибирска (MapLibre GL). Карта по умолчанию свёрнута и открывается кнопкой.
  Подложка — локальные тайлы z12–14, контуры зданий — отдельные тайлы z13–15
  (128 935 объектов OSM) с overzoom. Добавлены подписи улиц, районов, воды, POI
  и номеров домов; шрифты лежат локально. Внешние тайлы и интернет не используются.
  Данные — в `web-admin/public/offline-tiles`, `offline-buildings` и
  `offline-fonts`; источник — OpenStreetMap / OpenFreeMap.
- `POST /admin/mobile-builds` — загрузка `.apk`/`.ipa`; публичное скачивание —
  `GET /mobile-builds/{id}/download`.

Шаблоны загрузки находятся в каталоге `templates/` и доступны из раздела
«Шаблоны» админ-панели (например, `/templates/employees.csv`).

## Мобильное приложение (Этапы 7–9)

Каталог `mobile/` — Flutter-приложение (Android/iOS), русский язык. Сотрудник видит
свои отметки за сегодня/неделю и профиль; оператор сканирует QR (камера или ручной
ввод), проверяет данные, выбирает тип события (авто/приход/уход) и отправляет отметку
с геолокацией и телеметрией устройства. При отсутствии связи отметки попадают в
офлайн-очередь (защищённое хранилище), которая блокирует сканирование до отправки.
Есть раздел передачи прав (делегирование).

Сборка (нужны Flutter и Android SDK):

```
cd mobile
flutter pub get
flutter analyze
flutter test
flutter build apk --release
```

Готовый APK: `mobile/build/app/outputs/flutter-apk/app-release.apk`. Его можно
загрузить в панели администратора (раздел «Мобильное приложение»); сотрудники
скачивают сборку по `GET /mobile-builds`.

## Порты (по умолчанию)

| Сервис | Порт |
|--------|------|
| backend | 8080 |
| web-admin | 8081 |
| web-operator | 8082 |
| reports-web | 8083 |
| postgres | 5432 |
| pgbouncer | 6432 |
| redis | 6379 |
| garage (S3) | 9000 / 9001 |

## Режим разработки

TLS на данном этапе не настроен (локальная разработка по HTTP). Выпуск сертификатов
и HTTPS на порту 443 настраиваются на этапе деплоя.

## Работа с планом

Поэтапный план разработки: [ПЛАН_ЭТАПОВ.md](./ПЛАН_ЭТАПОВ.md).
