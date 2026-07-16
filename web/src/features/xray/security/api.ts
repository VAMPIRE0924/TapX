import { panelFetch } from '../../../app/runtime-path';

type PanelResponse<T> = {
  success?: boolean;
  msg?: string;
  error?: string;
  obj?: T;
};

export async function getPanelObject<T>(url: string): Promise<T> {
  const response = await panelFetch(url, {
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  return unwrapRequiredObject<T>(response, await response.json());
}

export async function postPanelObject<T>(url: string, body: Record<string, unknown>): Promise<T> {
  const response = await panelPost(url, body);
  return unwrapRequiredObject<T>(response, await response.json());
}

export async function postPanelResult<T>(url: string, body: Record<string, unknown>): Promise<T> {
  const response = await panelPost(url, body);
  const payload = await response.json() as PanelResponse<T> | T;
  if (!response.ok) throw new Error(responseError(payload, response.status));
  if (payload && typeof payload === 'object' && 'obj' in payload) {
    const boxed = payload as PanelResponse<T>;
    if (boxed.success === false) throw new Error(boxed.msg || boxed.error || 'Request failed');
    if (boxed.obj !== undefined) return boxed.obj;
  }
  return payload as T;
}

async function panelPost(url: string, body: Record<string, unknown>): Promise<Response> {
  return panelFetch(url, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify(body),
  });
}

function unwrapRequiredObject<T>(response: Response, payload: unknown): T {
  const boxed = payload as PanelResponse<T>;
  if (!response.ok || boxed?.success === false || boxed?.obj === undefined) {
    throw new Error(responseError(payload, response.status));
  }
  return boxed.obj;
}

function responseError(payload: unknown, status: number): string {
  if (payload && typeof payload === 'object') {
    const boxed = payload as PanelResponse<unknown>;
    if (boxed.msg) return boxed.msg;
    if (boxed.error) return boxed.error;
  }
  return `HTTP ${status}`;
}
