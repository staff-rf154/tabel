import { useState } from "react";
import type { FormEvent } from "react";
import { api, errorText } from "./api";
import { Field, Notice, fmt, useList } from "./sections";
import { platformLabel, roleLabel } from "./labels";

type Message = { kind: "error" | "ok"; text: string } | null;

/* ---------------- Создание пользователя ---------------- */

const roles = [
  ["employee", "Сотрудник"],
  ["operator", "Оператор"],
  ["shift_lead", "Руководитель смены"],
  ["director", "Директор"],
  ["hr", "Отдел кадров"],
  ["admin", "Администратор"],
] as const;

type EmployeeLite = { id: string; employee_number: string; full_name: string };

export function UserCreate() {
  const employees = useList<EmployeeLite>("/admin/employees?limit=500");
  const [login, setLogin] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("employee");
  const [employeeId, setEmployeeId] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<Message>(null);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setMessage(null);
    const body: Record<string, unknown> = { login, password, role };
    if (employeeId) body.employee_id = employeeId;
    const res = await api("/admin/users", { method: "POST", body: JSON.stringify(body) });
    setBusy(false);
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось создать пользователя") });
      return;
    }
    setMessage({ kind: "ok", text: `Пользователь «${login}» создан с ролью «${roleLabel(role)}»` });
    setLogin("");
    setPassword("");
    setEmployeeId("");
  }

  const linkable = role === "employee" || role === "operator";

  return (
    <section>
      <h2>Создание пользователя</h2>
      <p className="hint">Логин и пароль задаёт администратор. Роли «Руководитель смены», «Директор» и «Отдел кадров» получают доступ к отчётам за предыдущий и текущий месяц.</p>
      <form className="grid" onSubmit={submit}>
        <Field label="Логин"><input value={login} onChange={(e) => setLogin(e.target.value)} required /></Field>
        <Field label="Пароль"><input type="text" value={password} onChange={(e) => setPassword(e.target.value)} required /></Field>
        <Field label="Роль">
          <select value={role} onChange={(e) => { setRole(e.target.value); if (e.target.value !== "employee" && e.target.value !== "operator") setEmployeeId(""); }}>
            {roles.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
        </Field>
        {linkable && (
          <Field label="Сотрудник (необязательно)">
            <select value={employeeId} onChange={(e) => setEmployeeId(e.target.value)}>
              <option value="">— не привязывать —</option>
              {employees.items.map((item) => (
                <option key={item.id} value={item.id}>{item.employee_number} · {item.full_name}</option>
              ))}
            </select>
          </Field>
        )}
        <div className="form-actions">
          <button type="submit" disabled={busy}>{busy ? "Создание…" : "Создать пользователя"}</button>
        </div>
      </form>
      <Notice message={message} />
    </section>
  );
}

/* ---------------- Импорт данных ---------------- */

type ImportRow = { line: number; status: string; reason: string | null };
type ImportReport = { rows_total: number; rows_ok: number; rows_error: number; rows: ImportRow[] };

function FileImportCard({
  title,
  description,
  endpoint,
  template,
  previewable = true,
}: {
  title: string;
  description: string;
  endpoint: string;
  template: string;
  previewable?: boolean;
}) {
  const [file, setFile] = useState<File | null>(null);
  const [report, setReport] = useState<ImportReport | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<Message>(null);

  async function run(mode: "preview" | "commit") {
    if (!file) {
      setMessage({ kind: "error", text: "выберите CSV-файл" });
      return;
    }
    setBusy(true);
    setMessage(null);
    const form = new FormData();
    form.append("file", file);
    const suffix = previewable ? `?mode=${mode}` : "";
    const res = await api<ImportReport>(`${endpoint}${suffix}`, { method: "POST", body: form });
    setBusy(false);
    if (!res.ok || !res.body) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось обработать файл") });
      return;
    }
    setReport(res.body);
    setMessage({
      kind: res.body.rows_error > 0 ? "error" : "ok",
      text: `${mode === "preview" ? "Проверка" : "Импорт"} завершён: всего ${res.body.rows_total}, успешно ${res.body.rows_ok}, ошибок ${res.body.rows_error}`,
    });
  }

  const problems = report?.rows.filter((row) => row.status !== "ok") ?? [];

  return (
    <div className="card">
      <h3>{title}</h3>
      <p className="hint">
        {description} Шаблон: <a href={`/templates/${template}`} download>{template}</a>
      </p>
      <Field label="CSV-файл">
        <input
          type="file"
          accept=".csv,text/csv"
          onChange={(e) => {
            setFile(e.target.files?.[0] ?? null);
            setReport(null);
            setMessage(null);
          }}
        />
      </Field>
      <div className="actions">
        {previewable && (
          <button type="button" className="secondary" onClick={() => void run("preview")} disabled={busy || !file}>
            Проверить
          </button>
        )}
        <button type="button" onClick={() => void run("commit")} disabled={busy || !file}>
          {busy ? "Обработка…" : "Импортировать"}
        </button>
      </div>
      <Notice message={message} />
      {problems.length > 0 && (
        <table>
          <thead><tr><th>Строка</th><th>Причина</th></tr></thead>
          <tbody>
            {problems.map((row) => (
              <tr key={row.line}><td>{row.line}</td><td>{row.reason ?? "ошибка"}</td></tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

export function Imports() {
  return (
    <section>
      <h2>Импорт данных</h2>
      <p className="hint">
        Все массовые загрузки и шаблоны собраны на одной странице. Сначала проверьте файл,
        затем выполните импорт. Для отделов и должностей справочники должны быть заполнены
        до импорта сотрудников.
      </p>
      <div className="imports-grid">
        <FileImportCard
          title="Сотрудники"
          description="Колонки: employee_number, last_name, first_name, middle_name, position, department, email, phone, hired_at. Вместо отдельных колонок ФИО можно использовать full_name."
          endpoint="/admin/employees/import"
          template="employees.csv"
        />
        <FileImportCard
          title="Графики"
          description="Колонки: employee_number, pattern_type, workdays, start_time, end_time, valid_from, valid_to."
          endpoint="/admin/schedules/import"
          template="schedules.csv"
        />
        <FileImportCard
          title="Офлайн-отметки"
          description="Колонки: employee_number, event_type, timestamp_server, idempotency_key, timestamp_client, lat, lon, qr_source, client_type, operator_number."
          endpoint="/admin/offline-imports"
          template="offline_marks.csv"
          previewable={false}
        />
        <FileImportCard
          title="Отделы"
          description="CSV с колонкой name. Первая строка — заголовок."
          endpoint="/admin/departments/import"
          template="departments.csv"
        />
        <FileImportCard
          title="Должности"
          description="CSV с колонкой name. Первая строка — заголовок."
          endpoint="/admin/positions/import"
          template="positions.csv"
        />
        <div className="card">
          <h3>Шаблоны загрузки данных</h3>
          <TemplateLinks />
        </div>
      </div>
    </section>
  );
}

/* ---------------- Демо-данные ---------------- */
/* ---------------- Демо-данные ---------------- */

export function DemoPanel() {
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<Message>(null);

  async function run(path: string, successText: string) {
    setBusy(true);
    setMessage(null);
    const res = await api<{ seeded?: number; removed?: number }>(path, { method: "POST", body: JSON.stringify({}) });
    setBusy(false);
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "операция не удалась") });
      return;
    }
    setMessage({ kind: "ok", text: successText });
  }

  return (
    <section>
      <h2>Демонстрационная база</h2>
      <p className="hint">Загружает 10 демонстрационных сотрудников с телефонами, почтой, отделами, должностями, графиками и закрытыми сменами за последние 3 недели. Кнопка очистки удаляет демо-данные и демонстрационные учётные записи, кроме главного администратора.</p>
      <div className="form-actions">
        <button type="button" disabled={busy} onClick={() => void run("/admin/demo/seed", "Демо-данные загружены (10 сотрудников)")}>Загрузить демо-данные</button>
        <button type="button" className="danger" disabled={busy} onClick={() => { if (confirm("Удалить все демо-данные?")) void run("/admin/demo/clear", "Демо-данные удалены"); }}>Очистить демо-базу</button>
      </div>
      <Notice message={message} />
    </section>
  );
}

/* ---------------- Мобильные сборки ---------------- */

type MobileBuild = {
  id: string;
  platform: string;
  version: string;
  filename: string;
  size_bytes: number;
  is_active: boolean;
  created_at: string;
};

function humanSize(bytes: number): string {
  if (bytes > 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} МБ`;
  if (bytes > 1024) return `${(bytes / 1024).toFixed(0)} КБ`;
  return `${bytes} Б`;
}

export function MobileBuilds() {
  const { items, loading, message, setMessage, reload } = useList<MobileBuild>("/admin/mobile-builds");
  const [platform, setPlatform] = useState("android");
  const [version, setVersion] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);

  async function upload(event: FormEvent) {
    event.preventDefault();
    if (!file) {
      setMessage({ kind: "error", text: "выберите файл сборки (.apk или .ipa)" });
      return;
    }
    setBusy(true);
    const form = new FormData();
    form.append("platform", platform);
    form.append("version", version);
    form.append("file", file);
    const res = await api("/admin/mobile-builds", { method: "POST", body: form });
    setBusy(false);
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось загрузить сборку") });
      return;
    }
    setVersion("");
    setFile(null);
    await reload();
  }

  async function remove(id: string) {
    if (!confirm("Удалить сборку?")) return;
    const res = await api(`/admin/mobile-builds/${id}`, { method: "DELETE" });
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось удалить сборку") });
      return;
    }
    await reload();
  }

  return (
    <section>
      <h2>Мобильное приложение</h2>
      <p className="hint">Сборки публикуются здесь и доступны сотрудникам по адресу <code>/api/mobile-builds</code>. Загрузка выполняется прямо из панели администратора.</p>
      <form className="grid" onSubmit={upload}>
        <Field label="Платформа">
          <select value={platform} onChange={(e) => setPlatform(e.target.value)}>
            <option value="android">Android (.apk)</option>
            <option value="ios">iOS (.ipa)</option>
          </select>
        </Field>
        <Field label="Версия"><input value={version} onChange={(e) => setVersion(e.target.value)} placeholder="1.0.0" required /></Field>
        <Field label="Файл сборки"><input type="file" accept={platform === "android" ? ".apk" : ".ipa"} onChange={(e) => setFile(e.target.files?.[0] ?? null)} /></Field>
        <div className="form-actions">
          <button type="submit" disabled={busy}>{busy ? "Загрузка…" : "Загрузить сборку"}</button>
        </div>
      </form>
      <Notice message={message} />
      <table>
        <thead><tr><th>Платформа</th><th>Версия</th><th>Файл</th><th>Размер</th><th>Загружено</th><th></th></tr></thead>
        <tbody>
          {items.map((build) => (
            <tr key={build.id}>
              <td>{platformLabel(build.platform)}</td>
              <td>{build.version}</td>
              <td>{build.filename}</td>
              <td>{humanSize(build.size_bytes)}</td>
              <td>{fmt(build.created_at)}</td>
              <td>
                <a href={`/api/mobile-builds/${build.id}/download`} download>Скачать</a>{" "}
                <button type="button" className="danger small" onClick={() => void remove(build.id)}>Удалить</button>
              </td>
            </tr>
          ))}
          {!loading && items.length === 0 && <tr><td colSpan={6}>Сборок пока нет</td></tr>}
        </tbody>
      </table>
    </section>
  );
}

/* ---------------- Шаблоны ---------------- */

function TemplateLinks() {
  const files = [
    ["employees.csv", "Сотрудники"],
    ["schedules.csv", "Графики"],
    ["offline_marks.csv", "Офлайн-отметки"],
    ["departments.csv", "Отделы"],
    ["positions.csv", "Должности"],
    ["qr_bulk.json", "Массовые QR-токены"],
    ["README.md", "Описание форматов"],
  ] as const;
  return (
    <ul className="links">
      {files.map(([name, label]) => (
        <li key={name}><a href={`/templates/${name}`} download>{label} — {name}</a></li>
      ))}
    </ul>
  );
}

export function Templates() {
  return (
    <section>
      <h2>Шаблоны загрузки данных</h2>
      <p className="hint">Все шаблоны лежат в папке <code>templates/</code> репозитория и доступны для скачивания.</p>
      <TemplateLinks />
    </section>
  );
}
