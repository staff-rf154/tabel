import { Fragment, useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import maplibregl from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { api, errorText, getToken } from "./api";
import {
  OFFLINE_MAP_STYLE,
  officePointCollection,
  officeZoneCollection,
  type OfficeMapItem,
} from "./mapStyle";
import { Field, Notice, fmt, useList } from "./sections";
import { InfraSettings } from "./InfraSettings";
import {
  auditActionLabel,
  auditActionOptions,
  auditEntityLabel,
  auditEntityOptions,
  auditFieldLabel,
  auditValueLabel,
  clientTypeLabel,
  delegationStatusLabel,
  eventLabel,
  notificationTypeLabel,
  qrSourceLabel,
} from "./labels";

type Message = { kind: "error" | "ok"; text: string } | null;
type ImportRow = { line: number; status: string; reason: string | null };
type ImportReport = { rows_total: number; rows_ok: number; rows_error: number; rows: ImportRow[] };

/* ---------------- QR-токены ---------------- */

type Employee = { id: string; full_name: string; employee_number: string };
type BulkQRRow = {
  employee_id: string;
  employee_number?: string;
  full_name?: string;
  status: "created" | "existing" | "error";
  qr_id?: string;
  reason?: string;
};
type BulkQRReport = { total: number; created: number; existing: number; errors: number; rows: BulkQRRow[] };

export function QRPanel() {
  const employees = useList<Employee>("/admin/employees?limit=500");
  const [employeeID, setEmployeeID] = useState("");
  const [message, setMessage] = useState<Message>(null);
  const [bulk, setBulk] = useState<ImportReport | null>(null);
  const [qrImage, setQrImage] = useState("");
  const [qrBusy, setQrBusy] = useState(false);
  const [employeeSearch, setEmployeeSearch] = useState("");
  const [selectedEmployees, setSelectedEmployees] = useState<string[]>([]);
  const [bulkEmployees, setBulkEmployees] = useState<BulkQRReport | null>(null);
  const [bulkQrBusy, setBulkQrBusy] = useState(false);
  const [passBusy, setPassBusy] = useState(false);
  const [bulkPreview, setBulkPreview] = useState<{ name: string; url: string } | null>(null);

  async function loadEmployeeQR() {
    if (!employeeID) {
      setMessage({ kind: "error", text: "сначала выберите сотрудника" });
      return;
    }
    setQrBusy(true);
    const res = await fetch(`/api/admin/employees/${employeeID}/qr.png`, {
      headers: { Authorization: `Bearer ${getToken()}` },
      cache: "no-store",
    });
    setQrBusy(false);
    if (!res.ok) {
      setMessage({ kind: "error", text: `не удалось получить QR-код (HTTP ${res.status})` });
      return;
    }
    const blob = await res.blob();
    setQrImage((current) => {
      if (current) URL.revokeObjectURL(current);
      return URL.createObjectURL(blob);
    });
    setMessage(null);
  }

  async function rotateEmployeeQR() {
    if (!employeeID) {
      setMessage({ kind: "error", text: "сначала выберите сотрудника" });
      return;
    }
    if (!confirm("Сменить QR сотрудника? Старый QR сразу перестанет действовать.")) return;
    setQrBusy(true);
    const res = await api(`/admin/employees/${employeeID}/qr/rotate`, { method: "POST" });
    setQrBusy(false);
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось сменить QR-код") });
      return;
    }
    await loadEmployeeQR();
    setMessage({ kind: "ok", text: "QR-код сотрудника изменён. Старый код отозван." });
  }

  async function uploadBulk(file: File) {
    const data = new FormData();
    data.append("file", file);
    const res = await api<ImportReport>("/admin/qr/bulk", { method: "POST", body: data });
    if (!res.ok || !res.body) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось загрузить QR") });
      return;
    }
    setBulk(res.body);
    setMessage({ kind: "ok", text: "Массовая загрузка обработана" });
  }

  const filteredEmployees = employees.items.filter((employee) => {
    const query = employeeSearch.trim().toLowerCase();
    if (!query) return true;
    return employee.full_name.toLowerCase().includes(query) || employee.employee_number.toLowerCase().includes(query);
  });

  function toggleEmployee(id: string) {
    setSelectedEmployees((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id]);
  }

  function selectAllVisible() {
    setSelectedEmployees((current) => Array.from(new Set([...current, ...filteredEmployees.map((employee) => employee.id)])));
  }

  async function generateBulkQR() {
    if (selectedEmployees.length === 0) {
      setMessage({ kind: "error", text: "выберите хотя бы одного сотрудника" });
      return;
    }
    setBulkQrBusy(true);
    const res = await api<BulkQRReport>("/admin/qr/employees/bulk", {
      method: "POST",
      body: JSON.stringify({ employee_ids: selectedEmployees }),
    });
    setBulkQrBusy(false);
    if (!res.ok || !res.body) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось сгенерировать QR-коды") });
      return;
    }
    setBulkEmployees(res.body);
    setMessage({
      kind: res.body.errors > 0 ? "error" : "ok",
      text: `Обработано: ${res.body.total}, создано: ${res.body.created}, уже были: ${res.body.existing}, ошибок: ${res.body.errors}`,
    });
  }

  async function showBulkQR(row: BulkQRRow) {
    const res = await fetch(`/api/admin/employees/${row.employee_id}/qr.png`, {
      headers: { Authorization: `Bearer ${getToken()}` },
      cache: "no-store",
    });
    if (!res.ok) {
      setMessage({ kind: "error", text: `не удалось получить QR-код (HTTP ${res.status})` });
      return;
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    setBulkPreview((current) => {
      if (current) URL.revokeObjectURL(current.url);
      return { name: `${row.employee_number ?? ""} ${row.full_name ?? ""}`.trim(), url };
    });
  }

  async function downloadPasses(employeeIDs: string[] = selectedEmployees) {
    if (employeeIDs.length === 0) {
      setMessage({ kind: "error", text: "выберите хотя бы одного сотрудника" });
      return;
    }
    setPassBusy(true);
    const res = await fetch("/api/admin/qr/passes/pdf", {
      method: "POST",
      headers: {
        Authorization: `Bearer ${getToken()}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ employee_ids: employeeIDs }),
    });
    setPassBusy(false);
    if (!res.ok) {
      let text = `не удалось сформировать пропуска (HTTP ${res.status})`;
      try {
        const body = await res.json() as { error?: string };
        if (body.error) text = body.error;
      } catch {
        // ответ не JSON
      }
      setMessage({ kind: "error", text });
      return;
    }
    const printed = res.headers.get("X-Passes-Printed");
    const skipped = res.headers.get("X-Passes-Skipped");
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    window.open(url, "_blank");
    setMessage({
      kind: skipped && skipped !== "0" ? "error" : "ok",
      text: `Пропусков в PDF: ${printed ?? "?"}${skipped && skipped !== "0" ? `, пропущено: ${skipped}` : ""}`,
    });
  }

  return (
    <section>
      <h2>QR-токены</h2>
      <Notice message={message} />
      <Field label="Сотрудник">
        <select value={employeeID} onChange={(e) => { setEmployeeID(e.target.value); setQrImage((current) => { if (current) URL.revokeObjectURL(current); return ""; }); }} required>
          <option value="">— выберите —</option>
          {employees.items.map((employee) => (
            <option key={employee.id} value={employee.id}>
              {employee.employee_number} — {employee.full_name}
            </option>
          ))}
        </select>
      </Field>

      <div className="card">
        <h3>QR сотрудника</h3>
        <p>Личный бессрочный QR создаётся автоматически при первом входе сотрудника в мобильное приложение.</p>
        <div className="actions">
          <button type="button" onClick={() => void loadEmployeeQR()} disabled={qrBusy}>Показать текущий QR</button>
          <button type="button" className="danger" onClick={() => void rotateEmployeeQR()} disabled={qrBusy}>
            {qrBusy ? "Смена…" : "Сменить QR"}
          </button>
        </div>
        {qrImage && (
          <div className="qr-code">
            <img src={qrImage} alt="QR-код сотрудника" className="qr-code-image" />
          </div>
        )}
      </div>

      <div className="card">
        <h3>Массовая генерация QR</h3>
        <p>Отметьте сотрудников: для каждого будет создан личный статический QR. Если активный QR уже есть, он сохраняется без изменений.</p>
        <div className="grid">
          <Field label="Поиск сотрудника">
            <input value={employeeSearch} onChange={(e) => setEmployeeSearch(e.target.value)} placeholder="ФИО или табельный номер" />
          </Field>
        </div>
        <div className="actions">
          <button type="button" onClick={selectAllVisible} disabled={filteredEmployees.length === 0}>Выбрать всех найденных</button>
          <button type="button" onClick={() => setSelectedEmployees([])} disabled={selectedEmployees.length === 0}>Снять выбор</button>
          <span>Выбрано: {selectedEmployees.length}</span>
        </div>
        <div className="employee-picker">
          {filteredEmployees.map((employee) => (
            <div key={employee.id} className="employee-picker-row">
              <input
                type="checkbox"
                checked={selectedEmployees.includes(employee.id)}
                onChange={() => toggleEmployee(employee.id)}
              />
              <span><b>{employee.employee_number}</b> — {employee.full_name}</span>
              <button type="button" className="secondary" onClick={() => void downloadPasses([employee.id])} disabled={passBusy}>
                Печать пропуска
              </button>
            </div>
          ))}
          {filteredEmployees.length === 0 && <p>Сотрудники не найдены.</p>}
        </div>
        <div className="actions">
          <button type="button" onClick={() => void generateBulkQR()} disabled={bulkQrBusy || selectedEmployees.length === 0}>
            {bulkQrBusy ? "Генерация…" : `Сгенерировать QR (${selectedEmployees.length})`}
          </button>
          <button type="button" className="secondary" onClick={() => void downloadPasses()} disabled={passBusy || selectedEmployees.length === 0}>
            {passBusy ? "Подготовка PDF…" : `Печать пропусков (${selectedEmployees.length})`}
          </button>
        </div>
        <p className="hint">Печать пропусков: карточка 85,6 × 53,98 мм, по 8 штук на листе A4. Если у сотрудника уже есть действующий бумажный QR, он будет использован повторно.</p>
        {bulkEmployees && (
          <>
            <p>Всего: {bulkEmployees.total}, создано: {bulkEmployees.created}, уже были: {bulkEmployees.existing}, ошибок: {bulkEmployees.errors}</p>
            <table>
              <thead><tr><th>Сотрудник</th><th>Результат</th><th>Причина</th><th></th></tr></thead>
              <tbody>
                {bulkEmployees.rows.map((row) => (
                  <tr key={row.employee_id}>
                    <td>{row.employee_number ?? "—"} — {row.full_name ?? row.employee_id}</td>
                    <td>{row.status === "created" ? "создан" : row.status === "existing" ? "уже был" : "ошибка"}</td>
                    <td>{row.reason ?? "—"}</td>
                    <td><button type="button" onClick={() => void showBulkQR(row)}>Показать QR</button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        )}
        {bulkPreview && (
          <div className="qr-code">
            <h4>{bulkPreview.name}</h4>
            <img src={bulkPreview.url} alt="QR-код сотрудника" className="qr-code-image" />
            <div className="actions">
              <button type="button" onClick={() => { URL.revokeObjectURL(bulkPreview.url); setBulkPreview(null); }}>Закрыть</button>
            </div>
          </div>
        )}
      </div>

      <div className="card">
        <h3>Массовая загрузка</h3>
        <p>JSON, NDJSON или ZIP.</p>
        <input type="file" onChange={(e) => { const file = e.target.files?.[0]; if (file) void uploadBulk(file); }} />
        {bulk && <p>Строк: {bulk.rows_total}, успешно: {bulk.rows_ok}, ошибок: {bulk.rows_error}</p>}
        {bulk && bulk.rows_error > 0 && (
          <table>
            <thead><tr><th>Строка</th><th>Причина</th></tr></thead>
            <tbody>
              {bulk.rows.filter((row) => row.status !== "ok").slice(0, 50).map((row) => (
                <tr key={row.line}><td>{row.line}</td><td>{row.reason ?? "—"}</td></tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </section>
  );
}

/* ---------------- Отметки ---------------- */

type Mark = {
  id: string;
  employee_id: string;
  employee_number?: string;
  full_name?: string;
  operator_number: string | null;
  event_type: string;
  timestamp_server: string;
  timestamp_client: string | null;
  overtime: boolean;
  outside_office: boolean;
  auto_closed_previous: boolean;
  qr_source: string | null;
  client_type: string | null;
  qr_data_mismatch: boolean;
  invalid_reason?: string | null;
  lat?: number | null;
  lon?: number | null;
  accuracy_m?: number | null;
  mock_location?: boolean;
};

function localDateTimeValue(date: Date) {
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function todayRange() {
  const start = new Date();
  start.setHours(0, 0, 0, 0);
  const end = new Date();
  end.setHours(23, 59, 0, 0);
  return { from: localDateTimeValue(start), to: localDateTimeValue(end) };
}

const marksPageSize = 50;

export function Marks() {
  const departments = useList<{ id: string; name: string }>("/admin/departments");
  const offices = useList<OfficeMapItem>("/admin/offices");
  const positions = useList<{ id: string; name: string }>("/admin/positions");
  const [filter, setFilter] = useState({ employee: "", department_id: "", position_id: "", ...todayRange() });
  const [items, setItems] = useState<Mark[]>([]);
  const [message, setMessage] = useState<Message>(null);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [selectedMark, setSelectedMark] = useState<string | null>(null);
  const [mapOpen, setMapOpen] = useState(false);
  const [mapReady, setMapReady] = useState(false);
  const mapContainerRef = useRef<HTMLDivElement | null>(null);
  const mapRef = useRef<maplibregl.Map | null>(null);

  useEffect(() => {
    if (!mapOpen || !mapContainerRef.current || mapRef.current) return;
    const map = new maplibregl.Map({
      container: mapContainerRef.current,
      style: OFFLINE_MAP_STYLE,
      center: [82.9204, 55.0302],
      zoom: 13.2,
      minZoom: 12,
      maxZoom: 18.5,
      maxBounds: [[82.60, 54.80], [83.20, 55.18]],
      attributionControl: false,
    });
    mapRef.current = map;
    map.addControl(new maplibregl.NavigationControl({ showCompass: false }), "top-right");
    map.addControl(new maplibregl.ScaleControl({ unit: "metric" }), "bottom-left");
    map.addControl(new maplibregl.AttributionControl({
      compact: true,
      customAttribution: "© OpenStreetMap contributors · OpenFreeMap",
    }), "bottom-right");

    map.on("load", () => {
      map.addSource("office-points", { type: "geojson", data: officePointCollection(offices.items) });
      map.addSource("office-zones", { type: "geojson", data: officeZoneCollection(offices.items) });
      map.addLayer({
        id: "office-zone-fill",
        type: "fill",
        source: "office-zones",
        paint: {
          "fill-color": ["case", ["get", "is_active"], "#1d4ed8", "#6b7280"],
          "fill-opacity": 0.13,
        },
      }, "marks-halo");
      map.addLayer({
        id: "office-zone-line",
        type: "line",
        source: "office-zones",
        paint: {
          "line-color": ["case", ["get", "is_active"], "#1d4ed8", "#6b7280"],
          "line-width": 1.8,
          "line-dasharray": [3, 2],
        },
      }, "marks-halo");
      map.addLayer({
        id: "office-point-halo",
        type: "circle",
        source: "office-points",
        paint: { "circle-radius": 9, "circle-color": "#ffffff", "circle-opacity": 0.95 },
      }, "marks-halo");
      map.addLayer({
        id: "office-point",
        type: "circle",
        source: "office-points",
        paint: {
          "circle-radius": 6,
          "circle-color": ["case", ["get", "is_active"], "#15539c", "#6b7280"],
          "circle-stroke-color": "#ffffff",
          "circle-stroke-width": 2,
        },
      }, "marks-halo");
      map.addLayer({
        id: "office-labels",
        type: "symbol",
        source: "office-points",
        layout: {
          "text-field": ["get", "name"],
          "text-font": ["Noto Sans Bold"],
          "text-size": 11,
          "text-offset": [0, 1.3],
          "text-anchor": "top",
          "text-max-width": 10,
        },
        paint: {
          "text-color": "#152238",
          "text-halo-color": "#ffffff",
          "text-halo-width": 2,
        },
      }, "marks-halo");
      setMapReady(true);
      const selectMark = (event: maplibregl.MapLayerMouseEvent) => {
        const id = event.features?.[0]?.properties?.id;
        if (typeof id === "string") setSelectedMark(id);
      };
      map.on("click", "marks-circles", selectMark);
      map.on("click", "marks-selected", selectMark);
      map.on("mouseenter", "marks-circles", () => { map.getCanvas().style.cursor = "pointer"; });
      map.on("mouseleave", "marks-circles", () => { map.getCanvas().style.cursor = ""; });
    });

    return () => {
      map.remove();
      mapRef.current = null;
      setMapReady(false);
    };
  }, [mapOpen]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map || !mapReady) return;
    const source = map.getSource("marks") as maplibregl.GeoJSONSource | undefined;
    if (!source) return;
    source.setData({
      type: "FeatureCollection",
      features: items
        .filter((mark) => typeof mark.lat === "number" && typeof mark.lon === "number")
        .map((mark) => ({
          type: "Feature" as const,
          id: mark.id,
          geometry: { type: "Point" as const, coordinates: [mark.lon as number, mark.lat as number] },
          properties: {
            id: mark.id,
            event_type: mark.event_type,
            outside_office: mark.outside_office,
          },
        })),
    });
  }, [items, mapReady]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map || !mapReady) return;
    const points = map.getSource("office-points") as maplibregl.GeoJSONSource | undefined;
    const zones = map.getSource("office-zones") as maplibregl.GeoJSONSource | undefined;
    points?.setData(officePointCollection(offices.items) as GeoJSON.FeatureCollection);
    zones?.setData(officeZoneCollection(offices.items) as GeoJSON.FeatureCollection);
  }, [offices.items, mapReady]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map || !mapReady) return;
    map.setFilter("marks-selected", ["==", ["get", "id"], selectedMark ?? ""]);
  }, [selectedMark, mapReady]);

  async function load(targetPage = page) {
    const employeeQuery = filter.employee.trim();
    if (employeeQuery.length > 0 && employeeQuery.length < 3) {
      return;
    }
    const params = new URLSearchParams();
    if (employeeQuery.length >= 3) params.set("q", employeeQuery);
    if (filter.department_id) params.set("department_id", filter.department_id);
    if (filter.position_id) params.set("position_id", filter.position_id);
    if (filter.from) params.set("from", new Date(filter.from).toISOString());
    if (filter.to) params.set("to", new Date(filter.to).toISOString());
    params.set("limit", String(marksPageSize + 1));
    params.set("offset", String(targetPage * marksPageSize));
    setLoading(true);
    const res = await api<Mark[]>(`/admin/marks?${params.toString()}`);
    if (res.ok && Array.isArray(res.body)) {
      setHasMore(res.body.length > marksPageSize);
      setItems(res.body.slice(0, marksPageSize));
      setMessage(null);
    } else {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось загрузить отметки") });
    }
    setLoading(false);
  }

  function updateFilter(patch: Partial<typeof filter>) {
    setPage(0);
    setFilter((current) => ({ ...current, ...patch }));
  }

  useEffect(() => {
    const timer = setTimeout(() => void load(page), 300);
    return () => clearTimeout(timer);
  }, [filter, page]);

  return (
    <section>
      <h2>Отметки</h2>
      <Notice message={message} />
      <div className="grid">
        <Field label="Сотрудник (ФИО или номер)">
          <input
            value={filter.employee}
            placeholder="минимум 3 символа"
            onChange={(e) => updateFilter({ employee: e.target.value })}
          />
        </Field>
        <Field label="Отдел">
          <select value={filter.department_id} onChange={(e) => updateFilter({ department_id: e.target.value })}>
            <option value="">Все отделы</option>
            {departments.items.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </Field>
        <Field label="Должность">
          <select value={filter.position_id} onChange={(e) => updateFilter({ position_id: e.target.value })}>
            <option value="">Все должности</option>
            {positions.items.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </Field>
        <Field label="С даты"><input type="datetime-local" value={filter.from} onChange={(e) => updateFilter({ from: e.target.value })} /></Field>
        <Field label="По дату"><input type="datetime-local" value={filter.to} onChange={(e) => updateFilter({ to: e.target.value })} /></Field>
      </div>
      <div className="actions">
        <button type="button" onClick={() => void load()} disabled={loading}>Показать</button>
        <button type="button" onClick={() => { setPage(0); setFilter({ employee: "", department_id: "", position_id: "", ...todayRange() }); }}>Текущий день</button>
      </div>
      {filter.employee.trim().length > 0 && filter.employee.trim().length < 3 && (
        <p className="hint">Введите минимум 3 символа для поиска сотрудника.</p>
      )}
      {(() => {
        const withGeo = items.filter((mark) => typeof mark.lat === "number" && typeof mark.lon === "number");
        const selected = items.find((mark) => mark.id === selectedMark) ?? null;
        return (
          <div className="card">
            <div className="map-card-header">
              <div>
                <h3>Геоданные на карте Новосибирска</h3>
                <p className="hint">
                  Офлайн-карта с детализацией до домов. Синяя зона — разрешённая зона отметки офиса. На текущей странице: {withGeo.length} отметок с координатами.
                </p>
              </div>
              <button type="button" onClick={() => setMapOpen((value) => !value)}>
                {mapOpen ? "Свернуть карту" : "Показать карту"}
              </button>
            </div>
            {mapOpen && (
              <>
            <div className="offline-map" ref={mapContainerRef} />
            <p className="hint">Колесо мыши или кнопки справа — масштаб; перетаскивание — перемещение. Приближение до отдельных домов.</p>
            {selected && (
              <div className="map-selected">
                <b>{selected.full_name ?? "—"}</b> · {selected.employee_number ?? selected.employee_id}
                <div>{eventLabel(selected.event_type)} · {fmt(selected.timestamp_server)}</div>
                <div className="mono">Широта: {selected.lat}; долгота: {selected.lon}; точность: {selected.accuracy_m ?? "—"} м</div>
                {selected.outside_office && <div className="error">Вне геозоны офиса</div>}
                {selected.mock_location && <div className="error">Подозрение на фиктивную геолокацию</div>}
                {selected.invalid_reason && <div className="error">{selected.invalid_reason}</div>}
              </div>
            )}
            {withGeo.length === 0 && <p>За выбранный период координаты отсутствуют.</p>}
              </>
            )}
          </div>
        );
      })()}
      <table>
        <thead>
          <tr><th>Время (сервер)</th><th>Сотрудник</th><th>Тип</th><th>Оператор</th><th>Источник</th><th>Флаги</th></tr>
        </thead>
        <tbody>
          {items.map((mark) => (
            <tr key={mark.id} className={mark.invalid_reason ? "row-invalid" : ""}>
              <td>{fmt(mark.timestamp_server)}</td>
              <td>
                {mark.full_name ?? "—"}
                <div className="mono">{mark.employee_number ?? mark.employee_id}</div>
              </td>
              <td>{eventLabel(mark.event_type)}</td>
              <td>{mark.operator_number ?? "—"}</td>
              <td>
                {[mark.qr_source ? qrSourceLabel(mark.qr_source) : "", mark.client_type ? clientTypeLabel(mark.client_type) : ""]
                  .filter(Boolean)
                  .join(" · ") || "—"}
              </td>
              <td>
                {mark.overtime ? "переработка " : ""}
                {mark.outside_office ? "вне офиса " : ""}
                {mark.auto_closed_previous ? "авто-закрытие " : ""}
                {mark.qr_data_mismatch ? "расхождение QR " : ""}
                {mark.invalid_reason ? mark.invalid_reason : ""}
              </td>
            </tr>
          ))}
          {!loading && items.length === 0 && <tr><td colSpan={6}>За выбранный период записей нет</td></tr>}
        </tbody>
      </table>
      <div className="actions">
        <button type="button" onClick={() => setPage((value) => Math.max(0, value - 1))} disabled={loading || page === 0}>Назад</button>
        <span>Страница {page + 1}</span>
        <button type="button" onClick={() => setPage((value) => value + 1)} disabled={loading || !hasMore}>Вперёд</button>
      </div>
      {loading && <p>Загрузка…</p>}
    </section>
  );
}

/* ---------------- Делегирования ---------------- */

type Delegation = {
  id: string;
  from_user_id: string;
  to_user_id: string;
  from_login?: string;
  to_login?: string;
  status: string;
  requested_at: string;
  valid_until: string;
  decided_at: string | null;
};

type UserLite = { id: string; login: string; full_name?: string | null; employee_number?: string | null };

export function Delegations() {
  const { items, loading, message, setMessage, reload } = useList<Delegation>("/delegations");
  const users = useList<UserLite>("/admin/users?limit=500");
  const userByID = new Map(users.items.map((user) => [user.id, user]));
  const userText = (id: string) => {
    const user = userByID.get(id);
    if (!user) return id;
    return user.full_name ? `${user.login} — ${user.full_name}` : user.login;
  };

  async function action(id: string, name: "approve" | "reject" | "revoke") {
    const res = await api(`/delegations/${id}/${name}`, { method: "POST" });
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось изменить делегирование") });
      return;
    }
    setMessage({ kind: "ok", text: "Готово" });
    await reload();
  }

  return (
    <section>
      <h2>Делегирования</h2>
      <Notice message={message} />
      <table>
        <thead><tr><th>От</th><th>Кому</th><th>Статус</th><th>Запрошено</th><th>Действует до</th><th /></tr></thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.id}>
              <td>{item.from_login ?? userText(item.from_user_id)}</td>
              <td>{item.to_login ?? userText(item.to_user_id)}</td>
              <td>{delegationStatusLabel(item.status)}</td>
              <td>{fmt(item.requested_at)}</td>
              <td>{fmt(item.valid_until)}</td>
              <td className="actions-cell">
                {item.status === "pending" && <>
                  <button type="button" onClick={() => void action(item.id, "approve")}>Подтвердить</button>
                  <button type="button" onClick={() => void action(item.id, "reject")}>Отклонить</button>
                </>}
                {(item.status === "pending" || item.status === "approved") &&
                  <button type="button" onClick={() => void action(item.id, "revoke")}>Отозвать</button>}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {loading && <p>Загрузка…</p>}
    </section>
  );
}

/* ---------------- Уведомления ---------------- */

type Notification = {
  id: string;
  type: string;
  title: string;
  body: string | null;
  is_read: boolean;
  created_at: string;
};

export function Notifications() {
  const { items, loading, message, setMessage, reload } = useList<Notification>("/notifications?limit=200");

  async function read(id: string) {
    const res = await api(`/notifications/${id}/read`, { method: "POST" });
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось отметить уведомление") });
      return;
    }
    await reload();
  }

  return (
    <section>
      <h2>Уведомления</h2>
      <Notice message={message} />
      <table>
        <thead><tr><th>Время</th><th>Тип</th><th>Заголовок</th><th>Текст</th><th /></tr></thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.id} className={item.is_read ? "muted" : ""}>
              <td>{fmt(item.created_at)}</td>
              <td>{notificationTypeLabel(item.type)}</td>
              <td>{item.title}</td>
              <td>{item.body ?? "—"}</td>
              <td>{!item.is_read && <button type="button" onClick={() => void read(item.id)}>Прочитано</button>}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {loading && <p>Загрузка…</p>}
    </section>
  );
}

/* ---------------- OAuth ---------------- */

type OAuthClient = {
  id: string;
  client_id: string;
  name: string | null;
  is_active: boolean;
  secret_rotated_at: string | null;
  next_rotation_at: string | null;
  client_secret?: string;
};

type WhitelistEntry = { id: string; cidr: string; description: string | null };

export function OAuthPanel() {
  const { items, loading, message, setMessage, reload } = useList<OAuthClient>("/admin/oauth/clients");
  const [name, setName] = useState("");
  const [secret, setSecret] = useState("");
  const [whitelistFor, setWhitelistFor] = useState("");
  const [entries, setEntries] = useState<WhitelistEntry[]>([]);
  const [cidr, setCidr] = useState("");

  async function create(event: FormEvent) {
    event.preventDefault();
    const res = await api<OAuthClient>("/admin/oauth/clients", { method: "POST", body: JSON.stringify({ name }) });
    if (!res.ok || !res.body) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось создать клиента") });
      return;
    }
    setSecret(res.body.client_secret ?? "");
    setName("");
    setMessage({ kind: "ok", text: "Клиент создан. Сохраните секрет — он показывается один раз." });
    await reload();
  }

  async function rotate(id: string) {
    const res = await api<OAuthClient>(`/admin/oauth/clients/${id}/rotate`, { method: "POST" });
    if (!res.ok || !res.body) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось ротировать секрет") });
      return;
    }
    setSecret(res.body.client_secret ?? "");
    setMessage({ kind: "ok", text: "Секрет ротирован. Старые токены отозваны." });
    await reload();
  }

  async function toggleActive(client: OAuthClient) {
    const res = await api<OAuthClient>(`/admin/oauth/clients/${client.id}`, {
      method: "PATCH",
      body: JSON.stringify({ is_active: !client.is_active }),
    });
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось изменить состояние клиента") });
      return;
    }
    setMessage({ kind: "ok", text: client.is_active ? "Клиент отключён, выданные токены отозваны" : "Клиент включён" });
    await reload();
  }

  async function removeClient(client: OAuthClient) {
    if (!confirm(`Удалить OAuth-клиент «${client.name ?? client.client_id}»? Все его токены и IP-whitelist будут удалены.`)) return;
    const res = await api(`/admin/oauth/clients/${client.id}`, { method: "DELETE" });
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось удалить клиента") });
      return;
    }
    if (whitelistFor === client.id) {
      setWhitelistFor("");
      setEntries([]);
    }
    setMessage({ kind: "ok", text: "OAuth-клиент удалён" });
    await reload();
  }

  async function loadWhitelist(id: string) {
    setWhitelistFor(id);
    const res = await api<WhitelistEntry[]>(`/admin/oauth/clients/${id}/whitelist`);
    if (res.ok && Array.isArray(res.body)) setEntries(res.body);
  }

  async function addWhitelist(event: FormEvent) {
    event.preventDefault();
    const res = await api(`/admin/oauth/clients/${whitelistFor}/whitelist`, {
      method: "POST",
      body: JSON.stringify({ cidr }),
    });
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось добавить адрес") });
      return;
    }
    setCidr("");
    await loadWhitelist(whitelistFor);
  }

  return (
    <section>
      <h2>OAuth-клиенты и список разрешённых IP</h2>
      <Notice message={message} />
      <form className="toolbar" onSubmit={create}>
        <input placeholder="Название клиента" value={name} onChange={(e) => setName(e.target.value)} required />
        <button type="submit">Создать</button>
      </form>
      {secret && <p className="ok mono">Секрет клиента: {secret}</p>}
      <table>
        <thead><tr><th>Идентификатор клиента</th><th>Название</th><th>Статус</th><th>Следующая ротация</th><th /></tr></thead>
        <tbody>
          {items.map((client) => (
            <tr key={client.id} className={client.is_active ? "" : "muted"}>
              <td className="mono">{client.client_id}</td>
              <td>{client.name ?? "—"}</td>
              <td>{client.is_active ? "активен" : "отключён"}</td>
              <td>{fmt(client.next_rotation_at)}</td>
              <td className="actions-cell">
                <button type="button" onClick={() => void rotate(client.id)}>Ротировать</button>
                <button type="button" onClick={() => void loadWhitelist(client.id)}>Разрешённые IP</button>
                <button type="button" onClick={() => void toggleActive(client)}>
                  {client.is_active ? "Отключить" : "Включить"}
                </button>
                <button type="button" className="danger" onClick={() => void removeClient(client)}>Удалить</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {whitelistFor && (
        <div className="card">
          <h3>Разрешённые IP клиента</h3>
          <form className="toolbar" onSubmit={addWhitelist}>
            <input placeholder="Подсеть CIDR, например 203.0.113.0/24" value={cidr} onChange={(e) => setCidr(e.target.value)} required />
            <button type="submit">Добавить</button>
          </form>
          <p>Если список пуст — доступ разрешён с любого IP.</p>
          <ul>
            {entries.map((entry) => <li key={entry.id} className="mono">{entry.cidr} {entry.description ?? ""}</li>)}
          </ul>
        </div>
      )}
      {loading && <p>Загрузка…</p>}
    </section>
  );
}

/* ---------------- Офлайн-импорт ---------------- */

export function OfflineImport() {
  const [report, setReport] = useState<ImportReport | null>(null);
  const [message, setMessage] = useState<Message>(null);

  async function upload(file: File) {
    const data = new FormData();
    data.append("file", file);
    const res = await api<ImportReport>("/admin/offline-imports", { method: "POST", body: data });
    if (!res.ok || !res.body) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось выполнить импорт") });
      return;
    }
    setReport(res.body);
    setMessage({ kind: "ok", text: "Импорт выполнен" });
  }

  return (
    <section>
      <h2>Офлайн-импорт отметок</h2>
      <Notice message={message} />
      <p>CSV: <code>employee_number,event_type,timestamp_server,idempotency_key,...</code></p>
      <input type="file" accept=".csv,text/csv" onChange={(e) => { const file = e.target.files?.[0]; if (file) void upload(file); }} />
      {report && (
        <>
          <p>Строк: {report.rows_total}, успешно: {report.rows_ok}, ошибок: {report.rows_error}</p>
          <table>
            <thead><tr><th>Строка</th><th>Статус</th><th>Причина</th></tr></thead>
            <tbody>
              {report.rows.filter((row) => row.status !== "ok").map((row) => (
                <tr key={row.line}><td>{row.line}</td><td>{row.status}</td><td>{row.reason ?? "—"}</td></tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </section>
  );
}

/* ---------------- Настройки ---------------- */

type SystemProfile = {
  main_admin_email: string;
  support_phone: string;
  support_email: string;
  company_name: string;
  company_address: string;
  company_inn: string;
  website: string;
  paper_qr_valid_days: number;
  default_office_radius_m: number;
  pass_footer: string;
};

const emptySystemProfile: SystemProfile = {
  main_admin_email: "",
  support_phone: "",
  support_email: "",
  company_name: "",
  company_address: "",
  company_inn: "",
  website: "",
  paper_qr_valid_days: 30,
  default_office_radius_m: 150,
  pass_footer: "",
};

type SystemConnectionSettings = {
  database_url: string;
  database_url_set: boolean;
  redis_addr: string;
  redis_password_set: boolean;
  qr_private_key_set: boolean;
  qr_public_key: string;
  settings_db_path: string;
  app_env: string;
  restart_required?: boolean;
};

const emptySystemConnections: SystemConnectionSettings = {
  database_url: "",
  database_url_set: false,
  redis_addr: "",
  redis_password_set: false,
  qr_private_key_set: false,
  qr_public_key: "",
  settings_db_path: "",
  app_env: "",
};

export function SystemSettings() {
  const [form, setForm] = useState<SystemProfile>(emptySystemProfile);
  const [message, setMessage] = useState<Message>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [connections, setConnections] = useState<SystemConnectionSettings>(emptySystemConnections);
  const [databaseURL, setDatabaseURL] = useState("");
  const [redisAddr, setRedisAddr] = useState("");
  const [redisPassword, setRedisPassword] = useState("");
  const [connectionsMessage, setConnectionsMessage] = useState<Message>(null);
  const [connectionsBusy, setConnectionsBusy] = useState(false);
  const [testBusy, setTestBusy] = useState(false);
  const [connectionsLoading, setConnectionsLoading] = useState(true);

  useEffect(() => {
    void (async () => {
      const res = await api<SystemProfile>("/admin/profile");
      if (res.ok && res.body) setForm({ ...emptySystemProfile, ...res.body });
      else setMessage({ kind: "error", text: errorText(res.body, "не удалось загрузить настройки") });
      setLoading(false);
    })();
  }, []);

  useEffect(() => {
    void (async () => {
      const res = await api<SystemConnectionSettings>("/admin/system-settings");
      if (res.ok && res.body) {
        setConnections({ ...emptySystemConnections, ...res.body });
        setRedisAddr(res.body.redis_addr);
      } else {
        setConnectionsMessage({ kind: "error", text: errorText(res.body, "не удалось загрузить системные настройки") });
      }
      setConnectionsLoading(false);
    })();
  }, []);

  function update<K extends keyof SystemProfile>(key: K, value: SystemProfile[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  async function save(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    const res = await api<SystemProfile>("/admin/profile", { method: "PUT", body: JSON.stringify(form) });
    setBusy(false);
    if (!res.ok || !res.body) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось сохранить настройки") });
      return;
    }
    setForm({ ...emptySystemProfile, ...res.body });
    setMessage({ kind: "ok", text: "Настройки сохранены" });
  }

  async function testConnections() {
    setTestBusy(true);
    setConnectionsMessage(null);
    const res = await api("/admin/system-settings/test", {
      method: "POST",
      body: JSON.stringify({
        database_url: databaseURL.trim(),
        redis_addr: redisAddr.trim(),
        redis_password: redisPassword,
      }),
    });
    setTestBusy(false);
    if (!res.ok) {
      setConnectionsMessage({ kind: "error", text: errorText(res.body, "не удалось подключиться") });
      return;
    }
    setConnectionsMessage({ kind: "ok", text: "PostgreSQL и Redis доступны" });
  }

  async function saveConnections(event: FormEvent) {
    event.preventDefault();
    setConnectionsBusy(true);
    setConnectionsMessage(null);
    const res = await api<SystemConnectionSettings>("/admin/system-settings", {
      method: "PUT",
      body: JSON.stringify({
        database_url: databaseURL.trim(),
        redis_addr: redisAddr.trim(),
        redis_password: redisPassword,
      }),
    });
    setConnectionsBusy(false);
    if (!res.ok || !res.body) {
      setConnectionsMessage({ kind: "error", text: errorText(res.body, "не удалось сохранить подключения") });
      return;
    }
    setConnections({ ...emptySystemConnections, ...res.body });
    setRedisAddr(res.body.redis_addr);
    setDatabaseURL("");
    setRedisPassword("");
    setConnectionsMessage({
      kind: "ok",
      text: res.body.restart_required
        ? "Подключения сохранены. Для применения перезапустите backend."
        : "Подключения проверены, изменения не требовались.",
    });
  }

  return (
    <section>
      <h2>Настройки системы</h2>
      <Notice message={message} />
      {loading ? <p>Загрузка…</p> : (
        <form onSubmit={save}>
          <div className="card">
            <h3>Организация</h3>
            <div className="grid">
              <Field label="Название компании (печатается на пропуске)">
                <input
                  value={form.company_name}
                  onChange={(e) => update("company_name", e.target.value)}
                  maxLength={200}
                  placeholder="Например, ООО «Ромашка»"
                />
              </Field>
              <Field label="ИНН">
                <input value={form.company_inn} onChange={(e) => update("company_inn", e.target.value)} maxLength={12} />
              </Field>
            </div>
            <Field label="Юридический адрес">
              <input value={form.company_address} onChange={(e) => update("company_address", e.target.value)} maxLength={300} />
            </Field>
            <Field label="Сайт">
              <input value={form.website} onChange={(e) => update("website", e.target.value)} maxLength={200} placeholder="https://example.ru" />
            </Field>
          </div>

          <div className="card">
            <h3>Контакты</h3>
            <div className="grid">
              <Field label="Телефон поддержки">
                <input value={form.support_phone} onChange={(e) => update("support_phone", e.target.value)} maxLength={40} />
              </Field>
              <Field label="Email поддержки">
                <input type="email" value={form.support_email} onChange={(e) => update("support_email", e.target.value)} maxLength={200} />
              </Field>
              <Field label="Email главного администратора">
                <input type="email" value={form.main_admin_email} onChange={(e) => update("main_admin_email", e.target.value)} maxLength={200} />
              </Field>
            </div>
          </div>

          <div className="card">
            <h3>Пропуска и офисы</h3>
            <div className="grid">
              <Field label="Срок действия бумажного пропуска, дней">
                <input
                  type="number"
                  min={1}
                  max={365}
                  value={form.paper_qr_valid_days}
                  onChange={(e) => update("paper_qr_valid_days", Number(e.target.value))}
                />
              </Field>
              <Field label="Радиус зоны офиса по умолчанию, м">
                <input
                  type="number"
                  min={10}
                  max={10000}
                  value={form.default_office_radius_m}
                  onChange={(e) => update("default_office_radius_m", Number(e.target.value))}
                />
              </Field>
            </div>
            <Field label="Строка в нижнем колонтитуле пропуска">
              <input
                value={form.pass_footer}
                onChange={(e) => update("pass_footer", e.target.value)}
                maxLength={200}
                placeholder="Например, ООО «Ромашка» · отдел кадров"
              />
            </Field>
            <p className="hint">Название компании печатается в шапке пропуска. Если поле пустое, используется «Табель».</p>
          </div>

          <div className="actions">
            <button type="submit" disabled={busy}>{busy ? "Сохранение…" : "Сохранить все настройки"}</button>
          </div>
        </form>
      )}

      <div className="card">
        <h3>Подключение к серверам</h3>
        <Notice message={connectionsMessage} />
        {connectionsLoading ? <p>Загрузка…</p> : (
          <form onSubmit={saveConnections}>
            <Field label="Новая строка подключения PostgreSQL">
              <input
                type="password"
                value={databaseURL}
                onChange={(e) => setDatabaseURL(e.target.value)}
                placeholder="Оставьте пустым, чтобы не менять"
                autoComplete="new-password"
              />
            </Field>
            <p className="hint mono">
              Текущее подключение: {connections.database_url_set ? connections.database_url : "не задано"}
            </p>
            <div className="grid">
              <Field label="Адрес Redis (host:port)">
                <input value={redisAddr} onChange={(e) => setRedisAddr(e.target.value)} placeholder="redis:6379" />
              </Field>
              <Field label="Новый пароль Redis">
                <input
                  type="password"
                  value={redisPassword}
                  onChange={(e) => setRedisPassword(e.target.value)}
                  placeholder={connections.redis_password_set ? "Задан, оставьте пустым" : "Не задан"}
                  autoComplete="new-password"
                />
              </Field>
            </div>
            <p className="hint">
              Проверка PostgreSQL создаёт указанную базу при необходимости и применяет миграции.
              При смене адреса Redis пустой пароль означает подключение без пароля.
              Изменения подключений вступают в силу после перезапуска backend.
            </p>
            <div className="actions">
              <button type="button" onClick={() => void testConnections()} disabled={testBusy || connectionsBusy}>
                {testBusy ? "Проверка…" : "Проверить подключение"}
              </button>
              <button type="submit" disabled={connectionsBusy || testBusy}>
                {connectionsBusy ? "Сохранение…" : "Сохранить подключения"}
              </button>
            </div>
          </form>
        )}
      </div>

      <InfraSettings />

      <div className="card">
        <h3>QR-ключ и хранилище настроек</h3>
        <p>Приватный ключ QR: {connections.qr_private_key_set ? "задан" : "не задан"}. Ключ хранится в зашифрованном виде и не отображается.</p>
        {connections.qr_public_key && (
          <details>
            <summary>Публичный ключ QR</summary>
            <textarea className="mono" readOnly rows={7} value={connections.qr_public_key} />
          </details>
        )}
        <p className="hint mono">Хранилище: {connections.settings_db_path || "—"}</p>
        <p className="hint mono">Окружение: {connections.app_env || "—"}</p>
      </div>
    </section>
  );
}

/* ---------------- Аудит ---------------- */

type AuditEntry = {
  id: number;
  actor_user_id: string | null;
  action: string;
  entity_type: string | null;
  entity_id: string | null;
  details: unknown;
  ip: string | null;
  created_at: string;
};

function auditDetails(entry: AuditEntry): Record<string, unknown> {
  if (entry.details && typeof entry.details === "object" && !Array.isArray(entry.details)) {
    return entry.details as Record<string, unknown>;
  }
  return {};
}

function auditSummary(entry: AuditEntry): string {
  const action = auditActionLabel(entry.action);
  const details = auditDetails(entry);
  const subject = details.full_name ?? details.employee_number ?? details.name ?? details.login ?? details.filename;
  if (subject) return `${action}: ${String(subject)}`;
  if (details.rows_total !== undefined) {
    return `${action}: успешно ${String(details.rows_ok ?? 0)} из ${String(details.rows_total)}`;
  }
  return action;
}

type AuditFilters = {
  from: string;
  to: string;
  action: string;
  entity_type: string;
  actor_user_id: string;
  ip: string;
};

const emptyAuditFilters: AuditFilters = {
  from: "",
  to: "",
  action: "",
  entity_type: "",
  actor_user_id: "",
  ip: "",
};

const auditPageSize = 50;

export function Audit() {
  const users = useList<UserLite>("/admin/users?limit=500");
  const employees = useList<Employee>("/admin/employees?limit=500");
  const [filters, setFilters] = useState<AuditFilters>(emptyAuditFilters);
  const [page, setPage] = useState(0);
  const [selected, setSelected] = useState<AuditEntry | null>(null);

  const params = new URLSearchParams();
  params.set("limit", String(auditPageSize + 1));
  params.set("offset", String(page * auditPageSize));
  if (filters.action) params.set("action", filters.action);
  if (filters.entity_type) params.set("entity_type", filters.entity_type);
  if (filters.actor_user_id) params.set("actor_user_id", filters.actor_user_id);
  if (filters.ip.trim()) params.set("ip", filters.ip.trim());
  if (filters.from) params.set("from", new Date(filters.from).toISOString());
  if (filters.to) params.set("to", new Date(filters.to).toISOString());
  const { items: loaded, loading, message, reload } = useList<AuditEntry>(`/admin/audit?${params.toString()}`);
  const hasMore = loaded.length > auditPageSize;
  const items = loaded.slice(0, auditPageSize);

  const userByID = new Map(users.items.map((user) => [user.id, user]));
  const employeeByID = new Map(employees.items.map((employee) => [employee.id, employee]));
  const actorText = (id: string | null) => {
    if (!id) return "система";
    const user = userByID.get(id);
    return user ? (user.full_name ? `${user.login} — ${user.full_name}` : user.login) : id;
  };
  const employeeText = (id: string | null | undefined) => {
    if (!id) return "—";
    const employee = employeeByID.get(id);
    return employee ? `${employee.employee_number} — ${employee.full_name}` : id;
  };
  const detailValue = (key: string, value: unknown) => {
    if (key === "employee_id" && typeof value === "string") {
      return employeeText(value);
    }
    return auditValueLabel(key, value);
  };
  function updateFilter<K extends keyof AuditFilters>(key: K, value: AuditFilters[K]) {
    setPage(0);
    setFilters((current) => ({ ...current, [key]: value }));
  }
  function resetFilters() {
    setPage(0);
    setFilters(emptyAuditFilters);
  }

  return (
    <section>
      <h2>Аудит</h2>
      <Notice message={message} />
      <div className="actions">
        <button type="button" onClick={() => void reload()} disabled={loading}>Обновить</button>
        <button type="button" className="secondary" onClick={resetFilters}>Сбросить фильтры</button>
      </div>
      <table>
        <thead>
          <tr><th>Время</th><th>Что выполнено</th><th>Объект</th><th>Пользователь</th><th>IP-адрес</th><th /></tr>
          <tr className="filter-row">
            <th>
              <div className="audit-time-filter">
                <input type="datetime-local" aria-label="С даты" value={filters.from} onChange={(e) => updateFilter("from", e.target.value)} />
                <input type="datetime-local" aria-label="По дату" value={filters.to} onChange={(e) => updateFilter("to", e.target.value)} />
              </div>
            </th>
            <th>
              <select aria-label="Действие" value={filters.action} onChange={(e) => updateFilter("action", e.target.value)}>
                <option value="">Все действия</option>
                {auditActionOptions.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
              </select>
            </th>
            <th>
              <select aria-label="Объект" value={filters.entity_type} onChange={(e) => updateFilter("entity_type", e.target.value)}>
                <option value="">Все объекты</option>
                {auditEntityOptions.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
              </select>
            </th>
            <th>
              <select aria-label="Пользователь" value={filters.actor_user_id} onChange={(e) => updateFilter("actor_user_id", e.target.value)}>
                <option value="">Все пользователи</option>
                {users.items.map((user) => (
                  <option key={user.id} value={user.id}>{user.login}{user.full_name ? ` — ${user.full_name}` : ""}</option>
                ))}
              </select>
            </th>
            <th><input aria-label="IP-адрес" placeholder="Фильтр по IP" value={filters.ip} onChange={(e) => updateFilter("ip", e.target.value)} /></th>
            <th />
          </tr>
        </thead>
        <tbody>
          {items.map((entry) => (
            <tr key={entry.id}>
              <td>{fmt(entry.created_at)}</td>
              <td>{auditSummary(entry)}</td>
              <td>{auditEntityLabel(entry.entity_type)}</td>
              <td>{actorText(entry.actor_user_id)}</td>
              <td className="mono">{entry.ip ?? "—"}</td>
              <td><button type="button" className="secondary" onClick={() => setSelected(entry)}>Подробности</button></td>
            </tr>
          ))}
          {!loading && items.length === 0 && <tr><td colSpan={6}>Записей нет</td></tr>}
        </tbody>
      </table>
      <div className="actions">
        <button type="button" onClick={() => setPage((value) => Math.max(0, value - 1))} disabled={loading || page === 0}>Назад</button>
        <span>Страница {page + 1}</span>
        <button type="button" onClick={() => setPage((value) => value + 1)} disabled={loading || !hasMore}>Вперёд</button>
      </div>
      {loading && <p>Загрузка…</p>}
      {selected && (
        <div className="modal-backdrop" onClick={() => setSelected(null)}>
          <div className="modal" onClick={(event) => event.stopPropagation()}>
            <h3>Подробности операции</h3>
            <dl className="status">
              <dt>Что выполнено</dt><dd>{auditActionLabel(selected.action)}</dd>
              <dt>Объект</dt><dd>{auditEntityLabel(selected.entity_type)}</dd>
              <dt>Идентификатор</dt>
              <dd className="mono">{selected.entity_type === "employee" ? employeeText(selected.entity_id) : selected.entity_id ?? "—"}</dd>
              <dt>Пользователь</dt><dd>{actorText(selected.actor_user_id)}</dd>
              <dt>Время</dt><dd>{fmt(selected.created_at)}</dd>
              <dt>IP-адрес</dt><dd>{selected.ip ?? "—"}</dd>
              {Object.entries(auditDetails(selected)).map(([key, value]) => (
                <Fragment key={key}>
                  <dt>{auditFieldLabel(key)}</dt>
                  <dd>{detailValue(key, value)}</dd>
                </Fragment>
              ))}
            </dl>
            <div className="actions">
              <button type="button" onClick={() => setSelected(null)}>Закрыть</button>
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
