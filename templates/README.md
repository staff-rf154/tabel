# Шаблоны загрузки данных

Все шаблоны для массовой загрузки находятся в этой папке.

| Файл | Назначение | Куда загружать |
| --- | --- | --- |
| `employees.csv` | Справочник сотрудников | Панель администратора → «Импорт данных» (`POST /admin/employees/import?mode=preview\|commit`) |
| `schedules.csv` | Графики работы | Панель администратора → «Импорт данных» (`POST /admin/schedules/import?mode=preview\|commit`) |
| `offline_marks.csv` | Офлайн-отметки | Панель администратора → «Импорт данных» (`POST /admin/offline-imports`) |
| `departments.csv` | Справочник отделов | Панель администратора → «Импорт данных» (`POST /admin/departments/import?mode=preview\|commit`) |
| `positions.csv` | Справочник должностей | Панель администратора → «Импорт данных» (`POST /admin/positions/import?mode=preview\|commit`) |
| `qr_bulk.json` | Массовая выдача статических QR | Панель администратора → «QR-токены» (`POST /admin/qr/bulk`) |

## Форматы

### employees.csv
Колонки: `employee_number` (обязательно), `last_name`, `first_name`, `middle_name` (или `full_name`), `position`, `department`, `email`, `phone`, `hired_at` (YYYY-MM-DD). Должность и отдел должны существовать в справочниках.

### schedules.csv
Колонки: `employee_number` (обязательно), `pattern_type` (`5/2`, `2/2`, `custom`), `workdays` (`1-5`, `1,3,5`), `start_time` (HH:MM), `end_time` (HH:MM), `valid_from` (YYYY-MM-DD), `valid_to` (YYYY-MM-DD, необязательно).

### offline_marks.csv
Колонки: `employee_number`, `timestamp_server` (RFC3339) — обязательные; `event_type` (`check_in`/`check_out`), `idempotency_key` (UUID), `timestamp_client`, `lat`, `lon`, `qr_source` (`static`/`paper`), `client_type` (`mobile`/`web`), `operator_number`.

### qr_bulk.json
Массив объектов. Для одного сотрудника: `{ "employee_number": "E-0001", "payload": {} }`. `payload` может содержать `token` (UUID) и произвольные поля, проверяемые на совпадение с данными сотрудника.
