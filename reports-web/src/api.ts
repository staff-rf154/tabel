export type ApiResult<T = unknown> = { ok: boolean; status: number; body: T | null };

const TOKEN_KEY = "tabel_reports_access";
const REFRESH_KEY = "tabel_reports_refresh";

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? "";
}

export function setTokens(access: string, refresh: string): void {
  localStorage.setItem(TOKEN_KEY, access);
  localStorage.setItem(REFRESH_KEY, refresh);
}

export function clearTokens(): void {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(REFRESH_KEY);
}

export function errorText(body: unknown, fallback: string): string {
  if (body && typeof body === "object" && "error" in body) {
    const value = (body as { error?: unknown }).error;
    if (typeof value === "string") return value;
  }
  return fallback;
}

export async function api<T = unknown>(path: string, init?: RequestInit): Promise<ApiResult<T>> {
  const headers = new Headers(init?.headers ?? {});
  if (init?.body !== undefined && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  const token = getToken();
  if (token && !headers.has("Authorization")) headers.set("Authorization", `Bearer ${token}`);
  try {
    const res = await fetch(`/api${path}`, { ...init, headers });
    const text = await res.text();
    let body: unknown = null;
    if (text) {
      try {
        body = JSON.parse(text);
      } catch {
        body = text;
      }
    }
    return { ok: res.ok, status: res.status, body: body as T };
  } catch {
    return { ok: false, status: 0, body: null };
  }
}

export async function downloadFile(path: string, filename: string): Promise<string> {
  const res = await fetch(`/api${path}`, { headers: { Authorization: `Bearer ${getToken()}` } });
  if (!res.ok) return `не удалось выгрузить файл (HTTP ${res.status})`;
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
  return "";
}

export async function login(loginValue: string, password: string): Promise<ApiResult<{ access_token: string; refresh_token: string; role?: string }>> {
  const res = await api<{ access_token: string; refresh_token: string }>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ login: loginValue, password }),
  });
  if (res.ok && res.body) setTokens(res.body.access_token, res.body.refresh_token);
  return res;
}
