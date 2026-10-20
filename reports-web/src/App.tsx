import { useCallback, useEffect, useState } from "react";
import { api, clearTokens, downloadFile, errorText, getToken, login } from "./api";

type Mark = {
  employee_number: string;
  full_name: string;
  position?: string | null;
  event_type: string;
  timestamp_server: string;
  overtime: boolean;
  outside_office: boolean;
  auto_closed: boolean;
  operator_number?: string | null;
  timing_status?: string;
};

type Shift = {
  employee_number: string;
  full_name: string;
  started_at: string;
  ended_at?: string | null;
  status: string;
  overtime: boolean;
  worked_minutes?: number | null;
};

type Summary = {
  employee_number: string;
  full_name: string;
  position?: string | null;
  shifts_total: number;
  shifts_closed: number;
  worked_minutes: number;
  overtime_shifts: number;
  marks_total: number;
};

type Tab = "shifts" | "marks" | "summary";

function money(minutes: number): string {
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  return `${hours} ч ${rest} мин`;
}

function fmt(value: string | null | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  return date.toLocaleString("ru-RU", { timeZone: "Asia/Novosibirsk" });
}

function shiftStatusLabel(value: string): string {
  switch (value) {
    case "open": return "Открыта";
    case "closed": return "Закрыта";
    case "auto_closed": return "Закрыта автоматически";
    default: return value;
  }
}

function localInput(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

export default function App() {
  const [loggedIn, setLoggedIn] = useState<boolean>(() => getToken() !== "");
  const [loginValue, setLoginValue] = useState("");
  const [password, setPassword] = useState("");
  const [loginError, setLoginError] = useState("");
  const [busy, setBusy] = useState(false);

  const [tab, setTab] = useState<Tab>("shifts");
  const [from, setFrom] = useState(() => {
    const now = new Date();
    return localInput(new Date(now.getFullYear(), now.getMonth(), 1));
  });
  const [to, setTo] = useState(() => localInput(new Date()));
  const [position, setPosition] = useState("");
  const [employeeIds, setEmployeeIds] = useState("");
  const [marks, setMarks] = useState<Mark[]>([]);
  const [shifts, setShifts] = useState<Shift[]>([]);
  const [summary, setSummary] = useState<Summary[]>([]);
  const [loadError, setLoadError] = useState("");
  const [loading, setLoading] = useState(false);

  const query = useCallback(() => {
    const params = new URLSearchParams();
    params.set("from", new Date(`${from}T00:00:00+03:00`).toISOString());
    params.set("to", new Date(`${to}T23:59:59+03:00`).toISOString());
    if (position.trim()) params.set("position", position.trim());
    if (employeeIds.trim()) params.set("employee_ids", employeeIds.trim());
    return params.toString();
  }, [from, to, position, employeeIds]);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError("");
    const path = tab === "shifts" ? "/reports/shifts" : tab === "marks" ? "/reports/marks" : "/reports/summary";
    const res = await api<unknown>(`${path}?${query()}`);
    setLoading(false);
    if (!res.ok) {
      setLoadError(errorText(res.body, "не удалось загрузить отчёт"));
      return;
    }
    if (tab === "shifts") setShifts((res.body as Shift[]) ?? []);
    else if (tab === "marks") setMarks((res.body as Mark[]) ?? []);
    else setSummary((res.body as Summary[]) ?? []);
  }, [tab, query]);

  useEffect(() => {
    if (loggedIn && tab) void load();
  }, [loggedIn, tab, load]);

  async function doLogin(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setLoginError("");
    const res = await login(loginValue, password);
    setBusy(false);
    if (!res.ok) {
      setLoginError(errorText(res.body, "не удалось войти"));
      return;
    }
    setLoggedIn(true);
  }

  async function doExport() {
    const path = `${tab === "shifts" ? "/reports/shifts" : tab === "marks" ? "/reports/marks" : "/reports/summary"}?${query()}&format=csv`;
    const name = tab === "shifts" ? "shifts-report.csv" : tab === "marks" ? "marks-report.csv" : "summary-report.csv";
    const err = await downloadFile(path, name);
    if (err) setLoadError(err);
  }

  function logout() {
    clearTokens();
    setLoggedIn(false);
    setMarks([]);
    setShifts([]);
    setSummary([]);
  }

  if (!loggedIn) {
    return (
      <div className="app">
        <h1>Табель — отчёты</h1>
        <p>Доступ для ролей: руководитель смены, директор, отдел кадров, администратор.</p>
        <form className="login" onSubmit={doLogin}>
          <label>
            Логин
            <input value={loginValue} onChange={(e) => setLoginValue(e.target.value)} autoComplete="username" />
          </label>
          <label>
            Пароль
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
          </label>
          {loginError && <div className="error">{loginError}</div>}
          <button type="submit" disabled={busy}>Войти</button>
        </form>
      </div>
    );
  }

  return (
    <div className="app">
      <div className="top">
        <div>
          <h1>Табель — отчёты</h1>
          <span className="note">Период ограничен предыдущим и текущим месяцем.</span>
        </div>
        <div>
          <span className="who">{loginValue || "пользователь"}</span>
          <button onClick={logout}>Выйти</button>
        </div>
      </div>

      <div className="filters">
        <label>Начало<input type="date" value={from} onChange={(e) => setFrom(e.target.value)} /></label>
        <label>Окончание<input type="date" value={to} onChange={(e) => setTo(e.target.value)} /></label>
        <label>Должность<input value={position} onChange={(e) => setPosition(e.target.value)} placeholder="например, Оператор" /></label>
        <label>Табельные номера (через запятую)<input value={employeeIds} onChange={(e) => setEmployeeIds(e.target.value)} placeholder="E-0001,E-0002" /></label>
      </div>

      <div className="tabs">
        <button className={tab === "shifts" ? "active" : ""} onClick={() => setTab("shifts")}>Смены</button>
        <button className={tab === "marks" ? "active" : ""} onClick={() => setTab("marks")}>Отметки</button>
        <button className={tab === "summary" ? "active" : ""} onClick={() => setTab("summary")}>Сводка</button>
      </div>

      <div className="actions">
        <button onClick={() => void load()} disabled={loading}>{loading ? "Загрузка…" : "Обновить"}</button>
        <button onClick={() => void doExport()}>Выгрузить в CSV (Excel)</button>
      </div>

      {loadError && <div className="error">{loadError}</div>}

      {tab === "shifts" && (
        <table>
          <thead>
            <tr><th>Табельный номер</th><th>ФИО</th><th>Начало</th><th>Окончание</th><th>Статус</th><th>Переработка</th><th>Отработано</th></tr>
          </thead>
          <tbody>
            {shifts.map((s, i) => (
              <tr key={i}>
                <td className="mono">{s.employee_number}</td>
                <td>{s.full_name}</td>
                <td>{fmt(s.started_at)}</td>
                <td>{fmt(s.ended_at)}</td>
                <td>{shiftStatusLabel(s.status)}</td>
                <td>{s.overtime ? "да" : "нет"}</td>
                <td>{s.worked_minutes ? money(s.worked_minutes) : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {tab === "marks" && (
        <table>
          <thead>
            <tr><th>Табельный номер</th><th>ФИО</th><th>Должность</th><th>Событие</th><th>Время (Новосибирск)</th><th>Статус по графику</th><th>Переработка</th><th>Вне офиса</th><th>Автозакрытие</th><th>Оператор</th></tr>
          </thead>
          <tbody>
            {marks.map((m, i) => (
              <tr key={i}>
                <td className="mono">{m.employee_number}</td>
                <td>{m.full_name}</td>
                <td>{m.position ?? "—"}</td>
                <td>{m.event_type === "check_in" ? "Приход" : "Уход"}</td>
                <td>{fmt(m.timestamp_server)}</td>
                <td className={m.timing_status && m.timing_status !== "пришёл вовремя" && m.timing_status !== "ушёл вовремя" ? "timing-warn" : ""}>
                  {m.timing_status || "—"}
                </td>
                <td>{m.overtime ? "да" : "нет"}</td>
                <td>{m.outside_office ? "да" : "нет"}</td>
                <td>{m.auto_closed ? "да" : "нет"}</td>
                <td>{m.operator_number ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {tab === "summary" && (
        <table>
          <thead>
            <tr><th>Табельный номер</th><th>ФИО</th><th>Должность</th><th>Смен</th><th>Закрытых</th><th>Отработано</th><th>Переработок</th><th>Отметок</th></tr>
          </thead>
          <tbody>
            {summary.map((s, i) => (
              <tr key={i}>
                <td className="mono">{s.employee_number}</td>
                <td>{s.full_name}</td>
                <td>{s.position ?? "—"}</td>
                <td>{s.shifts_total}</td>
                <td>{s.shifts_closed}</td>
                <td>{money(s.worked_minutes)}</td>
                <td>{s.overtime_shifts}</td>
                <td>{s.marks_total}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
