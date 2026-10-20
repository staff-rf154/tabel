import { useCallback, useEffect, useState } from "react";
import type { FormEvent } from "react";
import { api, clearTokens, errorText, getToken, login } from "./api";
import { Employees, Offices, Schedules, Users } from "./sections";
import { Audit, Delegations, Marks, Notifications, OAuthPanel, QRPanel } from "./sections2";
import { Directories } from "./directories";
import { DemoPanel, Imports, MobileBuilds } from "./sections3";
import { roleLabel } from "./labels";

type SetupStatus = {
  installed: boolean;
  db_configured: boolean;
  db_reachable: boolean;
  redis_configured: boolean;
};

type Profile = { id: string; role: string; login: string; full_name: string | null };

const sections = [
  ["employees", "Сотрудники"],
  ["directories", "Отделы и должности"],
  ["imports", "Импорт данных"],
  ["users", "Пользователи"],
  ["schedules", "Графики"],
  ["offices", "Офисы"],
  ["qr", "QR-токены"],
  ["marks", "Отметки"],
  ["delegations", "Делегирования"],
  ["notifications", "Уведомления"],
  ["oauth", "OAuth"],
  ["mobile", "Мобильное приложение"],
  ["demo", "Демо-данные"],
  ["audit", "Аудит"],
] as const;

type SectionKey = (typeof sections)[number][0];

export default function App() {
  const [status, setStatus] = useState<SetupStatus | null>(null);
  const [profile, setProfile] = useState<Profile | null>(null);
  const [current, setCurrent] = useState<SectionKey>("employees");
  const [error, setError] = useState("");

  const loadStatus = useCallback(async () => {
    const res = await api<SetupStatus>("/setup/status");
    if (res.ok && res.body) {
      setStatus(res.body);
      return res.body;
    }
    setError(errorText(res.body, "не удалось получить статус установки"));
    return null;
  }, []);

  const loadProfile = useCallback(async () => {
    if (!getToken()) {
      setProfile(null);
      return;
    }
    const res = await api<Profile>("/me");
    if (res.ok && res.body) {
      setProfile(res.body);
    } else {
      clearTokens();
      setProfile(null);
    }
  }, []);

  useEffect(() => {
    void (async () => {
      const value = await loadStatus();
      if (value?.installed) {
        await loadProfile();
      }
    })();
  }, [loadStatus, loadProfile]);

  function logout() {
    void api("/auth/logout", { method: "POST" });
    clearTokens();
    setProfile(null);
  }

  if (!status) {
    return (
      <main className="app">
        <h1>Табель — панель администратора</h1>
        <p>{error || "Загрузка состояния системы…"}</p>
      </main>
    );
  }

  if (!status.installed) {
    return <Setup status={status} onDone={async () => { await loadStatus(); await loadProfile(); }} />;
  }

  if (!profile) {
    return <Login onLogin={async () => { await loadProfile(); }} />;
  }

  return (
    <div className="layout">
      <aside className="sidebar">
        <h1>Табель</h1>
        <p className="role">{roleLabel(profile.role)}{profile.full_name ? ` · ${profile.full_name}` : ""}</p>
        <nav>
          {sections.map(([key, label]) => (
            <button
              key={key}
              type="button"
              className={key === current ? "active" : ""}
              onClick={() => setCurrent(key)}
            >
              {label}
            </button>
          ))}
        </nav>
        <button type="button" className="logout" onClick={logout}>Выйти</button>
      </aside>
      <main className="content">
        {current === "employees" && <Employees />}
      {current === "directories" && <Directories />}
          {current === "imports" && <Imports />}
        {current === "users" && <Users />}
          {current === "schedules" && <Schedules />}
        {current === "offices" && <Offices />}
        {current === "qr" && <QRPanel />}
        {current === "marks" && <Marks />}
        {current === "delegations" && <Delegations />}
        {current === "notifications" && <Notifications />}
        {current === "oauth" && <OAuthPanel />}
        {current === "mobile" && <MobileBuilds />}
        {current === "demo" && <DemoPanel />}
        {current === "audit" && <Audit />}
      </main>
    </div>
  );
}

function Login({ onLogin }: { onLogin: () => Promise<void> }) {
  const [loginValue, setLoginValue] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    const res = await login(loginValue, password);
    if (!res.ok) {
      setError(errorText(res.body, "не удалось войти"));
      setBusy(false);
      return;
    }
    await onLogin();
    setBusy(false);
  }

  return (
    <main className="app narrow">
      <h1>Табель — вход</h1>
      <form onSubmit={submit}>
        <label>Логин<input value={loginValue} onChange={(e) => setLoginValue(e.target.value)} autoComplete="username" required /></label>
        <label>Пароль<input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" required /></label>
        {error && <p className="error">{error}</p>}
        <button type="submit" disabled={busy}>{busy ? "Вход…" : "Войти"}</button>
      </form>
    </main>
  );
}

function Setup({ status, onDone }: { status: SetupStatus; onDone: () => Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [databaseUrl, setDatabaseUrl] = useState(
    "postgres://tabel:CHANGE_ME_postgres@pgbouncer:5432/tabel?sslmode=disable",
  );
  const [redisAddr, setRedisAddr] = useState("redis:6379");
  const [redisPassword, setRedisPassword] = useState("");
  const [adminEmail, setAdminEmail] = useState("");
  const [supportPhone, setSupportPhone] = useState("");
  const [adminLogin, setAdminLogin] = useState("");
  const [adminPassword, setAdminPassword] = useState("");

  async function run(action: () => Promise<{ ok: boolean; status: number; body: unknown }>, successMessage: string) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const res = await action();
      if (res.ok) {
        setNotice(successMessage);
      } else {
        setError(errorText(res.body, `ошибка (HTTP ${res.status})`));
      }
    } catch {
      setError("сетевая ошибка при обращении к серверу");
    } finally {
      setBusy(false);
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    await run(
      () =>
        api("/setup/save", {
          method: "POST",
          body: JSON.stringify({
            database_url: databaseUrl,
            redis_addr: redisAddr,
            redis_password: redisPassword,
            main_admin_email: adminEmail,
            support_phone: supportPhone,
          }),
        }),
      "Параметры сохранены, миграции применены",
    );
  }

  async function complete() {
    await run(
      () =>
        api("/setup/complete", {
          method: "POST",
          body: JSON.stringify({
            main_admin_email: adminEmail,
            support_phone: supportPhone,
            admin_login: adminLogin,
            admin_password: adminPassword,
          }),
        }),
      "Установка завершена",
    );
    await onDone();
  }

  return (
    <main className="app narrow">
      <h1>Первичная установка</h1>
      <p>Параметры подключения сохраняются на сервере приложений в зашифрованном виде.</p>
      <dl className="status">
        <dt>PostgreSQL</dt>
        <dd>{status.db_configured ? (status.db_reachable ? "настроено, доступно" : "настроено, недоступно") : "не настроено"}</dd>
        <dt>Redis</dt>
        <dd>{status.redis_configured ? "настроено" : "не настроено"}</dd>
      </dl>
      {notice && <p className="ok">{notice}</p>}
      {error && <p className="error">{error}</p>}
      <form onSubmit={submit}>
        <label>Строка подключения PostgreSQL<input value={databaseUrl} onChange={(e) => setDatabaseUrl(e.target.value)} autoComplete="off" /></label>
        <label>Адрес Redis<input value={redisAddr} onChange={(e) => setRedisAddr(e.target.value)} autoComplete="off" /></label>
        <label>Пароль Redis<input type="password" value={redisPassword} onChange={(e) => setRedisPassword(e.target.value)} autoComplete="new-password" /></label>
        <label>Email главного администратора<input type="email" value={adminEmail} onChange={(e) => setAdminEmail(e.target.value)} autoComplete="off" /></label>
        <label>Телефон поддержки<input value={supportPhone} onChange={(e) => setSupportPhone(e.target.value)} autoComplete="off" /></label>
        <label>Логин главного администратора<input value={adminLogin} onChange={(e) => setAdminLogin(e.target.value)} autoComplete="off" /></label>
        <label>Пароль главного администратора<input type="password" value={adminPassword} onChange={(e) => setAdminPassword(e.target.value)} autoComplete="new-password" /></label>
        <div className="actions">
          <button type="button" onClick={() => void run(() => api("/setup/test-db", { method: "POST", body: JSON.stringify({ database_url: databaseUrl }) }), "Подключение к PostgreSQL успешно")} disabled={busy}>
            Проверить БД
          </button>
          <button type="submit" disabled={busy}>Сохранить и применить</button>
          <button type="button" onClick={() => void complete()} disabled={busy || !status.db_reachable}>Завершить установку</button>
        </div>
      </form>
    </main>
  );
}
