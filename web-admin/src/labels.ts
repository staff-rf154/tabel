const roles: Record<string, string> = {
  employee: "Сотрудник",
  operator: "Оператор",
  admin: "Администратор",
  shift_lead: "Руководитель смены",
  director: "Директор",
  hr: "Отдел кадров",
};

const events: Record<string, string> = {
  check_in: "Приход",
  check_out: "Уход",
};

const qrSources: Record<string, string> = {
  static: "Статический QR",
  paper: "Бумажный QR",
};

const clientTypes: Record<string, string> = {
  web: "Веб-панель",
  mobile: "Мобильное приложение",
};

const shiftStatuses: Record<string, string> = {
  open: "Открыта",
  closed: "Закрыта",
  auto_closed: "Закрыта автоматически",
};

const delegationStatuses: Record<string, string> = {
  pending: "Ожидает решения",
  approved: "Подтверждено",
  rejected: "Отклонено",
  revoked: "Отозвано",
  expired: "Истекло",
};

const tokenStatuses: Record<string, string> = {
  active: "Активен",
  revoked: "Отозван",
  expired: "Истёк",
  not_found: "Не найден",
};

const patterns: Record<string, string> = {
  "5/2": "5/2 (пн–пт)",
  "2/2": "2/2",
  "4/2": "4/2",
  "1/2": "1/2",
  custom: "Индивидуальный",
};

const platforms: Record<string, string> = {
  android: "Android",
  ios: "iOS",
};

const notificationTypes: Record<string, string> = {
  "delegation.requested": "Запрос делегирования",
  "delegation.approved": "Делегирование подтверждено",
  "delegation.rejected": "Делегирование отклонено",
  "delegation.revoked": "Делегирование отозвано",
  "shift.auto_closed": "Автоматическое закрытие смены",
};

const auditActions: Record<string, string> = {
  "admin.profile_update": "Профиль администратора обновлён",
  "admin.system_settings_update": "Системные настройки обновлены",
  "auth.login": "Выполнен вход в систему",
  "auth.login_blocked": "Вход заблокирован",
  "auth.login_failed": "Неудачная попытка входа",
  "auth.logout": "Выполнен выход из системы",
  "auth.password_changed": "Пароль изменён",
  "auth.register": "Учётная запись зарегистрирована",
  "delegation.request": "Запрошено делегирование",
  "delegation.approve": "Делегирование подтверждено",
  "delegation.reject": "Делегирование отклонено",
  "delegation.revoke": "Делегирование отозвано",
  "demo.clear": "Демонстрационные данные удалены",
  "demo.seed": "Демонстрационные данные загружены",
  "demo.reset": "Демонстрационная база заменена",
  "department.create": "Отдел создан",
  "department.import": "Выполнен импорт отделов",
  "position.create": "Должность создана",
  "position.import": "Выполнен импорт должностей",
  "employee.create": "Сотрудник создан",
  "employee.import": "Выполнен импорт сотрудников",
  "employee.update": "Данные сотрудника изменены",
  "mark.create": "Отметка создана",
  "mobile_build.delete": "Сборка приложения удалена",
  "mobile_build.upload": "Сборка приложения загружена",
  "oauth.client_create": "OAuth-клиент создан",
  "oauth.client_update": "OAuth-клиент изменён",
  "oauth.client_disable": "OAuth-клиент отключён",
  "oauth.client_enable": "OAuth-клиент включён",
  "oauth.client_delete": "OAuth-клиент удалён",
  "oauth.client_rotate": "Секрет OAuth-клиента изменён",
  "oauth.whitelist_add": "IP-адрес добавлен в разрешённые",
  "office.create": "Офис создан",
  "office.update": "Офис изменён",
  "office.delete": "Офис удалён",
  "offline_import": "Выполнен офлайн-импорт",
  "qr.admin_issue": "Личный QR создан администратором",
  "qr.admin_rotate": "QR сотрудника изменён администратором",
  "qr.self_rotate": "QR изменён сотрудником",
  "qr.paper_print_bulk": "Массовая печать бумажных пропусков",
  "qr.paper_issue": "Бумажный QR выпущен",
  "qr.paper_revoke": "Бумажный QR отозван",
  "qr.static_upload": "Статический QR загружен",
  "schedule.create": "График создан",
  "schedule.update": "График изменён",
  "schedule.delete": "График удалён",
  "schedule.bulk": "Графики назначены массово",
  "schedule.import": "Выполнен импорт графиков",
  "schedule.assign": "График назначен",
  "schedule_template.create": "Шаблон графика создан",
  "schedule_template.update": "Шаблон графика изменён",
  "schedule_template.delete": "Шаблон графика удалён",
  "setup.complete": "Первичная настройка завершена",
  "shift.auto_close": "Смена закрыта автоматически",
  "user.create": "Пользователь создан",
  "user.reset_registration": "Регистрация пользователя сброшена",
  "user.role_update": "Роль пользователя изменена",
  "user.password_update": "Пароль пользователя изменён",
  "user.delete": "Пользователь удалён",
  "user.block": "Пользователь заблокирован",
  "user.unblock": "Пользователь разблокирован",
};

const auditEntities: Record<string, string> = {
  employee: "Сотрудник",
  mark: "Отметка",
  qr_token: "QR-токен",
  system: "Система",
  user: "Пользователь",
  delegation: "Делегирование",
  notification: "Уведомление",
  oauth_client: "OAuth-клиент",
  ip_whitelist: "Список разрешённых IP",
  office: "Офис",
  department: "Отдел",
  position: "Должность",
  schedule: "График",
  shift: "Смена",
  mobile_build: "Сборка приложения",
  offline_import: "Офлайн-импорт",
  settings: "Настройки",
};

function translate(values: Record<string, string>, value: string | null | undefined): string {
  if (!value) return "—";
  return values[value] ?? value;
}

export const roleLabel = (value: string | null | undefined) => translate(roles, value);
export const eventLabel = (value: string | null | undefined) => translate(events, value);
export const qrSourceLabel = (value: string | null | undefined) => translate(qrSources, value);
export const clientTypeLabel = (value: string | null | undefined) => translate(clientTypes, value);
export const shiftStatusLabel = (value: string | null | undefined) => translate(shiftStatuses, value);
export const delegationStatusLabel = (value: string | null | undefined) => translate(delegationStatuses, value);
export const tokenStatusLabel = (value: string | null | undefined) => translate(tokenStatuses, value);
export const patternLabel = (value: string | null | undefined) => translate(patterns, value);
export const platformLabel = (value: string | null | undefined) => translate(platforms, value);
export const notificationTypeLabel = (value: string | null | undefined) => translate(notificationTypes, value);
export const auditActionLabel = (value: string | null | undefined) => translate(auditActions, value);
export const auditEntityLabel = (value: string | null | undefined) => translate(auditEntities, value);
export const auditActionOptions = Object.entries(auditActions).sort((a, b) => a[1].localeCompare(b[1], "ru"));
export const auditEntityOptions = Object.entries(auditEntities).sort((a, b) => a[1].localeCompare(b[1], "ru"));

const auditFields: Record<string, string> = {
  action: "Действие",
  actor_user_id: "Пользователь",
  auto_closed_previous: "Автоматическое закрытие",
  cidr: "IP-адрес или подсеть",
  client_id: "Идентификатор клиента",
  client_type: "Тип клиента",
  closed_at: "Время закрытия",
  count: "Количество",
  delegation_id: "Делегирование",
  department: "Отдел",
  department_id: "Отдел",
  employee_id: "Сотрудник",
  employee_number: "Табельный номер",
  entity_id: "Идентификатор объекта",
  event_type: "Тип отметки",
  filename: "Файл",
  fired_at: "Дата увольнения",
  full_name: "ФИО",
  from: "Начало периода",
  from_user_id: "Кто передаёт",
  hired_at: "Дата приёма",
  ip: "IP-адрес",
  is_active: "Активен",
  lat: "Широта",
  lon: "Долгота",
  name: "Название",
  new_role: "Новая роль",
  old_role: "Прежняя роль",
  outside_office: "Вне офиса",
  overtime: "Переработка",
  platform: "Платформа",
  position: "Должность",
  position_id: "Должность",
  qr_data_mismatch: "Расхождение данных QR",
  qr_source: "Источник QR",
  reason: "Причина",
  removed: "Удалено записей",
  role: "Роль",
  rows_error: "Строк с ошибками",
  rows_ok: "Успешных строк",
  rows_total: "Всего строк",
  seeded: "Загружено записей",
  shift_id: "Смена",
  size: "Размер, байт",
  to: "Конец периода",
  to_user_id: "Кому передаётся",
  valid_until: "Действует до",
  version: "Версия",
};

export const auditFieldLabel = (value: string) => auditFields[value] ?? value;

export function auditValueLabel(key: string, value: unknown): string {
  if (value === null || value === undefined || value === "") return "—";
  if (typeof value === "boolean") return value ? "да" : "нет";
  if (key === "role" || key.endsWith("_role")) return roleLabel(String(value));
  if (key === "event_type") return eventLabel(String(value));
  if (key === "qr_source") return qrSourceLabel(String(value));
  if (key === "client_type") return clientTypeLabel(String(value));
  if (key === "platform") return platformLabel(String(value));
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}
