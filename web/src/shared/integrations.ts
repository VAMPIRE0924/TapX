import { panelFetch } from '../app/runtime-path';

export interface WarpAccount {
  access_token: string;
  device_id: string;
  license_key: string;
  private_key: string;
  client_id?: string;
  update_interval_days?: number;
  last_updated_at?: number;
}

export interface WarpConfig {
  name?: string;
  model?: string;
  enabled?: boolean;
  config?: {
    client_id?: string;
    interface?: { addresses?: { v4?: string; v6?: string } };
    peers?: Array<{ public_key?: string; endpoint?: { host?: string } }>;
  };
  account?: {
    account_type?: string;
    role?: string;
    premium_data?: number;
    quota?: number;
    usage?: number;
  };
}

export interface WarpRegistrationResult {
  data: WarpAccount;
  config: WarpConfig;
}

export interface NordAccount {
  token?: string;
  private_key: string;
}

export interface NordCountry {
  id: number;
  name: string;
  code: string;
}

export interface NordCity {
  id: number;
  name: string;
}

export interface NordServer {
  id: number;
  name: string;
  hostname: string;
  station: string;
  load: number;
  technologies?: Array<{ id: number; metadata?: Array<{ name: string; value: string }> }>;
  location_ids?: number[];
  cityId?: number | null;
  cityName?: string;
}

export interface NordServerResult {
  servers?: NordServer[];
  locations?: Array<{
    id: number;
    country?: { city?: NordCity };
  }>;
}

export const integrations = {
  warp: {
    data: (managedNodeID = 'local') => integrationRequest<WarpAccount | null>(integrationPath('warp', 'data', managedNodeID)),
    register: (privateKey: string, publicKey: string, managedNodeID = 'local') => integrationRequest<WarpRegistrationResult>(integrationPath('warp', 'register', managedNodeID), {
      method: 'POST',
      body: JSON.stringify({ privateKey, publicKey }),
    }),
    rotate: (privateKey: string, publicKey: string, managedNodeID = 'local') => integrationRequest<WarpRegistrationResult>(integrationPath('warp', 'rotate', managedNodeID), {
      method: 'POST',
      body: JSON.stringify({ privateKey, publicKey }),
    }),
    config: (managedNodeID = 'local') => integrationRequest<WarpConfig>(integrationPath('warp', 'config', managedNodeID)),
    license: (license: string, managedNodeID = 'local') => integrationRequest<WarpAccount>(integrationPath('warp', 'license', managedNodeID), {
      method: 'POST',
      body: JSON.stringify({ license }),
    }),
    interval: (days: number, managedNodeID = 'local') => integrationRequest<WarpAccount>(integrationPath('warp', 'interval', managedNodeID), {
      method: 'POST',
      body: JSON.stringify({ days }),
    }),
    remove: (managedNodeID = 'local') => integrationRequest<null>(integrationPath('warp', 'delete', managedNodeID), { method: 'DELETE' }),
  },
  nord: {
    data: (managedNodeID = 'local') => integrationRequest<NordAccount | null>(integrationPath('nord', 'data', managedNodeID)),
    countries: (managedNodeID = 'local') => integrationRequest<NordCountry[]>(integrationPath('nord', 'countries', managedNodeID)),
    servers: (countryId: number, managedNodeID = 'local') => integrationRequest<NordServerResult>(`${integrationPath('nord', 'servers', managedNodeID)}?countryId=${encodeURIComponent(countryId)}`),
    login: (token: string, managedNodeID = 'local') => integrationRequest<NordAccount>(integrationPath('nord', 'login', managedNodeID), {
      method: 'POST',
      body: JSON.stringify({ token }),
    }),
    privateKey: (privateKey: string, managedNodeID = 'local') => integrationRequest<NordAccount>(integrationPath('nord', 'private-key', managedNodeID), {
      method: 'POST',
      body: JSON.stringify({ privateKey }),
    }),
    remove: (managedNodeID = 'local') => integrationRequest<null>(integrationPath('nord', 'delete', managedNodeID), { method: 'DELETE' }),
  },
};

function integrationPath(provider: 'warp' | 'nord', action: string, managedNodeID: string): string {
  if (!managedNodeID || managedNodeID === 'local') return `/api/integrations/${provider}/${action}`;
  return `/api/nodes/${encodeURIComponent(managedNodeID)}/integrations/${provider}/${action}`;
}

async function integrationRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set('Accept', 'application/json');
  if (init.body != null) headers.set('Content-Type', 'application/json');
  const response = await panelFetch(path, { ...init, headers, credentials: 'same-origin' });
  const text = await response.text();
  let payload: { ok?: boolean; data?: T; error?: string; message?: string } = {};
  if (text) {
    try {
      payload = JSON.parse(text) as typeof payload;
    } catch {
      if (!response.ok) throw new Error(text);
    }
  }
  if (!response.ok || payload.ok === false) {
    throw new Error(payload.error || payload.message || `${response.status} ${response.statusText}`);
  }
  return payload.data as T;
}
