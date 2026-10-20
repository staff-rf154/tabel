export type ApiResult<T = unknown> = { ok: boolean; status: number; body: T | null };

const TOKEN_KEY = "tabel_operator_access";
const REFRESH_KEY = "tabel_operator_refresh";

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
  if (init?.body !== undefined && !(init.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
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

export async function login(loginValue: string, password: string): Promise<ApiResult<{ access_token: string; refresh_token: string }>> {
  const res = await api<{ access_token: string; refresh_token: string }>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ login: loginValue, password }),
  });
  if (res.ok && res.body) setTokens(res.body.access_token, res.body.refresh_token);
  return res;
}
