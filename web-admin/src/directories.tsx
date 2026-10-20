import { useState } from "react";
import { api, errorText } from "./api";
import { Field, Notice, useList } from "./sections";

type DirectoryItem = { id: string; name: string; is_active: boolean };
type ImportRow = { line: number; status: string; reason: string | null };
type ImportReport = { rows_total: number; rows_ok: number; rows_error: number; rows: ImportRow[] };

type Message = { kind: "error" | "ok"; text: string } | null;

function DirectoryEditor({
  title,
  description,
  endpoint,
  items,
  reload,
}: {
  title: string;
  description: string;
  endpoint: string;
  items: DirectoryItem[];
  reload: () => Promise<void>;
}) {
  const [name, setName] = useState("");
  const [csv, setCsv] = useState("name\n");
  const [report, setReport] = useState<ImportReport | null>(null);
  const [message, setMessage] = useState<Message>(null);
  const [busy, setBusy] = useState(false);

  async function create(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    const res = await api(endpoint, { method: "POST", body: JSON.stringify({ name }) });
    setBusy(false);
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось сохранить") });
      return;
    }
    setMessage({ kind: "ok", text: "Запись добавлена" });
    setName("");
    await reload();
  }

  async function upload(mode: "preview" | "commit") {
    const blob = new Blob([csv], { type: "text/csv" });
    const form = new FormData();
    form.append("file", blob, "directory.csv");
    const res = await api<ImportReport>(`${endpoint}/import?mode=${mode}`, { method: "POST", body: form });
    if (!res.ok || !res.body) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось загрузить CSV") });
      return;
    }
    setReport(res.body);
    setMessage({ kind: "ok", text: mode === "commit" ? "Импорт выполнен" : "Предпросмотр готов" });
    if (mode === "commit") await reload();
  }

  return (
    <div className="card">
      <h3>{title}</h3>
      <p>{description}</p>
      <form className="toolbar" onSubmit={create}>
        <input placeholder="Название" value={name} onChange={(e) => setName(e.target.value)} required />
        <button type="submit" disabled={busy}>Добавить</button>
      </form>
      <Notice message={message} />
      <table>
        <thead><tr><th>Название</th><th>Статус</th></tr></thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.id}>
              <td>{item.name}</td>
              <td>{item.is_active ? "активна" : "неактивна"}</td>
            </tr>
          ))}
          {items.length === 0 && <tr><td colSpan={2}>Список пуст</td></tr>}
        </tbody>
      </table>
      <Field label="Импорт CSV (колонка name)">
        <textarea rows={5} className="mono" value={csv} onChange={(e) => setCsv(e.target.value)} />
      </Field>
      <div className="actions">
        <button type="button" onClick={() => void upload("preview")}>Предпросмотр</button>
        <button type="button" onClick={() => void upload("commit")}>Импортировать</button>
      </div>
      {report && (
        <div>
          <p>Строк: {report.rows_total}, успешно: {report.rows_ok}, ошибок: {report.rows_error}</p>
          <table>
            <thead><tr><th>Строка</th><th>Статус</th><th>Причина</th></tr></thead>
            <tbody>
              {report.rows.filter((row) => row.status !== "ok").map((row) => (
                <tr key={row.line}><td>{row.line}</td><td>ошибка</td><td>{row.reason ?? "—"}</td></tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

export function Directories() {
  const departments = useList<DirectoryItem>("/admin/departments");
  const positions = useList<DirectoryItem>("/admin/positions");

  return (
    <section>
      <h2>Отделы и должности</h2>
      <p className="hint">Справочники используются для привязки сотрудников. Импорт CSV: колонка name, первая строка — заголовок.</p>
      <div className="columns">
        <DirectoryEditor
          title="Отделы"
          description="Подразделения организации."
          endpoint="/admin/departments"
          items={departments.items}
          reload={departments.reload}
        />
        <DirectoryEditor
          title="Должности"
          description="Справочник должностей."
          endpoint="/admin/positions"
          items={positions.items}
          reload={positions.reload}
        />
      </div>
    </section>
  );
}
