export type AnyRecord = Record<string, any>;

export interface RuntimeConfig {
  Devices?: AnyRecord[];
  Listeners?: AnyRecord[];
  Connectors?: AnyRecord[];
  Clients?: AnyRecord[];
  Routes?: AnyRecord[];
  VKeys?: AnyRecord[];
  Addresses?: AnyRecord[];
  XrayProfiles?: AnyRecord[];
  Settings?: AnyRecord[];
}

export interface AuthSession {
  authEnabled: boolean;
  authenticated: boolean;
  username?: string;
}

function basePath() {
  const path = window.location.pathname.replace(/\/+$/, '');
  return path === '' || path === '/' ? '' : path;
}

export function apiURL(path: string) {
  return `${basePath()}${path.startsWith('/') ? path : `/${path}`}`;
}

export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers || {});
  if (init.body && !headers.has('content-type') && !(init.body instanceof FormData)) {
    headers.set('content-type', 'application/json');
  }
  const response = await fetch(apiURL(path), {
    credentials: 'same-origin',
    ...init,
    headers,
  });
  const text = await response.text();
  const payload = text ? JSON.parse(text) : null;
  if (!response.ok) {
    throw new Error(payload?.error || payload?.message || response.statusText);
  }
  return payload as T;
}

export async function loadAuthSession() {
  return request<AuthSession>('/api/auth/session');
}

export async function login(username: string, password: string) {
  return request<AuthSession>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  });
}

export async function logout() {
  return request<{ ok: boolean }>('/api/auth/logout', { method: 'POST' });
}

export async function loadConfig() {
  const payload = await request<{ config: RuntimeConfig }>('/api/config');
  return payload.config || {};
}

export async function saveConfig(config: RuntimeConfig) {
  return request<{ ok: boolean; config: RuntimeConfig }>('/api/config', {
    method: 'PUT',
    body: JSON.stringify(config),
  });
}

export async function validateConfig(config: RuntimeConfig, mode: 'save' | 'apply' = 'save') {
  return request<{ ok: boolean; mode: string }>(`/api/config/validate?mode=${mode}`, {
    method: 'POST',
    body: JSON.stringify(config),
  });
}

export async function upsertObject(kind: string, id: string, value: AnyRecord) {
  return request<{ ok: boolean; config: RuntimeConfig }>(`/api/objects/${kind}/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(value),
  });
}

export async function deleteObject(kind: string, id: string) {
  return request<{ ok: boolean; config: RuntimeConfig }>(`/api/objects/${kind}/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
}

export async function loadDashboard() {
  return request<AnyRecord>('/api/dashboard');
}

export async function loadRuntimeState() {
  const payload = await request<{ state: AnyRecord }>('/api/runtime/state');
  return payload.state || {};
}

export async function applyRuntime() {
  return request<{ ok: boolean; state: AnyRecord }>('/api/runtime/apply', { method: 'POST' });
}

export async function stopRuntime() {
  return request<{ ok: boolean; state: AnyRecord }>('/api/runtime/stop', { method: 'POST' });
}

export async function loadLogs() {
  return request<{ events: AnyRecord[] }>('/api/logs');
}

export async function clearLogs() {
  return request<{ ok: boolean }>('/api/logs', { method: 'DELETE' });
}

export async function resetClientTraffic(id: string) {
  return request<{ ok: boolean }>(`/api/clients/${encodeURIComponent(id)}/traffic/reset`, { method: 'POST' });
}

export async function loadClientShare(id: string) {
  return request<{ share: AnyRecord }>(`/api/share/clients/${encodeURIComponent(id)}`);
}

export async function loadDiagnostics() {
  return request<AnyRecord>('/api/diagnostics');
}

export async function loadXrayBinary(path?: string) {
  const qs = path ? `?path=${encodeURIComponent(path)}` : '';
  return request<{ binary: AnyRecord }>(`/api/xray/external/status${qs}`);
}

export async function downloadXrayBinary(url: string, path?: string) {
  return request<{ ok: boolean; binary: AnyRecord }>('/api/xray/external/download', {
    method: 'POST',
    body: JSON.stringify({ url, path }),
  });
}

export async function uploadXrayBinary(file: File, path?: string) {
  const form = new FormData();
  form.append('file', file);
  const qs = path ? `?path=${encodeURIComponent(path)}` : '';
  return request<{ ok: boolean; binary: AnyRecord }>(`/api/xray/external/upload${qs}`, {
    method: 'POST',
    body: form,
  });
}

export function deepClone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value ?? null));
}
