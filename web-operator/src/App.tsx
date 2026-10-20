import { useCallback, useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import { api, clearTokens, errorText, getToken, login } from "./api";

type Verify = {
  valid: boolean;
  token_status: string;
  signature_valid: boolean;
  employee_id: string | null;
  employee_number: string | null;
  full_name: string | null;
  qr_data_mismatch: boolean;
  reason?: string | null;
};

function eventLabel(value: string | null | undefined): string {
  if (value === "check_in") return "Приход";
  if (value === "check_out") return "Уход";
  return value || "авто";
}

function tokenStatusLabel(value: string | null | undefined): string {
  switch (value) {
    case "active": return "Активен";
    case "revoked": return "Отозван";
    case "expired": return "Истёк";
    case "not_found": return "Не найден";
    default: return value || "—";
  }
}

type Mark = {
  id: string;
  employee_id: string;
  event_type: string;
  timestamp_server: string;
  overtime: boolean;
  outside_office: boolean;
  auto_closed_previous: boolean;
};

type LogEntry = { at: string; text: string; ok: boolean };

type BarcodeResult = { rawValue: string };
type BarcodeDetectorType = {
  detect: (source: CanvasImageSource) => Promise<BarcodeResult[]>;
};

export default function App() {
  const [profile, setProfile] = useState<{ login: string; role: string } | null>(null);
  const [checked, setChecked] = useState(false);

  const loadProfile = useCallback(async () => {
    if (!getToken()) {
      setProfile(null);
      setChecked(true);
      return;
    }
    const res = await api<{ login: string; role: string }>("/me");
    if (res.ok && res.body) setProfile(res.body);
    else clearTokens();
    setChecked(true);
  }, []);

  useEffect(() => {
    void loadProfile();
  }, [loadProfile]);

  if (!checked) {
    return <main className="app"><h1>Табель — оператор</h1><p>Загрузка…</p></main>;
  }

  if (!profile) {
    return <Login onLogin={async () => { await loadProfile(); }} />;
  }

  return (
    <main className="app wide">
      <header className="topline">
        <h1>Табель — оператор</h1>
        <div>
          <span className="who">{profile.login}</span>
          <button type="button" onClick={() => { void api("/auth/logout", { method: "POST" }); clearTokens(); setProfile(null); }}>
            Выйти
          </button>
        </div>
      </header>
      <Scanner />
    </main>
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
    <main className="app">
      <h1>Табель — вход оператора</h1>
      <form onSubmit={submit}>
        <label>Логин<input value={loginValue} onChange={(e) => setLoginValue(e.target.value)} autoComplete="username" required /></label>
        <label>Пароль<input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" required /></label>
        {error && <p className="error">{error}</p>}
        <button type="submit" disabled={busy}>{busy ? "Вход…" : "Войти"}</button>
      </form>
    </main>
  );
}

function Scanner() {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const detectorRef = useRef<BarcodeDetectorType | null>(null);
  const timerRef = useRef<number | null>(null);
  const lastRef = useRef<string>("");
  const manualRef = useRef<HTMLInputElement | null>(null);
  const busyRef = useRef(false);
  const processRef = useRef<(value: string) => Promise<void>>(async () => {});
  const scanBufferRef = useRef("");
  const lastKeyAtRef = useRef(0);

  const [scanning, setScanning] = useState(false);
  const [manual, setManual] = useState("");
  const [token, setToken] = useState("");
  const [verify, setVerify] = useState<Verify | null>(null);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [log, setLog] = useState<LogEntry[]>([]);
  const [busy, setBusy] = useState(false);

  function addLog(text: string, ok: boolean) {
    setLog((prev) => [{ at: new Date().toISOString(), text, ok }, ...prev].slice(0, 30));
  }

  const processScan = useCallback(async (value: string) => {
    const clean = value.trim();
    if (!clean || busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setMessage(null);
    setManual("");
    const verifyRes = await api<Verify>(`/qr/${encodeURIComponent(clean)}`);
    if (!verifyRes.ok || !verifyRes.body) {
      busyRef.current = false;
      setBusy(false);
      setVerify(null);
      setMessage({ ok: false, text: errorText(verifyRes.body, "QR-токен не распознан") });
      addLog("Проверка QR: ошибка", false);
      manualRef.current?.focus();
      return;
    }
    const verification = verifyRes.body;
    setToken(clean);
    setVerify(verification);
    if (!verification.valid) {
      busyRef.current = false;
      setBusy(false);
      setMessage({
        ok: false,
        text: `QR недействителен: ${tokenStatusLabel(verification.token_status)}${verification.reason ? ` (${verification.reason})` : ""}`,
      });
      addLog(`Проверка QR: ${tokenStatusLabel(verification.token_status)}`, false);
      manualRef.current?.focus();
      return;
    }
    await createMark(clean, verification, "");
    busyRef.current = false;
    manualRef.current?.focus();
  }, []);

  useEffect(() => {
    processRef.current = processScan;
  }, [processScan]);

  useEffect(() => {
    manualRef.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      if (target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable)) {
        return;
      }
      if (event.key === "Enter") {
        const value = scanBufferRef.current.trim();
        scanBufferRef.current = "";
        if (value) {
          event.preventDefault();
          void processRef.current(value);
        }
        return;
      }
      if (event.key.length === 1) {
        const now = Date.now();
        if (now - lastKeyAtRef.current > 120) scanBufferRef.current = "";
        scanBufferRef.current += event.key;
        lastKeyAtRef.current = now;
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  async function startCamera() {
    setMessage(null);
    lastRef.current = "";
    try {
      const supported = "BarcodeDetector" in window;
      if (!supported) {
        setMessage({ ok: false, text: "Браузер не поддерживает распознавание QR — введите токен вручную" });
        return;
      }
      const Detector = (window as unknown as { BarcodeDetector: new (opts: { formats: string[] }) => BarcodeDetectorType }).BarcodeDetector;
      detectorRef.current = new Detector({ formats: ["qr_code"] });
      const stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: "environment" } });
      streamRef.current = stream;
      if (videoRef.current) {
        videoRef.current.srcObject = stream;
        await videoRef.current.play();
      }
      setScanning(true);
      const tick = async () => {
        if (!videoRef.current || !detectorRef.current) return;
        try {
          const results = await detectorRef.current.detect(videoRef.current);
          if (results.length > 0 && results[0].rawValue && results[0].rawValue !== lastRef.current) {
            lastRef.current = results[0].rawValue;
            await processRef.current(results[0].rawValue);
            stopCamera();
            return;
          }
        } catch {
          /* игнорируем ошибки отдельных кадров */
        }
        timerRef.current = window.setTimeout(tick, 350);
      };
      timerRef.current = window.setTimeout(tick, 350);
    } catch {
      setMessage({ ok: false, text: "Нет доступа к камере" });
    }
  }

  function stopCamera() {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    streamRef.current?.getTracks().forEach((track) => track.stop());
    streamRef.current = null;
    setScanning(false);
  }

  useEffect(() => () => stopCamera(), []);

  async function createMark(scanToken: string, currentVerify: Verify | null, eventType: "check_in" | "check_out" | "") {
    const key = crypto.randomUUID();
    let lat: number | undefined;
    let lon: number | undefined;
    try {
      const position = await new Promise<GeolocationPosition>((resolve, reject) =>
        navigator.geolocation.getCurrentPosition(resolve, reject, { timeout: 5000 }),
      );
      lat = position.coords.latitude;
      lon = position.coords.longitude;
    } catch {
      /* геолокация необязательна */
    }
    setBusy(true);
    const res = await api<Mark>("/mark", {
      method: "POST",
      headers: { "Idempotency-Key": key },
      body: JSON.stringify({
        qr_token: scanToken,
        client_type: "web",
        event_type: eventType || undefined,
        lat,
        lon,
      }),
    });
    setBusy(false);
    if (!res.ok || !res.body) {
      const text = errorText(res.body, "не удалось создать отметку");
      setMessage({ ok: false, text });
      addLog(`Отметка ${eventType || "авто"}: ${text}`, false);
      return;
    }
    const mark = res.body;
    const flags = [mark.overtime ? "переработка" : "", mark.outside_office ? "вне офиса" : "", mark.auto_closed_previous ? "авто-закрытие" : ""]
      .filter(Boolean)
      .join(", ");
    const employee = currentVerify?.full_name || currentVerify?.employee_number || "сотрудник";
    setMessage({ ok: true, text: `Отметка «${eventLabel(mark.event_type)}» создана${flags ? ` (${flags})` : ""}` });
    addLog(`Отметка «${eventLabel(mark.event_type)}» для ${employee}`, true);
    setVerify(null);
    setToken("");
    setManual("");
    manualRef.current?.focus();
  }

  return (
    <div className="scanner">
      <section className="card">
        <h2>Сканирование QR</h2>
        <video ref={videoRef} className={scanning ? "video" : "video hidden"} playsInline muted />
        <div className="actions">
          {!scanning && <button type="button" onClick={() => void startCamera()}>Включить камеру</button>}
          {scanning && <button type="button" onClick={stopCamera}>Остановить</button>}
        </div>
        <form
          className="manual"
          onSubmit={(e) => {
            e.preventDefault();
            void processScan(manual);
          }}
        >
          <label>USB-сканер или ручной ввод — отправка автоматическая
            <input
              ref={manualRef}
              value={manual}
              onChange={(e) => setManual(e.target.value)}
              placeholder="Отсканируйте QR-код"
              className="mono"
              autoFocus
            />
          </label>
          <button type="submit" disabled={busy}>Отправить</button>
        </form>
        {message && <p className={message.ok ? "ok" : "error"}>{message.text}</p>}
      </section>

      {verify && (
        <section className="card">
          <h2>Сотрудник</h2>
          <dl className="details">
            <dt>ФИО</dt><dd>{verify.full_name ?? "—"}</dd>
            <dt>Табельный номер</dt><dd>{verify.employee_number ?? "—"}</dd>
            <dt>Статус токена</dt><dd>{tokenStatusLabel(verify.token_status)}</dd>
            {verify.reason && <><dt>Причина</dt><dd>{verify.reason}</dd></>}
            <dt>Подпись</dt><dd>{verify.signature_valid ? "верна" : "неверна"}</dd>
            <dt>Расхождение полей</dt><dd>{verify.qr_data_mismatch ? "да" : "нет"}</dd>
          </dl>
          {verify.valid ? (
            <div className="actions">
              <button type="button" disabled={busy} onClick={() => void createMark(token, verify, "")}>Определить автоматически</button>
              <button type="button" disabled={busy} onClick={() => void createMark(token, verify, "check_in")}>Приход</button>
              <button type="button" disabled={busy} onClick={() => void createMark(token, verify, "check_out")}>Уход</button>
            </div>
          ) : (
            <p className="error">Отметка недоступна: QR недействителен.</p>
          )}
        </section>
      )}

      <section className="card">
        <h2>Журнал сеанса</h2>
        {log.length === 0 && <p>Пока пусто.</p>}
        <ul className="log">
          {log.map((entry, index) => (
            <li key={`${entry.at}-${index}`} className={entry.ok ? "ok" : "error"}>
              <span className="mono">{new Date(entry.at).toLocaleTimeString("ru-RU")}</span> {entry.text}
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
