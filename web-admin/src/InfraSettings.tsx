import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { api, downloadFile, errorText } from "./api";
import { Field, Notice } from "./sections";

type Message = { kind: "error" | "ok"; text: string } | null;

type ServicePorts = {
  backend: number;
  admin: number;
  operator: number;
  reports: number;
  postgres: number;
  pgbouncer: number;
  redis: number;
  s3_api: number;
  s3_admin: number;
  https: number;
};

type PgBouncer = {
  host: string;
  port: number;
  pool_mode: string;
  max_client_conn: number;
  default_pool_size: number;
  auth_type: string;
};

type S3Settings = {
  endpoint: string;
  region: string;
  bucket: string;
  access_key_set: boolean;
  secret_key_set: boolean;
  use_ssl: boolean;
  path_style: boolean;
};

type SSLStatus = {
  cert_path: string;
  key_path: string;
  ready: boolean;
  subject: string;
  issuer: string;
  not_after: string;
  days_left: number;
  dns_names: string[] | null;
  error?: string;
  expiring_soon: boolean;
};

type DemoStatus = {
  employees: number;
  marks: number;
  qr_tokens: number;
  last_reset: string;
};

type PGSetting = {
  name: string;
  value: string;
  note: string;
};

type DBScript = {
  name: string;
  title: string;
  description: string;
  size: number;
};

type Settings = {
  database_url: string;
  database_url_set: boolean;
  database_direct_url: string;
  database_direct_url_set: boolean;
  redis_addr: string;
  settings_db_path: string;
  settings_db_size: number;
  service_ports: ServicePorts;
  pgbouncer: PgBouncer;
  s3: S3Settings;
  ssl: SSLStatus;
  demo: DemoStatus;
  postgres_recommendations: PGSetting[];
};

type S3Form = {
  endpoint: string;
  region: string;
  bucket: string;
  access_key: string;
  secret_key: string;
  use_ssl: boolean;
  path_style: boolean;
};

type MigrationResult = {
  status: string;
  switched: boolean;
  employees: number;
  marks: number;
  target_url: string;
  restart_required: boolean;
};

const emptyPorts: ServicePorts = {
  backend: 8080,
  admin: 8081,
  operator: 8082,
  reports: 8083,
  postgres: 5432,
  pgbouncer: 6432,
  redis: 6379,
  s3_api: 9000,
  s3_admin: 9001,
  https: 443,
};

const emptyPgBouncer: PgBouncer = {
  host: "pgbouncer",
  port: 6432,
  pool_mode: "transaction",
  max_client_conn: 200,
  default_pool_size: 40,
  auth_type: "scram-sha-256",
};

const emptyS3: S3Form = {
  endpoint: "",
  region: "garage",
  bucket: "tabel",
  access_key: "",
  secret_key: "",
  use_ssl: false,
  path_style: true,
};

function formatSize(value: number): string {
  if (!value) return "—";
  if (value < 1024) return `${value} Б`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} КБ`;
  return `${(value / (1024 * 1024)).toFixed(1)} МБ`;
}

function formatDate(value: string): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("ru-RU");
}

const portLabels: Array<[keyof ServicePorts, string]> = [
  ["backend", "Backend"],
  ["admin", "Админка"],
  ["operator", "Оператор"],
  ["reports", "Отчёты"],
  ["postgres", "PostgreSQL"],
  ["pgbouncer", "PgBouncer"],
  ["redis", "Redis"],
  ["s3_api", "S3 API"],
  ["s3_admin", "S3 Admin"],
  ["https", "HTTPS"],
];

export function InfraSettings() {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [scripts, setScripts] = useState<DBScript[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [testBusy, setTestBusy] = useState(false);
  const [message, setMessage] = useState<Message>(null);
  const [directURL, setDirectURL] = useState("");
  const [ports, setPorts] = useState<ServicePorts>(emptyPorts);
  const [pgBouncer, setPgBouncer] = useState<PgBouncer>(emptyPgBouncer);
  const [s3, setS3] = useState<S3Form>(emptyS3);
  const [sslCert, setSslCert] = useState("");
  const [sslKey, setSslKey] = useState("");
  const [migrationTarget, setMigrationTarget] = useState("");
  const [migrationRuntime, setMigrationRuntime] = useState("");
  const [migrationSwitch, setMigrationSwitch] = useState(false);
  const [migrationConfirm, setMigrationConfirm] = useState("");
  const [migrationBusy, setMigrationBusy] = useState(false);
  const [migrationMessage, setMigrationMessage] = useState<Message>(null);
  const [migrationResult, setMigrationResult] = useState<MigrationResult | null>(null);
  const [downloading, setDownloading] = useState("");

  function applySettings(data: Settings) {
    setSettings(data);
    setPorts({ ...emptyPorts, ...data.service_ports });
    setPgBouncer({ ...emptyPgBouncer, ...data.pgbouncer });
    setS3({
      endpoint: data.s3?.endpoint ?? "",
      region: data.s3?.region ?? "garage",
      bucket: data.s3?.bucket ?? "tabel",
      access_key: "",
      secret_key: "",
      use_ssl: data.s3?.use_ssl ?? false,
      path_style: data.s3?.path_style ?? true,
    });
    setSslCert(data.ssl?.cert_path ?? "");
    setSslKey(data.ssl?.key_path ?? "");
  }

  async function load() {
    setLoading(true);
    const [res, scriptRes] = await Promise.all([
      api<Settings>("/admin/system-settings"),
      api<DBScript[]>("/admin/system-settings/db-scripts"),
    ]);
    if (res.ok && res.body) {
      applySettings(res.body);
    } else {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось загрузить настройки инфраструктуры") });
    }
    if (scriptRes.ok && Array.isArray(scriptRes.body)) {
      setScripts(scriptRes.body);
    }
    setLoading(false);
  }

  useEffect(() => {
    void load();
  }, []);

  async function save(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setMessage(null);
    const res = await api<Settings & { restart_required: boolean }>("/admin/system-settings", {
      method: "PUT",
      body: JSON.stringify({
        database_direct_url: directURL.trim(),
        service_ports: ports,
        pgbouncer: pgBouncer,
        s3,
        ssl: { cert_path: sslCert.trim(), key_path: sslKey.trim() },
      }),
    });
    setBusy(false);
    if (!res.ok || !res.body) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось сохранить настройки инфраструктуры") });
      return;
    }
    applySettings(res.body);
    setDirectURL("");
    setS3((current) => ({ ...current, access_key: "", secret_key: "" }));
    setMessage({
      kind: "ok",
      text: res.body.restart_required
        ? "Настройки сохранены. Порты, SSL, S3 и PgBouncer применятся после перезапуска контейнеров."
        : "Настройки проверены, изменения не требовались.",
    });
  }

  async function testAll() {
    setTestBusy(true);
    setMessage(null);
    const res = await api("/admin/system-settings/test", {
      method: "POST",
      body: JSON.stringify({
        database_direct_url: directURL.trim(),
        service_ports: ports,
        pgbouncer: pgBouncer,
        s3,
        ssl: { cert_path: sslCert.trim(), key_path: sslKey.trim() },
      }),
    });
    setTestBusy(false);
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "проверка не пройдена") });
      return;
    }
    setMessage({ kind: "ok", text: "PostgreSQL, Redis" + (s3.endpoint ? ", S3" : "") + (sslCert ? " и SSL" : "") + " доступны." });
  }

  async function download(path: string, fallback: string, key: string) {
    setDownloading(key);
    setMessage(null);
    const res = await downloadFile(path, fallback);
    setDownloading("");
    if (!res.ok) {
      setMessage({ kind: "error", text: res.error ?? "не удалось скачать файл" });
      return;
    }
    setMessage({ kind: "ok", text: `Файл ${fallback} скачан.` });
  }

  async function migrate(event: FormEvent) {
    event.preventDefault();
    setMigrationBusy(true);
    setMigrationMessage(null);
    setMigrationResult(null);
    const res = await api<MigrationResult>("/admin/system-settings/migrate-postgres", {
      method: "POST",
      body: JSON.stringify({
        target_url: migrationTarget.trim(),
        target_runtime_url: migrationRuntime.trim(),
        switch_connection: migrationSwitch,
        confirm: migrationConfirm,
      }),
    });
    setMigrationBusy(false);
    if (!res.ok || !res.body) {
      setMigrationMessage({ kind: "error", text: errorText(res.body, "перенос не выполнен") });
      return;
    }
    setMigrationResult(res.body);
    setMigrationMessage({
      kind: "ok",
      text: res.body.switched
        ? "Перенос выполнен, приложение переключено на новую базу."
        : "Перенос выполнен. Новую строку подключения нужно сохранить и перезапустить backend.",
    });
    setMigrationTarget("");
    setMigrationRuntime("");
    setMigrationConfirm("");
    void load();
  }

  if (loading) return <div className="card"><h3>Инфраструктура</h3><p>Загрузка…</p></div>;

  return (
    <>
      <div className="card">
        <h3>Порты сервисов</h3>
        <Notice message={message} />
        <form onSubmit={save}>
          <div className="grid">
            {portLabels.map(([key, label]) => (
              <Field key={key} label={label}>
                <input
                  type="number"
                  min={1}
                  max={65535}
                  value={ports[key]}
                  onChange={(e) => setPorts((current) => ({ ...current, [key]: Number(e.target.value) }))}
                />
              </Field>
            ))}
          </div>
          <p className="hint">
            Порты хранятся в настройках и используются при развёртывании. Изменение портов контейнеров
            требует обновить <span className="mono">docker-compose.yaml</span> и перезапустить сервисы.
          </p>

          <h3>PgBouncer</h3>
          <div className="grid">
            <Field label="Хост">
              <input value={pgBouncer.host} onChange={(e) => setPgBouncer((c) => ({ ...c, host: e.target.value }))} />
            </Field>
            <Field label="Порт">
              <input type="number" min={1} max={65535} value={pgBouncer.port} onChange={(e) => setPgBouncer((c) => ({ ...c, port: Number(e.target.value) }))} />
            </Field>
            <Field label="Режим пула">
              <select value={pgBouncer.pool_mode} onChange={(e) => setPgBouncer((c) => ({ ...c, pool_mode: e.target.value }))}>
                <option value="transaction">transaction</option>
                <option value="session">session</option>
                <option value="statement">statement</option>
              </select>
            </Field>
            <Field label="max_client_conn">
              <input type="number" min={1} value={pgBouncer.max_client_conn} onChange={(e) => setPgBouncer((c) => ({ ...c, max_client_conn: Number(e.target.value) }))} />
            </Field>
            <Field label="default_pool_size">
              <input type="number" min={1} value={pgBouncer.default_pool_size} onChange={(e) => setPgBouncer((c) => ({ ...c, default_pool_size: Number(e.target.value) }))} />
            </Field>
            <Field label="auth_type">
              <input value={pgBouncer.auth_type} onChange={(e) => setPgBouncer((c) => ({ ...c, auth_type: e.target.value }))} />
            </Field>
          </div>

          <h3>S3-хранилище</h3>
          <div className="grid">
            <Field label="Endpoint">
              <input value={s3.endpoint} onChange={(e) => setS3((c) => ({ ...c, endpoint: e.target.value }))} placeholder="http://garage:3900" />
            </Field>
            <Field label="Регион">
              <input value={s3.region} onChange={(e) => setS3((c) => ({ ...c, region: e.target.value }))} />
            </Field>
            <Field label="Bucket">
              <input value={s3.bucket} onChange={(e) => setS3((c) => ({ ...c, bucket: e.target.value }))} />
            </Field>
            <Field label="Access key">
              <input
                value={s3.access_key}
                onChange={(e) => setS3((c) => ({ ...c, access_key: e.target.value }))}
                placeholder={settings?.s3.access_key_set ? "Задан, оставьте пустым" : "Не задан"}
                autoComplete="off"
              />
            </Field>
            <Field label="Secret key">
              <input
                type="password"
                value={s3.secret_key}
                onChange={(e) => setS3((c) => ({ ...c, secret_key: e.target.value }))}
                placeholder={settings?.s3.secret_key_set ? "Задан, оставьте пустым" : "Не задан"}
                autoComplete="new-password"
              />
            </Field>
          </div>
          <div className="grid">
            <label className="checkbox">
              <input type="checkbox" checked={s3.use_ssl} onChange={(e) => setS3((c) => ({ ...c, use_ssl: e.target.checked }))} />
              Использовать TLS для S3
            </label>
            <label className="checkbox">
              <input type="checkbox" checked={s3.path_style} onChange={(e) => setS3((c) => ({ ...c, path_style: e.target.checked }))} />
              Path-style адресация
            </label>
          </div>

          <h3>SSL-сертификат</h3>
          <div className="grid">
            <Field label="Путь к fullchain.pem">
              <input value={sslCert} onChange={(e) => setSslCert(e.target.value)} placeholder="/ssl/staff-rf/fullchain.pem" />
            </Field>
            <Field label="Путь к privkey.pem">
              <input value={sslKey} onChange={(e) => setSslKey(e.target.value)} placeholder="/ssl/staff-rf/privkey.pem" />
            </Field>
          </div>
          {settings?.ssl && (
            <p className={settings.ssl.ready ? "hint" : "error"}>
              {settings.ssl.ready
                ? `Сертификат действителен до ${formatDate(settings.ssl.not_after)} (${settings.ssl.days_left} дн.). DNS: ${(settings.ssl.dns_names ?? []).join(", ") || "—"}`
                : `SSL не готов: ${settings.ssl.error ?? "проверьте пути"}`}
            </p>
          )}

          <h3>Прямое подключение к PostgreSQL</h3>
          <Field label="Новая строка прямого подключения (для миграций и дампа)">
            <input
              type="password"
              value={directURL}
              onChange={(e) => setDirectURL(e.target.value)}
              placeholder="Оставьте пустым, чтобы не менять"
              autoComplete="new-password"
            />
          </Field>
          <p className="hint mono">
            Текущее прямое подключение: {settings?.database_direct_url_set ? settings.database_direct_url : "не задано"}
          </p>

          <div className="actions">
            <button type="button" onClick={() => void testAll()} disabled={busy || testBusy}>
              {testBusy ? "Проверка…" : "Проверить подключения"}
            </button>
            <button type="submit" disabled={busy || testBusy}>
              {busy ? "Сохранение…" : "Сохранить инфраструктуру"}
            </button>
          </div>
        </form>
      </div>

      <div className="card">
        <h3>Скрипты установки PostgreSQL</h3>
        <p className="hint">Скрипты создаются из миграций приложения и всегда соответствуют текущей версии схемы.</p>
        <table>
          <thead><tr><th>Файл</th><th>Назначение</th><th>Размер</th><th></th></tr></thead>
          <tbody>
            {scripts.map((script) => (
              <tr key={script.name}>
                <td className="mono">{script.name}</td>
                <td>{script.title}<br /><span className="hint">{script.description}</span></td>
                <td>{formatSize(script.size)}</td>
                <td>
                  <button type="button" className="secondary" disabled={downloading === script.name}
                    onClick={() => void download(`/admin/system-settings/db-scripts/${encodeURIComponent(script.name)}`, script.name, script.name)}>
                    {downloading === script.name ? "…" : "Скачать"}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <div className="actions">
          <button type="button" onClick={() => void download("/admin/system-settings/db-scripts.zip", "tabel-postgres-scripts.zip", "zip")} disabled={downloading === "zip"}>
            {downloading === "zip" ? "Подготовка…" : "Скачать всё архивом"}
          </button>
          <button type="button" className="secondary" onClick={() => void download("/admin/system-settings/settings-db", "settings.db", "db")} disabled={downloading === "db"}>
            {downloading === "db" ? "Подготовка…" : "Скачать settings.db"}
          </button>
        </div>
        <p className="hint mono">База настроек: {settings?.settings_db_path || "—"} ({formatSize(settings?.settings_db_size ?? 0)})</p>
      </div>

      <div className="card">
        <h3>Перенос PostgreSQL</h3>
        <Notice message={migrationMessage} />
        <p className="hint">
          Используется прямое подключение текущей базы и строка подключения целевого сервера.
          Сначала создаётся дамп, затем он восстанавливается и проверяется. Для подтверждения введите MIGRATE.
        </p>
        <form onSubmit={migrate}>
          <Field label="Строка подключения целевого PostgreSQL">
            <input type="password" value={migrationTarget} onChange={(e) => setMigrationTarget(e.target.value)} placeholder="postgres://user:pass@new-pg:5432/tabel?sslmode=disable" autoComplete="new-password" />
          </Field>
          <Field label="Рабочая строка после переключения (необязательно)">
            <input type="password" value={migrationRuntime} onChange={(e) => setMigrationRuntime(e.target.value)} placeholder="Оставьте пустым, чтобы использовать прямую строку" autoComplete="new-password" />
          </Field>
          <label className="checkbox">
            <input type="checkbox" checked={migrationSwitch} onChange={(e) => setMigrationSwitch(e.target.checked)} />
            Переключить приложение на новую базу сразу после успешного переноса
          </label>
          <Field label="Подтверждение">
            <input value={migrationConfirm} onChange={(e) => setMigrationConfirm(e.target.value)} placeholder="MIGRATE" />
          </Field>
          <div className="actions">
            <button type="submit" disabled={migrationBusy || migrationConfirm.trim().toUpperCase() !== "MIGRATE"}>
              {migrationBusy ? "Перенос…" : "Перенести базу"}
            </button>
          </div>
        </form>
        {migrationResult && (
          <p className="hint">
            Проверено: сотрудников {migrationResult.employees}, отметок {migrationResult.marks}. Цель: <span className="mono">{migrationResult.target_url}</span>.
            {migrationResult.restart_required ? " Требуется перезапуск backend." : ""}
          </p>
        )}
      </div>

      <div className="card">
        <h3>Демо-база</h3>
        <div className="grid">
          <p>Сотрудников: <strong>{settings?.demo.employees ?? 0}</strong></p>
          <p>Отметок: <strong>{settings?.demo.marks ?? 0}</strong></p>
          <p>QR-токенов: <strong>{settings?.demo.qr_tokens ?? 0}</strong></p>
          <p>Последний сброс: <strong>{settings?.demo.last_reset || "—"}</strong></p>
        </div>
      </div>

      <div className="card">
        <h3>Рекомендуемые параметры PostgreSQL</h3>
        <p className="hint">Ориентир для PostgreSQL 16 на сервере 8–10 ГБ RAM / 8 vCPU / SSD. Проверяйте под фактическую нагрузку.</p>
        <table>
          <thead><tr><th>Параметр</th><th>Значение</th><th>Комментарий</th></tr></thead>
          <tbody>
            {(settings?.postgres_recommendations ?? []).map((item) => (
              <tr key={item.name}>
                <td className="mono">{item.name}</td>
                <td className="mono">{item.value}</td>
                <td>{item.note}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <div className="actions">
          <button type="button" className="secondary" onClick={() => void download("/admin/system-settings/db-scripts/04_recommended_postgresql.conf", "04_recommended_postgresql.conf", "conf")} disabled={downloading === "conf"}>
            {downloading === "conf" ? "Подготовка…" : "Скачать postgresql.conf"}
          </button>
        </div>
      </div>
    </>
  );
}
