import { useCallback, useEffect, useState } from "react";
import type { FormEvent } from "react";
import { api, errorText } from "./api";
import { patternLabel, roleLabel } from "./labels";

type Message = { kind: "error" | "ok"; text: string } | null;

export function useList<T>(path: string) {
  const [items, setItems] = useState<T[]>([]);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<Message>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    const res = await api<T[]>(path);
    if (res.ok && Array.isArray(res.body)) {
      setItems(res.body);
      setMessage(null);
    } else {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось загрузить данные") });
    }
    setLoading(false);
  }, [path]);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { items, loading, message, setMessage, reload };
}

export function Notice({ message }: { message: Message }) {
  if (!message) return null;
  return <p className={message.kind === "ok" ? "ok" : "error"}>{message.text}</p>;
}

export function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label>
      {label}
      {children}
    </label>
  );
}

export function fmt(value: string | null | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("ru-RU");
}

export function fmtDate(value: string | null | undefined): string {
  if (!value) return "—";
  const date = new Date(`${value}T00:00:00`);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString("ru-RU");
}

/* ---------------- Сотрудники ---------------- */

type Employee = {
  id: string;
  employee_number: string;
  full_name: string;
  last_name: string | null;
  first_name: string | null;
  middle_name: string | null;
  email: string | null;
  phone: string | null;
  position: string | null;
  department_id: string | null;
  position_id: string | null;
  is_active: boolean;
  hired_at: string | null;
  fired_at: string | null;
};

const emptyEmployee = {
  employee_number: "",
  last_name: "",
  first_name: "",
  middle_name: "",
  email: "",
  phone: "",
  department_id: "",
  position_id: "",
  is_active: true,
};

type Department = { id: string; name: string; is_active: boolean };
type Position = { id: string; name: string; is_active: boolean };

const employeesPageSize = 20;

export function Employees() {
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const path = `/admin/employees?limit=${employeesPageSize}&offset=${page * employeesPageSize}&q=${encodeURIComponent(search)}`;
  const { items, loading, message, setMessage, reload } = useList<Employee>(path);
  const departments = useList<Department>("/admin/departments");
  const positions = useList<Position>("/admin/positions");
  const [form, setForm] = useState({ ...emptyEmployee });
  const [editing, setEditing] = useState<string>("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    const payload = {
      employee_number: form.employee_number.trim(),
      last_name: form.last_name.trim(),
      first_name: form.first_name.trim(),
      middle_name: form.middle_name.trim(),
      email: form.email.trim() || null,
      phone: form.phone.trim() || null,
      department_id: form.department_id || null,
      position_id: form.position_id || null,
      is_active: form.is_active,
    };
    const res = editing
      ? await api(`/admin/employees/${editing}`, { method: "PATCH", body: JSON.stringify(payload) })
      : await api("/admin/employees", { method: "POST", body: JSON.stringify(payload) });
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось сохранить сотрудника") });
      return;
    }
    setMessage({ kind: "ok", text: editing ? "Сотрудник обновлён" : "Сотрудник создан" });
    setForm({ ...emptyEmployee });
    setEditing("");
    await reload();
  }

  function edit(employee: Employee) {
    setEditing(employee.id);
    setForm({
      employee_number: employee.employee_number,
      last_name: employee.last_name ?? employee.full_name.split(/\s+/)[0] ?? "",
      first_name: employee.first_name ?? employee.full_name.split(/\s+/)[1] ?? "",
      middle_name: employee.middle_name ?? employee.full_name.split(/\s+/).slice(2).join(" "),
      email: employee.email ?? "",
      phone: employee.phone ?? "",
      department_id: employee.department_id ?? "",
      position_id: employee.position_id ?? "",
      is_active: employee.is_active,
    });
    window.scrollTo({ top: 0, behavior: "smooth" });
  }

  function applySearch(event: FormEvent) {
    event.preventDefault();
    setPage(0);
    setSearch(query.trim());
  }

  return (
    <section>
      <h2>Сотрудники</h2>
      <Notice message={message} />
      <form className="grid employee-form" onSubmit={submit}>
        <Field label="Табельный номер">
          <input value={form.employee_number} onChange={(e) => setForm({ ...form, employee_number: e.target.value })} required />
        </Field>
        <Field label="Фамилия">
          <input value={form.last_name} onChange={(e) => setForm({ ...form, last_name: e.target.value })} required />
        </Field>
        <Field label="Имя">
          <input value={form.first_name} onChange={(e) => setForm({ ...form, first_name: e.target.value })} required />
        </Field>
        <Field label="Отчество">
          <input value={form.middle_name} onChange={(e) => setForm({ ...form, middle_name: e.target.value })} />
        </Field>
        <Field label="Отдел">
          <select value={form.department_id} onChange={(e) => setForm({ ...form, department_id: e.target.value })}>
            <option value="">— не указан —</option>
            {departments.items.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </Field>
        <Field label="Должность">
          <select
            value={form.position_id}
            onChange={(e) => setForm({ ...form, position_id: e.target.value })}
            required
          >
            <option value="">— выберите из справочника —</option>
            {positions.items.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </Field>
        <Field label="Email">
          <input type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
        </Field>
        <Field label="Телефон">
          <input value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} />
        </Field>
        <Field label="Активен">
          <input type="checkbox" checked={form.is_active} onChange={(e) => setForm({ ...form, is_active: e.target.checked })} />
        </Field>
        <div className="actions">
          <button type="submit">{editing ? "Сохранить" : "Добавить"}</button>
          {editing && (
            <button type="button" onClick={() => { setEditing(""); setForm({ ...emptyEmployee }); }}>
              Отмена
            </button>
          )}
        </div>
      </form>

      <form className="toolbar" onSubmit={applySearch}>
        <input placeholder="Поиск по номеру или ФИО" value={query} onChange={(e) => setQuery(e.target.value)} />
        <button type="submit" disabled={loading}>Найти</button>
        <button type="button" onClick={() => { setQuery(""); setSearch(""); setPage(0); }}>Сбросить</button>
        <button type="button" onClick={() => void reload()} disabled={loading}>Обновить</button>
      </form>

      <table>
        <thead>
          <tr><th>Номер</th><th>ФИО</th><th>Отдел</th><th>Должность</th><th>Телефон</th><th>Статус</th><th /></tr>
        </thead>
        <tbody>
          {items.map((employee) => (
            <tr key={employee.id}>
              <td>{employee.employee_number}</td>
              <td>{employee.full_name}</td>
              <td>{departments.items.find((item) => item.id === employee.department_id)?.name ?? "—"}</td>
              <td>{positions.items.find((item) => item.id === employee.position_id)?.name ?? "—"}</td>
              <td>{employee.phone ?? "—"}</td>
              <td>{employee.is_active ? "активен" : "неактивен"}</td>
              <td><button type="button" onClick={() => edit(employee)}>Изменить</button></td>
            </tr>
          ))}
          {!loading && items.length === 0 && <tr><td colSpan={7}>Сотрудников нет</td></tr>}
        </tbody>
      </table>
      <div className="toolbar">
        <button type="button" disabled={page === 0 || loading} onClick={() => setPage((value) => Math.max(0, value - 1))}>Назад</button>
        <span>Страница {page + 1}</span>
        <button type="button" disabled={items.length < employeesPageSize || loading} onClick={() => setPage((value) => value + 1)}>Вперёд</button>
      </div>
      {loading && <p>Загрузка…</p>}
    </section>
  );
}

/* ---------------- Офисы ---------------- */

type Office = {
  id: string;
  name: string;
  lat: number;
  lon: number;
  radius_m: number;
  is_active: boolean;
};

export function Offices() {
  const { items, loading, message, setMessage, reload } = useList<Office>("/admin/offices");
  const [form, setForm] = useState({ name: "", lat: "", lon: "", radius_m: "150" });
  const [editing, setEditing] = useState("");
  const [busy, setBusy] = useState(false);

  function startEdit(office: Office) {
    setEditing(office.id);
    setForm({
      name: office.name,
      lat: String(office.lat),
      lon: String(office.lon),
      radius_m: String(office.radius_m),
    });
    setMessage(null);
  }

  function resetForm() {
    setEditing("");
    setForm({ name: "", lat: "", lon: "", radius_m: "150" });
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    const body = {
      name: form.name.trim(),
      lat: Number(form.lat),
      lon: Number(form.lon),
      radius_m: Number(form.radius_m),
    };
    const res = editing
      ? await api(`/admin/offices/${editing}`, { method: "PATCH", body: JSON.stringify(body) })
      : await api("/admin/offices", { method: "POST", body: JSON.stringify(body) });
    setBusy(false);
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, editing ? "не удалось сохранить офис" : "не удалось создать офис") });
      return;
    }
    setMessage({ kind: "ok", text: editing ? "Офис сохранён" : "Офис создан" });
    resetForm();
    await reload();
  }

  async function remove(office: Office) {
    if (!confirm(`Удалить офис «${office.name}»?`)) return;
    const res = await api(`/admin/offices/${office.id}`, { method: "DELETE" });
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось удалить офис") });
      return;
    }
    setMessage({ kind: "ok", text: `Офис «${office.name}» удалён` });
    if (editing === office.id) resetForm();
    await reload();
  }

  return (
    <section>
      <h2>Офисы</h2>
      <Notice message={message} />
      <form className="grid" onSubmit={submit}>
        <Field label="Название"><input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required /></Field>
        <Field label="Широта"><input type="number" step="any" value={form.lat} onChange={(e) => setForm({ ...form, lat: e.target.value })} required /></Field>
        <Field label="Долгота"><input type="number" step="any" value={form.lon} onChange={(e) => setForm({ ...form, lon: e.target.value })} required /></Field>
        <Field label="Радиус, м"><input type="number" min="1" value={form.radius_m} onChange={(e) => setForm({ ...form, radius_m: e.target.value })} required /></Field>
        <div className="form-actions">
          <button type="submit" disabled={busy}>{busy ? "Сохранение…" : editing ? "Сохранить офис" : "Добавить офис"}</button>
          {editing && <button type="button" className="secondary" onClick={resetForm}>Отмена</button>}
        </div>
      </form>
      <table>
        <thead><tr><th>Название</th><th>Координаты</th><th>Радиус</th><th>Статус</th><th /></tr></thead>
        <tbody>
          {items.map((office) => (
            <tr key={office.id}>
              <td>{office.name}</td>
              <td className="mono">{office.lat}, {office.lon}</td>
              <td>{office.radius_m} м</td>
              <td>{office.is_active ? "активен" : "неактивен"}</td>
              <td className="actions-cell">
                <button type="button" onClick={() => startEdit(office)}>Изменить</button>
                <button type="button" className="danger" onClick={() => void remove(office)}>Удалить</button>
              </td>
            </tr>
          ))}
          {!loading && items.length === 0 && <tr><td colSpan={5}>Офисов нет</td></tr>}
        </tbody>
      </table>
      {loading && <p>Загрузка…</p>}
    </section>
  );
}

/* ---------------- Пользователи ---------------- */

type User = {
  id: string;
  login: string;
  role: string;
  employee_id: string | null;
  is_locked: boolean;
  auth_blocked: boolean;
  is_main_admin: boolean;
  registered_at: string | null;
  last_login_at: string | null;
  full_name?: string | null;
  employee_number?: string | null;
};

const roleOptions = [
  ["employee", "Сотрудник"],
  ["operator", "Оператор"],
  ["admin", "Администратор"],
  ["shift_lead", "Руководитель смены"],
  ["director", "Директор"],
  ["hr", "Отдел кадров"],
] as const;

const usersPageSize = 20;

export function Users() {
  const [page, setPage] = useState(0);
  const path = `/admin/users?limit=${usersPageSize}&offset=${page * usersPageSize}`;
  const { items, loading, message, setMessage, reload } = useList<User>(path);
  const employees = useList<EmployeeLite>("/admin/employees?limit=500");
  const [roleDrafts, setRoleDrafts] = useState<Record<string, string>>({});
  const [login, setLogin] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("employee");
  const [employeeId, setEmployeeId] = useState("");
  const [creating, setCreating] = useState(false);
  const [passwordTarget, setPasswordTarget] = useState<User | null>(null);
  const [newPassword, setNewPassword] = useState("");
  const [passwordBusy, setPasswordBusy] = useState(false);

  async function action(user: User, name: "block" | "unblock" | "reset-registration") {
    const res = await api(`/admin/users/${user.id}/${name}`, { method: "POST" });
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось выполнить действие") });
      return;
    }
    setMessage({ kind: "ok", text: "Готово" });
    await reload();
  }

  async function saveRole(user: User) {
    const nextRole = roleDrafts[user.id] ?? user.role;
    if (nextRole === user.role) return;
    const res = await api(`/admin/users/${user.id}/role`, {
      method: "PATCH",
      body: JSON.stringify({ role: nextRole }),
    });
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось изменить роль") });
      return;
    }
    setMessage({ kind: "ok", text: `Роль пользователя «${user.login}» изменена на «${roleLabel(nextRole)}»` });
    setRoleDrafts((current) => ({ ...current, [user.id]: nextRole }));
    await reload();
  }

  async function removeUser(user: User) {
    if (!confirm(`Удалить пользователя «${user.login}»? Действие необратимо.`)) return;
    const res = await api(`/admin/users/${user.id}`, { method: "DELETE" });
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось удалить пользователя") });
      return;
    }
    setMessage({ kind: "ok", text: `Пользователь «${user.login}» удалён` });
    if (passwordTarget?.id === user.id) {
      setPasswordTarget(null);
      setNewPassword("");
    }
    await reload();
  }

  async function savePassword(event: FormEvent) {
    event.preventDefault();
    if (!passwordTarget) return;
    setPasswordBusy(true);
    const res = await api(`/admin/users/${passwordTarget.id}/password`, {
      method: "PATCH",
      body: JSON.stringify({ password: newPassword }),
    });
    setPasswordBusy(false);
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось изменить пароль") });
      return;
    }
    setMessage({ kind: "ok", text: `Пароль пользователя «${passwordTarget.login}» изменён, его сеансы завершены` });
    setPasswordTarget(null);
    setNewPassword("");
  }

  async function createUser(event: FormEvent) {
    event.preventDefault();
    setCreating(true);
    const body: Record<string, unknown> = { login: linkable ? "" : login, password, role };
    if (employeeId) body.employee_id = employeeId;
    const res = await api<{ login: string }>("/admin/users", { method: "POST", body: JSON.stringify(body) });
    setCreating(false);
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось создать пользователя") });
      return;
    }
    setMessage({ kind: "ok", text: `Пользователь «${res.body?.login ?? login}» создан с ролью «${roleLabel(role)}»` });
    setLogin("");
    setPassword("");
    setEmployeeId("");
    await reload();
  }

  const linkable = role === "employee" || role === "operator";

  return (
    <section>
      <h2>Пользователи</h2>
      <Notice message={message} />

      <form className="grid" onSubmit={createUser}>
        {linkable ? (
          <div className="hint">
            Логин будет сформирован автоматически: фамилия и первая буква имени.
            Если логин занят, добавляются следующие буквы имени.
          </div>
        ) : (
          <Field label="Логин">
            <input value={login} onChange={(e) => setLogin(e.target.value)} required />
          </Field>
        )}
        <Field label="Пароль"><input type="text" value={password} onChange={(e) => setPassword(e.target.value)} required /></Field>
        <Field label="Роль">
          <select value={role} onChange={(e) => {
            setRole(e.target.value);
            if (e.target.value === "employee" || e.target.value === "operator") {
              setLogin("");
            } else {
              setEmployeeId("");
            }
          }}>
            {roleOptions.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
        </Field>
        {linkable && (
          <Field label="Сотрудник">
            <select value={employeeId} onChange={(e) => setEmployeeId(e.target.value)} required>
              <option value="">— выберите сотрудника —</option>
              {employees.items.map((item) => (
                <option key={item.id} value={item.id}>{item.employee_number} · {item.full_name}</option>
              ))}
            </select>
          </Field>
        )}
        <div className="form-actions">
          <button type="submit" disabled={creating}>{creating ? "Создание…" : "Создать пользователя"}</button>
        </div>
      </form>

      {passwordTarget && (
        <form className="card" onSubmit={savePassword}>
          <h3>Новый пароль: {passwordTarget.login}</h3>
          <Field label="Пароль">
            <input
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              autoComplete="new-password"
              required
            />
          </Field>
          <p className="hint">После смены пароля все активные сеансы пользователя будут завершены.</p>
          <div className="actions">
            <button type="submit" disabled={passwordBusy}>{passwordBusy ? "Сохранение…" : "Установить пароль"}</button>
            <button type="button" className="secondary" onClick={() => { setPasswordTarget(null); setNewPassword(""); }}>Отмена</button>
          </div>
        </form>
      )}

      <table>
        <thead>
          <tr><th>Логин</th><th>Роль</th><th>Сотрудник</th><th>Блокировка</th><th>Регистрация</th><th /></tr>
        </thead>
        <tbody>
          {items.map((user) => (
            <tr key={user.id}>
              <td>{user.login}{user.is_main_admin ? " (главный администратор)" : ""}</td>
              <td>
                <select
                  value={roleDrafts[user.id] ?? user.role}
                  onChange={(e) => setRoleDrafts((current) => ({ ...current, [user.id]: e.target.value }))}
                  disabled={user.is_main_admin}
                >
                  {roleOptions.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                </select>
                {(roleDrafts[user.id] ?? user.role) !== user.role && (
                  <button type="button" onClick={() => void saveRole(user)}>Сохранить роль</button>
                )}
              </td>
              <td>{user.full_name ?? user.employee_number ?? "—"}</td>
              <td>{user.auth_blocked ? "запрет входа" : user.is_locked ? "заблокирован" : "нет"}</td>
              <td>{fmt(user.registered_at)}</td>
              <td className="actions-cell">
                {!user.is_main_admin && (
                  <>
                    {user.auth_blocked
                      ? <button type="button" onClick={() => void action(user, "unblock")}>Разблокировать</button>
                      : <button type="button" onClick={() => void action(user, "block")}>Заблокировать</button>}
                    <button type="button" onClick={() => { setPasswordTarget(user); setNewPassword(""); }}>Задать пароль</button>
                    <button type="button" onClick={() => void action(user, "reset-registration")}>Сбросить регистрацию</button>
                    <button type="button" className="danger" onClick={() => void removeUser(user)}>Удалить</button>
                  </>
                )}
              </td>
            </tr>
          ))}
          {!loading && items.length === 0 && <tr><td colSpan={6}>Пользователей нет</td></tr>}
        </tbody>
      </table>
      <div className="toolbar">
        <button type="button" disabled={page === 0 || loading} onClick={() => setPage((value) => Math.max(0, value - 1))}>Назад</button>
        <span>Страница {page + 1}</span>
        <button type="button" disabled={items.length < usersPageSize || loading} onClick={() => setPage((value) => value + 1)}>Вперёд</button>
        <button type="button" onClick={() => void reload()} disabled={loading}>Обновить</button>
      </div>
      {loading && <p>Загрузка…</p>}
    </section>
  );
}

/* ---------------- Графики ---------------- */

type Schedule = {
  id: string;
  employee_id: string;
  pattern_type: string;
  workdays: number[];
  start_time: string;
  end_time: string;
  valid_from: string;
  valid_to: string | null;
  is_active: boolean;
};

type EmployeeLite = { id: string; employee_number: string; full_name: string };

type ImportRow = { line: number; status: string; reason: string | null };
type ImportReport = { rows_total: number; rows_ok: number; rows_error: number; rows: ImportRow[] };

const weekDays = [
  [1, "Пн"],
  [2, "Вт"],
  [3, "Ср"],
  [4, "Чт"],
  [5, "Пт"],
  [6, "Сб"],
  [7, "Вс"],
] as const;

const patternOptions = [
  ["5/2", "5/2 (пн–пт)"],
  ["2/2", "2/2"],
  ["custom", "Индивидуальный"],
] as const;

function todayValue(): string {
  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}

function emptyTemplate() {
  return {
    name: "",
    pattern_type: "5/2",
    workdays: [1, 2, 3, 4, 5] as number[],
    start_time: "09:00",
    end_time: "18:00",
  };
}

function workdaysText(days: number[]): string {
  return days
    .slice()
    .sort((a, b) => a - b)
    .map((day) => weekDays.find(([value]) => value === day)?.[1] ?? String(day))
    .join(", ");
}

type ScheduleTemplate = {
  id: string;
  name: string;
  pattern_type: string;
  workdays: number[];
  start_time: string;
  end_time: string;
};

type DirectoryItem = { id: string; name: string };

type TargetType = "all" | "department" | "position" | "employee";

export function Schedules() {
  const schedules = useList<Schedule>("/admin/schedules?limit=500");
  const templates = useList<ScheduleTemplate>("/admin/schedule-templates");
  const employees = useList<EmployeeLite>("/admin/employees?limit=500");
  const departments = useList<DirectoryItem>("/admin/departments");
  const positions = useList<DirectoryItem>("/admin/positions");
  const [templateForm, setTemplateForm] = useState(() => emptyTemplate());
  const [editingTemplate, setEditingTemplate] = useState("");
  const [templateBusy, setTemplateBusy] = useState(false);
  const [assignForm, setAssignForm] = useState({
    template_id: "",
    target_type: "all" as TargetType,
    target_id: "",
    valid_from: todayValue(),
    valid_to: "",
  });
  const [assignBusy, setAssignBusy] = useState(false);
  const [message, setMessage] = useState<Message>(null);

  const employeeByID = new Map(employees.items.map((item) => [item.id, item]));
  const templateByID = new Map(templates.items.map((item) => [item.id, item]));

  function employeeText(id: string): string {
    const employee = employeeByID.get(id);
    return employee ? `${employee.employee_number} — ${employee.full_name}` : id;
  }

  function resetTemplate() {
    setEditingTemplate("");
    setTemplateForm(emptyTemplate());
  }

  function startEditTemplate(template: ScheduleTemplate) {
    setEditingTemplate(template.id);
    setTemplateForm({
      name: template.name,
      pattern_type: template.pattern_type,
      workdays: template.workdays,
      start_time: template.start_time,
      end_time: template.end_time,
    });
    setMessage(null);
  }

  function toggleTemplateDay(day: number, checked: boolean) {
    setTemplateForm((current) => {
      const days = new Set(current.workdays);
      if (checked) days.add(day);
      else days.delete(day);
      return { ...current, workdays: Array.from(days).sort((a, b) => a - b) };
    });
  }

  async function submitTemplate(event: FormEvent) {
    event.preventDefault();
    setTemplateBusy(true);
    const body = {
      name: templateForm.name.trim(),
      pattern_type: templateForm.pattern_type,
      workdays: templateForm.workdays,
      start_time: templateForm.start_time,
      end_time: templateForm.end_time,
    };
    const res = editingTemplate
      ? await api(`/admin/schedule-templates/${editingTemplate}`, { method: "PATCH", body: JSON.stringify(body) })
      : await api("/admin/schedule-templates", { method: "POST", body: JSON.stringify(body) });
    setTemplateBusy(false);
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, editingTemplate ? "не удалось сохранить график" : "не удалось создать график") });
      return;
    }
    setMessage({ kind: "ok", text: editingTemplate ? "График сохранён" : "График создан. Теперь его можно назначить." });
    resetTemplate();
    await templates.reload();
  }

  async function removeTemplate(template: ScheduleTemplate) {
    if (!confirm(`Удалить график «${template.name}»? Назначенные сотрудникам графики останутся.`)) return;
    const res = await api(`/admin/schedule-templates/${template.id}`, { method: "DELETE" });
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось удалить график") });
      return;
    }
    setMessage({ kind: "ok", text: "График удалён" });
    if (editingTemplate === template.id) resetTemplate();
    if (assignForm.template_id === template.id) setAssignForm((current) => ({ ...current, template_id: "" }));
    await templates.reload();
  }

  async function assignTemplate(event: FormEvent) {
    event.preventDefault();
    if (!assignForm.template_id) {
      setMessage({ kind: "error", text: "выберите график" });
      return;
    }
    if (assignForm.target_type !== "all" && !assignForm.target_id) {
      setMessage({ kind: "error", text: "выберите получателя графика" });
      return;
    }
    const template = templateByID.get(assignForm.template_id);
    const targetText = assignForm.target_type === "all"
      ? "всем активным сотрудникам"
      : assignForm.target_type === "department"
        ? `отделу «${departments.items.find((item) => item.id === assignForm.target_id)?.name ?? ""}»`
        : assignForm.target_type === "position"
          ? `должности «${positions.items.find((item) => item.id === assignForm.target_id)?.name ?? ""}»`
          : `сотруднику ${employeeText(assignForm.target_id)}`;
    if (!confirm(`Назначить график «${template?.name ?? ""}» ${targetText}? Текущий активный график будет заменён.`)) return;
    setAssignBusy(true);
    const res = await api<ImportReport>("/admin/schedule-templates/" + assignForm.template_id + "/assign", {
      method: "POST",
      body: JSON.stringify({
        target_type: assignForm.target_type,
        target_id: assignForm.target_type === "all" ? "" : assignForm.target_id,
        valid_from: assignForm.valid_from,
        valid_to: assignForm.valid_to || null,
      }),
    });
    setAssignBusy(false);
    if (!res.ok || !res.body) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось назначить график") });
      return;
    }
    setMessage({
      kind: res.body.rows_error > 0 ? "error" : "ok",
      text: `Назначено: ${res.body.rows_ok} из ${res.body.rows_total}, ошибок: ${res.body.rows_error}`,
    });
    await schedules.reload();
  }

  async function removeSchedule(schedule: Schedule) {
    if (!confirm(`Удалить назначенный график сотрудника ${employeeText(schedule.employee_id)}?`)) return;
    const res = await api(`/admin/schedules/${schedule.id}`, { method: "DELETE" });
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось удалить график") });
      return;
    }
    setMessage({ kind: "ok", text: "График удалён" });
    await schedules.reload();
  }

  function targetOptions() {
    switch (assignForm.target_type) {
      case "department":
        return departments.items.map((item) => [item.id, item.name] as const);
      case "position":
        return positions.items.map((item) => [item.id, item.name] as const);
      case "employee":
        return employees.items.map((item) => [item.id, `${item.employee_number} — ${item.full_name}`] as const);
      default:
        return [];
    }
  }

  return (
    <section>
      <h2>Графики</h2>
      <Notice message={message} />

      <div className="card">
        <h3>{editingTemplate ? "Изменить график" : "Новый график"}</h3>
        <p className="hint">Сначала задайте рабочие дни и время. Сотрудник при создании не выбирается — график назначается отдельно.</p>
        <form className="grid" onSubmit={submitTemplate}>
          <Field label="Название">
            <input value={templateForm.name} onChange={(e) => setTemplateForm({ ...templateForm, name: e.target.value })} placeholder="например, Офис 5/2" required />
          </Field>
          <Field label="Тип">
            <select
              value={templateForm.pattern_type}
              onChange={(e) => {
                const pattern = e.target.value;
                const workdays = pattern === "5/2" ? [1, 2, 3, 4, 5] : pattern === "2/2" ? [1, 2] : templateForm.workdays;
                setTemplateForm({ ...templateForm, pattern_type: pattern, workdays });
              }}
            >
              {patternOptions.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
            </select>
          </Field>
          <Field label="Начало работы"><input type="time" value={templateForm.start_time} onChange={(e) => setTemplateForm({ ...templateForm, start_time: e.target.value })} required /></Field>
          <Field label="Окончание работы"><input type="time" value={templateForm.end_time} onChange={(e) => setTemplateForm({ ...templateForm, end_time: e.target.value })} required /></Field>
          <Field label="Рабочие дни">
            <div className="days">
              {weekDays.map(([value, label]) => (
                <label key={value} className="day">
                  <input type="checkbox" checked={templateForm.workdays.includes(value)} onChange={(e) => toggleTemplateDay(value, e.target.checked)} />
                  {label}
                </label>
              ))}
            </div>
          </Field>
          <div className="form-actions">
            <button type="submit" disabled={templateBusy}>{templateBusy ? "Сохранение…" : editingTemplate ? "Сохранить график" : "Создать график"}</button>
            {editingTemplate && <button type="button" className="secondary" onClick={resetTemplate}>Отмена</button>}
          </div>
        </form>
      </div>

      <div className="card">
        <h3>Назначение графика</h3>
        <form className="grid" onSubmit={assignTemplate}>
          <Field label="График">
            <select value={assignForm.template_id} onChange={(e) => setAssignForm({ ...assignForm, template_id: e.target.value })} required>
              <option value="">— выберите график —</option>
              {templates.items.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name} · {item.start_time}–{item.end_time} · {workdaysText(item.workdays)}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Кому назначить">
            <select value={assignForm.target_type} onChange={(e) => setAssignForm({ ...assignForm, target_type: e.target.value as TargetType, target_id: "" })}>
              <option value="all">Всем сотрудникам</option>
              <option value="department">Отделу</option>
              <option value="position">Должности</option>
              <option value="employee">Одному сотруднику</option>
            </select>
          </Field>
          {assignForm.target_type !== "all" && (
            <Field label={assignForm.target_type === "department" ? "Отдел" : assignForm.target_type === "position" ? "Должность" : "Сотрудник"}>
              <select value={assignForm.target_id} onChange={(e) => setAssignForm({ ...assignForm, target_id: e.target.value })} required>
                <option value="">— выберите —</option>
                {targetOptions().map(([value, label]) => <option key={value} value={value}>{label}</option>)}
              </select>
            </Field>
          )}
          <Field label="Действует с"><input type="date" value={assignForm.valid_from} onChange={(e) => setAssignForm({ ...assignForm, valid_from: e.target.value })} required /></Field>
          <Field label="Действует по (необязательно)"><input type="date" value={assignForm.valid_to} onChange={(e) => setAssignForm({ ...assignForm, valid_to: e.target.value })} /></Field>
          <div className="form-actions">
            <button type="submit" disabled={assignBusy || templates.items.length === 0}>{assignBusy ? "Назначение…" : "Назначить график"}</button>
          </div>
        </form>
      </div>

      <h3>Шаблоны графиков</h3>
      <table>
        <thead><tr><th>Название</th><th>Тип</th><th>Дни</th><th>Время</th><th /></tr></thead>
        <tbody>
          {templates.items.map((template) => (
            <tr key={template.id}>
              <td>{template.name}</td>
              <td>{patternLabel(template.pattern_type)}</td>
              <td>{workdaysText(template.workdays)}</td>
              <td>{template.start_time}–{template.end_time}</td>
              <td className="actions-cell">
                <button type="button" onClick={() => setAssignForm((current) => ({ ...current, template_id: template.id }))}>Назначить</button>
                <button type="button" onClick={() => startEditTemplate(template)}>Изменить</button>
                <button type="button" className="danger" onClick={() => void removeTemplate(template)}>Удалить</button>
              </td>
            </tr>
          ))}
          {!templates.loading && templates.items.length === 0 && <tr><td colSpan={5}>Графиков пока нет</td></tr>}
        </tbody>
      </table>

      <h3>Назначенные графики</h3>
      <table>
        <thead><tr><th>Сотрудник</th><th>Тип</th><th>Дни</th><th>Время</th><th>Срок действия</th><th>Статус</th><th /></tr></thead>
        <tbody>
          {schedules.items.map((schedule) => (
            <tr key={schedule.id}>
              <td>{employeeText(schedule.employee_id)}</td>
              <td>{patternLabel(schedule.pattern_type)}</td>
              <td>{workdaysText(schedule.workdays)}</td>
              <td>{schedule.start_time}–{schedule.end_time}</td>
              <td>{fmtDate(schedule.valid_from)} — {schedule.valid_to ? fmtDate(schedule.valid_to) : "бессрочно"}</td>
              <td>{schedule.is_active ? "активен" : "неактивен"}</td>
              <td className="actions-cell">
                <button type="button" className="danger" onClick={() => void removeSchedule(schedule)}>Удалить</button>
              </td>
            </tr>
          ))}
          {!schedules.loading && schedules.items.length === 0 && <tr><td colSpan={7}>Графиков пока нет</td></tr>}
        </tbody>
      </table>
      {schedules.loading && <p>Загрузка…</p>}
    </section>
  );
}
