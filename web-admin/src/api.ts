export type ApiResult<T = unknown> = {
  ok: boolean;
  status: number;
  body: T | null;
  error?: string;
};

const TOKEN_KEY = "tabel_admin_access";
const REFRESH_KEY = "tabel_admin_refresh";

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? "";
}

export function getRefreshToken(): string {
  return localStorage.getItem(REFRESH_KEY) ?? "";
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
    if (typeof value === "string") {
      return value;
    }
  }
  return fallback;
}

export async function api<T = unknown>(path: string, init?: RequestInit): Promise<ApiResult<T>> {
  const headers = new Headers(init?.headers ?? {});
  const hasBody = init?.body !== undefined && !(init.body instanceof FormData);
  if (hasBody && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const token = getToken();
  if (token && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${token}`);
  }
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
    return { ok: false, status: 0, body: null, error: "сетевая ошибка при обращении к серверу" };
  }
}

export async function login(loginValue: string, password: string): Promise<ApiResult<{ access_token: string; refresh_token: string }>> {
  const res = await api<{ access_token: string; refresh_token: string }>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ login: loginValue, password }),
  });
  if (res.ok && res.body) {
    setTokens(res.body.access_token, res.body.refresh_token);
  }
  return res;
}

export async function refresh(): Promise<boolean> {
  const value = getRefreshToken();
  if (!value) {
    return false;
  }
  const res = await api<{ access_token: string; refresh_token: string }>("/auth/refresh", {
    method: "POST",
    body: JSON.stringify({ refresh_token: value }),
  });
  if (res.ok && res.body) {
    setTokens(res.body.access_token, res.body.refresh_token);
    return true;
  }
  return false;
}

export async function downloadFile(path: string, fallbackName: string): Promise<{ ok: boolean; error?: string }> {
  const headers = new Headers();
  const token = getToken();
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  try {
    const res = await fetch(`/api${path}`, { headers });
    if (!res.ok) {
      const text = await res.text();
      let body: unknown = null;
      if (text) {
        try {
          body = JSON.parse(text);
        } catch {
          body = text;
        }
      }
      return { ok: false, error: errorText(body, `не удалось скачать файл (HTTP ${res.status})`) };
    }
    const disposition = res.headers.get("Content-Disposition") ?? "";
    const match = /filename="?([^";]+)"?/i.exec(disposition);
    const filename = match?.[1] ?? fallbackName;
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
    return { ok: true };
  } catch {
    return { ok: false, error: "сетевая ошибка при скачивании файла" };
  }
}
