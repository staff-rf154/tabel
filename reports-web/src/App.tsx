import { Fragment, useCallback, useEffect, useMemo, useState } from "react";
import type { FormEvent } from "react";
import { api, clearTokens, downloadFile, errorText, getToken, login } from "./api";

type Mark = {
  employee_number: string;
  full_name: string;
  position?: string | null;
  department?: string | null;
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
  position?: string | null;
  department?: string | null;
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
  department?: string | null;
  shifts_total: number;
  shifts_closed: number;
  worked_minutes: number;
  overtime_shifts: number;
  marks_total: number;
};

type WorkedTime = {
  employee_id: string;
  employee_number: string;
  full_name: string;
  position?: string | null;
  department?: string | null;
  days_worked: number;
  shifts_total: number;
  worked_minutes: number;
  average_minutes: number;
  overtime_shifts: number;
};

type FirstLast = {
  employee_id: string;
  employee_number: string;
  full_name: string;
  position?: string | null;
  department?: string | null;
  work_date: string;
  first_in: string;
  last_out?: string | null;
  worked_minutes: number;
};

type EntryExit = {
  employee_id: string;
  employee_number: string;
  full_name: string;
  position?: string | null;
  department?: string | null;
  work_date: string;
  check_in: string;
  check_out?: string | null;
  minutes?: number | null;
  status: string;
};

type Daily = {
  employee_id: string;
  employee_number: string;
  full_name: string;
  position?: string | null;
  department?: string | null;
  work_date: string;
  check_in: string;
  check_out?: string | null;
  presence_minutes: number;
  worked_minutes: number;
  shifts_total: number;
  overtime_shifts: number;
};

type TimesheetDay = { date: string; worked_minutes: number; code: string };

type Timesheet = {
  employee_id: string;
  employee_number: string;
  full_name: string;
  position?: string | null;
  department?: string | null;
  days: TimesheetDay[];
  days_worked: number;
  worked_minutes: number;
  overtime_shifts: number;
};

type Tab = "worked" | "first-last" | "entries-exits" | "daily" | "timesheet" | "shifts" | "marks" | "summary";

const tabs: Array<[Tab, string]> = [
  ["worked", "Сводный график"],
  ["first-last", "Первый приход / последний выход"],
  ["entries-exits", "Входы-выходы"],
  ["daily", "Приход-уход по дням"],
  ["timesheet", "Табель Т-13"],
  ["shifts", "Смены"],
  ["marks", "Отметки"],
  ["summary", "Сводка"],
];

const reportTitles: Record<Tab, string> = {
  worked: "Сводный график отработанного времени за период",
  "first-last": "Первый приход и последний выход по сотруднику за каждый день",
  "entries-exits": "Входы и выходы с группировкой по сотруднику",
  daily: "Приход, уход и время присутствия с разбивкой по дням",
  timesheet: "Табель учёта рабочего времени (Т-13)",
  shifts: "Смены",
  marks: "Отметки",
  summary: "Сводка по сотрудникам",
};

const reportMeta: Record<Tab, { path: string; filename: string }> = {
  worked: { path: "/reports/worked-time", filename: "worked-time-report.csv" },
  "first-last": { path: "/reports/first-last", filename: "first-last-report.csv" },
  "entries-exits": { path: "/reports/entries-exits", filename: "entries-exits-report.csv" },
  daily: { path: "/reports/daily", filename: "daily-report.csv" },
  timesheet: { path: "/reports/timesheet", filename: "timesheet-t13.csv" },
  shifts: { path: "/reports/shifts", filename: "shifts-report.csv" },
  marks: { path: "/reports/marks", filename: "marks-report.csv" },
  summary: { path: "/reports/summary", filename: "summary-report.csv" },
};

function minutesText(minutes: number): string {
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  return `${hours} ч ${rest} мин`;
}

function hoursText(minutes: number): string {
  if (!minutes) return "";
  const value = minutes / 60;
  return Number.isInteger(value) ? String(value) : value.toFixed(2).replace(".", ",");
}

function fmt(value: string | null | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  return date.toLocaleString("ru-RU", { timeZone: "Asia/Novosibirsk" });
}

function fmtTime(value: string | null | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  return date.toLocaleTimeString("ru-RU", {
    timeZone: "Asia/Novosibirsk",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function fmtDate(value: string | null | undefined): string {
  if (!value) return "—";
  const date = new Date(`${value}T00:00:00Z`);
  return date.toLocaleDateString("ru-RU", { timeZone: "UTC" });
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

function dateRange(from: string, to: string): string[] {
  if (!from || !to) return [];
  const start = new Date(`${from}T00:00:00Z`);
  const end = new Date(`${to}T00:00:00Z`);
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) return [];
  const result: string[] = [];
  for (let time = start.getTime(); time <= end.getTime() && result.length < 366; time += 24 * 60 * 60 * 1000) {
    const date = new Date(time);
    result.push(`${date.getUTCFullYear()}-${String(date.getUTCMonth() + 1).padStart(2, "0")}-${String(date.getUTCDate()).padStart(2, "0")}`);
  }
  return result;
}

type EntryGroup = {
  employee_id: string;
  employee_number: string;
  full_name: string;
  position?: string | null;
  department?: string | null;
  rows: EntryExit[];
};

function groupEntries(items: EntryExit[]): EntryGroup[] {
  const map = new Map<string, EntryGroup>();
  const result: EntryGroup[] = [];
  for (const item of items) {
    let group = map.get(item.employee_id);
    if (!group) {
      group = {
        employee_id: item.employee_id,
        employee_number: item.employee_number,
        full_name: item.full_name,
        position: item.position,
        department: item.department,
        rows: [],
      };
      map.set(item.employee_id, group);
      result.push(group);
    }
    group.rows.push(item);
  }
  return result;
}

function EmptyRow({ colSpan }: { colSpan: number }) {
  return <tr><td colSpan={colSpan} className="empty">Нет данных за выбранный период</td></tr>;
}

export default function App() {
  const [loggedIn, setLoggedIn] = useState<boolean>(() => getToken() !== "");
  const [loginValue, setLoginValue] = useState("");
  const [password, setPassword] = useState("");
  const [loginError, setLoginError] = useState("");
  const [busy, setBusy] = useState(false);

  const [tab, setTab] = useState<Tab>("worked");
  const [from, setFrom] = useState(() => {
    const now = new Date();
    return localInput(new Date(now.getFullYear(), now.getMonth(), 1));
  });
  const [to, setTo] = useState(() => localInput(new Date()));
  const [position, setPosition] = useState("");
  const [employees, setEmployees] = useState("");

  const [marks, setMarks] = useState<Mark[]>([]);
  const [shifts, setShifts] = useState<Shift[]>([]);
  const [summary, setSummary] = useState<Summary[]>([]);
  const [worked, setWorked] = useState<WorkedTime[]>([]);
  const [firstLast, setFirstLast] = useState<FirstLast[]>([]);
  const [entries, setEntries] = useState<EntryExit[]>([]);
  const [daily, setDaily] = useState<Daily[]>([]);
  const [timesheet, setTimesheet] = useState<Timesheet[]>([]);
  const [loadError, setLoadError] = useState("");
  const [loading, setLoading] = useState(false);

  const query = useCallback(() => {
    const params = new URLSearchParams();
    params.set("from", new Date(`${from}T00:00:00+07:00`).toISOString());
    params.set("to", new Date(`${to}T23:59:59+07:00`).toISOString());
    if (position.trim()) params.set("position", position.trim());
    if (employees.trim()) params.set("employees", employees.trim());
    return params.toString();
  }, [from, to, position, employees]);

  const load = useCallback(async () => {
    if (!from || !to) {
      setLoadError("Укажите начало и окончание периода");
      return;
    }
    setLoading(true);
    setLoadError("");
    const res = await api<unknown>(`${reportMeta[tab].path}?${query()}`);
    setLoading(false);
    if (!res.ok) {
      setLoadError(errorText(res.body, "не удалось загрузить отчёт"));
      return;
    }
    switch (tab) {
      case "worked": setWorked((res.body as WorkedTime[]) ?? []); break;
      case "first-last": setFirstLast((res.body as FirstLast[]) ?? []); break;
      case "entries-exits": setEntries((res.body as EntryExit[]) ?? []); break;
      case "daily": setDaily((res.body as Daily[]) ?? []); break;
      case "timesheet": setTimesheet((res.body as Timesheet[]) ?? []); break;
      case "shifts": setShifts((res.body as Shift[]) ?? []); break;
      case "marks": setMarks((res.body as Mark[]) ?? []); break;
      case "summary": setSummary((res.body as Summary[]) ?? []); break;
    }
  }, [tab, query, from, to]);

  useEffect(() => {
    if (loggedIn && tab) void load();
  }, [loggedIn, tab, load]);

  const timesheetDates = useMemo(() => dateRange(from, to), [from, to]);
  const entryGroups = useMemo(() => groupEntries(entries), [entries]);
  const maxWorked = useMemo(() => Math.max(1, ...worked.map((item) => item.worked_minutes)), [worked]);

  async function doLogin(event: FormEvent) {
    event.preventDefault();
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
    if (!from || !to) {
      setLoadError("Укажите начало и окончание периода");
      return;
    }
    const meta = reportMeta[tab];
    const err = await downloadFile(`${meta.path}?${query()}&format=csv`, meta.filename);
    if (err) setLoadError(err);
  }

  function logout() {
    clearTokens();
    setLoggedIn(false);
    setMarks([]);
    setShifts([]);
    setSummary([]);
    setWorked([]);
    setFirstLast([]);
    setEntries([]);
    setDaily([]);
    setTimesheet([]);
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
        <label>ФИО или табельный номер (через запятую)<input value={employees} onChange={(e) => setEmployees(e.target.value)} placeholder="Иванов Иван Иванович, н-00000027" /></label>
      </div>

      <div className="tabs">
        {tabs.map(([value, label]) => (
          <button key={value} className={tab === value ? "active" : ""} onClick={() => setTab(value)}>{label}</button>
        ))}
      </div>

      <div className="actions">
        <button onClick={() => void load()} disabled={loading}>{loading ? "Загрузка…" : "Обновить"}</button>
        <button onClick={() => void doExport()}>Выгрузить в CSV (Excel)</button>
      </div>

      {loadError && <div className="error">{loadError}</div>}
      <h2>{reportTitles[tab]}</h2>
      {tab === "timesheet" && <p className="note">Для месячной формы Т-13 выберите первое и последнее число месяца.</p>}

      {tab === "worked" && (
        <div className="table-scroll">
          <table>
            <thead>
              <tr><th>№</th><th>ФИО</th><th>Должность</th><th>Отдел</th><th>Дней</th><th>Смен</th><th>Отработано</th><th>Среднее в день</th><th>Переработок</th></tr>
            </thead>
            <tbody>
              {worked.map((item, index) => (
                <tr key={item.employee_id}>
                  <td>{index + 1}</td>
                  <td>{item.full_name}</td>
                  <td>{item.position ?? "—"}</td>
                  <td>{item.department ?? "—"}</td>
                  <td>{item.days_worked}</td>
                  <td>{item.shifts_total}</td>
                  <td>
                    <span className="work-value">{minutesText(item.worked_minutes)}</span>
                    <span className="bar"><span style={{ width: `${Math.max(2, Math.round(item.worked_minutes * 100 / maxWorked))}%` }} /></span>
                  </td>
                  <td>{minutesText(item.average_minutes)}</td>
                  <td>{item.overtime_shifts}</td>
                </tr>
              ))}
              {worked.length === 0 && <EmptyRow colSpan={9} />}
            </tbody>
          </table>
        </div>
      )}

      {tab === "first-last" && (
        <div className="table-scroll">
          <table>
            <thead>
              <tr><th>№</th><th>ФИО</th><th>Должность</th><th>Отдел</th><th>Дата</th><th>Первый приход</th><th>Последний выход</th><th>Отработано</th></tr>
            </thead>
            <tbody>
              {firstLast.map((item, index) => (
                <tr key={`${item.employee_id}-${item.work_date}`} className={index === 0 || firstLast[index - 1].employee_id !== item.employee_id ? "group-start" : ""}>
                  <td>{index + 1}</td>
                  <td>{item.full_name}</td>
                  <td>{item.position ?? "—"}</td>
                  <td>{item.department ?? "—"}</td>
                  <td>{fmtDate(item.work_date)}</td>
                  <td>{fmtTime(item.first_in)}</td>
                  <td>{fmtTime(item.last_out)}</td>
                  <td>{minutesText(item.worked_minutes)}</td>
                </tr>
              ))}
              {firstLast.length === 0 && <EmptyRow colSpan={8} />}
            </tbody>
          </table>
        </div>
      )}

      {tab === "entries-exits" && (
        <div className="table-scroll">
          <table>
            <thead>
              <tr><th>ФИО</th><th>Должность</th><th>Отдел</th><th>Дата</th><th>Вход</th><th>Выход</th><th>Длительность</th><th>Статус</th></tr>
            </thead>
            <tbody>
              {entryGroups.map((group) => group.rows.map((item, index) => (
                <tr key={`${group.employee_id}-${item.check_in}`} className={index === 0 ? "group-start" : ""}>
                  {index === 0 && (
                    <>
                      <td rowSpan={group.rows.length}>{group.full_name}</td>
                      <td rowSpan={group.rows.length}>{group.position ?? "—"}</td>
                      <td rowSpan={group.rows.length}>{group.department ?? "—"}</td>
                    </>
                  )}
                  <td>{fmtDate(item.work_date)}</td>
                  <td>{fmtTime(item.check_in)}</td>
                  <td>{fmtTime(item.check_out)}</td>
                  <td>{item.minutes ? minutesText(item.minutes) : "—"}</td>
                  <td>{shiftStatusLabel(item.status)}</td>
                </tr>
              )))}
              {entries.length === 0 && <EmptyRow colSpan={8} />}
            </tbody>
          </table>
        </div>
      )}

      {tab === "daily" && (
        <div className="table-scroll">
          <table>
            <thead>
              <tr><th>ФИО</th><th>Должность</th><th>Отдел</th><th>Дата</th><th>Приход</th><th>Уход</th><th>Присутствие</th><th>Отработано</th><th>Смен</th></tr>
            </thead>
            <tbody>
              {daily.map((item, index) => (
                <tr key={`${item.employee_id}-${item.work_date}`} className={index === 0 || daily[index - 1].employee_id !== item.employee_id ? "group-start" : ""}>
                  <td>{item.full_name}</td>
                  <td>{item.position ?? "—"}</td>
                  <td>{item.department ?? "—"}</td>
                  <td>{fmtDate(item.work_date)}</td>
                  <td>{fmtTime(item.check_in)}</td>
                  <td>{fmtTime(item.check_out)}</td>
                  <td>{minutesText(item.presence_minutes)}</td>
                  <td>{minutesText(item.worked_minutes)}</td>
                  <td>{item.shifts_total}</td>
                </tr>
              ))}
              {daily.length === 0 && <EmptyRow colSpan={9} />}
            </tbody>
          </table>
        </div>
      )}

      {tab === "timesheet" && (
        <div className="table-scroll">
          <table className="timesheet">
            <thead>
              <tr>
                <th>№</th><th>ФИО</th><th>Должность</th><th>Отдел</th>
                {timesheetDates.map((date) => <th key={date} title={date}>{date.slice(8)}</th>)}
                <th>Дней</th><th>Часов</th>
              </tr>
            </thead>
            <tbody>
              {timesheet.map((item, index) => {
                const byDate = new Map(item.days.map((day) => [day.date, day.worked_minutes]));
                return (
                  <Fragment key={item.employee_id}>
                    <tr className="timesheet-main">
                      <td rowSpan={2}>{index + 1}</td>
                      <td rowSpan={2} className="ts-name">{item.full_name}</td>
                      <td rowSpan={2}>{item.position ?? "—"}</td>
                      <td rowSpan={2}>{item.department ?? "—"}</td>
                      {timesheetDates.map((date) => <td key={date} className="ts-code">{byDate.get(date) ? "Я" : ""}</td>)}
                      <td rowSpan={2}>{item.days_worked}</td>
                      <td rowSpan={2}>{hoursText(item.worked_minutes)}</td>
                    </tr>
                    <tr>
                      {timesheetDates.map((date) => <td key={date} className="ts-hours">{hoursText(byDate.get(date) ?? 0)}</td>)}
                    </tr>
                  </Fragment>
                );
              })}
              {timesheet.length === 0 && <EmptyRow colSpan={6 + timesheetDates.length} />}
            </tbody>
          </table>
        </div>
      )}

      {tab === "shifts" && (
        <div className="table-scroll">
          <table>
            <thead>
              <tr><th>Табельный номер</th><th>ФИО</th><th>Должность</th><th>Отдел</th><th>Начало</th><th>Окончание</th><th>Статус</th><th>Переработка</th><th>Отработано</th></tr>
            </thead>
            <tbody>
              {shifts.map((item, index) => (
                <tr key={index}>
                  <td className="mono">{item.employee_number}</td>
                  <td>{item.full_name}</td>
                  <td>{item.position ?? "—"}</td>
                  <td>{item.department ?? "—"}</td>
                  <td>{fmt(item.started_at)}</td>
                  <td>{fmt(item.ended_at)}</td>
                  <td>{shiftStatusLabel(item.status)}</td>
                  <td>{item.overtime ? "да" : "нет"}</td>
                  <td>{item.worked_minutes ? minutesText(item.worked_minutes) : "—"}</td>
                </tr>
              ))}
              {shifts.length === 0 && <EmptyRow colSpan={9} />}
            </tbody>
          </table>
        </div>
      )}

      {tab === "marks" && (
        <div className="table-scroll">
          <table>
            <thead>
              <tr><th>Табельный номер</th><th>ФИО</th><th>Должность</th><th>Отдел</th><th>Событие</th><th>Время (Новосибирск)</th><th>Статус по графику</th><th>Переработка</th><th>Вне офиса</th><th>Автозакрытие</th><th>Оператор</th></tr>
            </thead>
            <tbody>
              {marks.map((item, index) => (
                <tr key={index}>
                  <td className="mono">{item.employee_number}</td>
                  <td>{item.full_name}</td>
                  <td>{item.position ?? "—"}</td>
                  <td>{item.department ?? "—"}</td>
                  <td>{item.event_type === "check_in" ? "Приход" : "Уход"}</td>
                  <td>{fmt(item.timestamp_server)}</td>
                  <td className={item.timing_status && item.timing_status !== "пришёл вовремя" && item.timing_status !== "ушёл вовремя" ? "timing-warn" : ""}>
                    {item.timing_status || "—"}
                  </td>
                  <td>{item.overtime ? "да" : "нет"}</td>
                  <td>{item.outside_office ? "да" : "нет"}</td>
                  <td>{item.auto_closed ? "да" : "нет"}</td>
                  <td>{item.operator_number ?? "—"}</td>
                </tr>
              ))}
              {marks.length === 0 && <EmptyRow colSpan={11} />}
            </tbody>
          </table>
        </div>
      )}

      {tab === "summary" && (
        <div className="table-scroll">
          <table>
            <thead>
              <tr><th>Табельный номер</th><th>ФИО</th><th>Должность</th><th>Отдел</th><th>Смен</th><th>Закрытых</th><th>Отработано</th><th>Переработок</th><th>Отметок</th></tr>
            </thead>
            <tbody>
              {summary.map((item, index) => (
                <tr key={index}>
                  <td className="mono">{item.employee_number}</td>
                  <td>{item.full_name}</td>
                  <td>{item.position ?? "—"}</td>
                  <td>{item.department ?? "—"}</td>
                  <td>{item.shifts_total}</td>
                  <td>{item.shifts_closed}</td>
                  <td>{minutesText(item.worked_minutes)}</td>
                  <td>{item.overtime_shifts}</td>
                  <td>{item.marks_total}</td>
                </tr>
              ))}
              {summary.length === 0 && <EmptyRow colSpan={9} />}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
