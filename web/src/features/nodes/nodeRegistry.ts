import { panelFetch } from '../../app/runtime-path';
import type { RuntimeConfig } from '../../shared/api';
import { responseError } from '../../shared/http-error';

export type NodeStatus = 'online' | 'offline' | 'checking';
export type TLSVerifyMode = 'system' | 'pin' | 'skip';

export interface ManagedNode {
  ID: string;
  Enabled: boolean;
  Name: string;
  Remark?: string;
  Protocol: 'https' | 'http';
  Host: string;
  Port: number;
  BasePath: string;
  AllowPrivateAddress: boolean;
  TLSVerify: TLSVerifyMode;
  CertificateSHA256?: string;
  APIToken?: string;
  APITokenConfigured?: boolean;
  Status: NodeStatus;
  CPU?: number;
  Memory?: number;
  PanelVersion?: string;
  TapXVersion?: string;
  EmbeddedXrayVersion?: string;
  ExternalXrayVersion?: string;
  Uptime?: string;
  Latency?: number;
  LastSeen?: string;
  ObjectCounts?: {
    devices: number;
    listeners: number;
    users: number;
    connectors: number;
    links: number;
  };
}

export interface ManagedNodeMTLS {
  Enabled: boolean;
  CertificateFile?: string;
  PrivateKeyFile?: string;
  CAFile?: string;
}

export const nodeRegistryChangedEvent = 'tapx-node-registry-change';

let cachedNodes: ManagedNode[] = [];

export function readManagedNodes(): ManagedNode[] {
  return cachedNodes;
}

export async function loadManagedNodes(): Promise<ManagedNode[]> {
  const response = await request('/api/nodes');
  const payload = await response.json() as { nodes?: ManagedNode[] };
  return publishNodes(Array.isArray(payload.nodes) ? payload.nodes : []);
}

export async function saveManagedNode(node: ManagedNode): Promise<ManagedNode> {
  const response = await request(`/api/nodes/${encodeURIComponent(node.ID)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(node),
  });
  const payload = await response.json() as { node: ManagedNode };
  publishNodes([...cachedNodes.filter((item) => item.ID !== payload.node.ID), payload.node]);
  return payload.node;
}

export async function removeManagedNode(id: string): Promise<void> {
  await request(`/api/nodes/${encodeURIComponent(id)}`, { method: 'DELETE' });
  publishNodes(cachedNodes.filter((node) => node.ID !== id));
}

export async function testManagedNode(id: string): Promise<ManagedNode> {
  const response = await request(`/api/nodes/${encodeURIComponent(id)}/test`, { method: 'POST' });
  const payload = await response.json() as { node: ManagedNode };
  publishNodes(cachedNodes.map((node) => node.ID === id ? payload.node : node));
  return payload.node;
}

export async function testManagedNodeDraft(node: ManagedNode): Promise<ManagedNode> {
  const response = await request('/api/nodes/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(node),
  });
  const payload = await response.json() as { node: ManagedNode };
  return payload.node;
}

export async function updateManagedNode(id: string): Promise<void> {
  await request(`/api/nodes/${encodeURIComponent(id)}/update`, { method: 'POST' });
}

export async function getManagedNodeConfig(id: string): Promise<RuntimeConfig> {
  const response = await request(`/api/nodes/${encodeURIComponent(id)}/config`);
  return unwrapConfig(await response.json());
}

export async function saveManagedNodeConfig(id: string, config: RuntimeConfig): Promise<RuntimeConfig> {
  const response = await request(`/api/nodes/${encodeURIComponent(id)}/config`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  });
  return unwrapConfig(await response.json(), config);
}

export async function applyManagedNodeConfig(id: string): Promise<void> {
  await request(`/api/nodes/${encodeURIComponent(id)}/runtime/apply`, { method: 'POST' });
}

export async function getManagedNodeMTLS(): Promise<ManagedNodeMTLS> {
  const response = await request('/api/nodes/mtls');
  const payload = await response.json() as { mtls?: ManagedNodeMTLS };
  return payload.mtls || { Enabled: false };
}

export async function saveManagedNodeMTLS(mtls: ManagedNodeMTLS): Promise<ManagedNodeMTLS> {
  const response = await request('/api/nodes/mtls', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(mtls),
  });
  const payload = await response.json() as { mtls?: ManagedNodeMTLS };
  return payload.mtls || mtls;
}

function publishNodes(nodes: ManagedNode[]): ManagedNode[] {
  cachedNodes = [...nodes].sort((left, right) => left.Name.localeCompare(right.Name));
  if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent(nodeRegistryChangedEvent));
  return cachedNodes;
}

async function request(path: string, init?: RequestInit): Promise<Response> {
  const response = await panelFetch(path, {
    ...init,
    cache: init?.cache || 'no-store',
    credentials: 'same-origin',
    headers: { Accept: 'application/json', ...init?.headers },
  });
  if (!response.ok) throw await responseError(response, 'managed node');
  return response;
}

function unwrapConfig(payload: unknown, fallback: RuntimeConfig = {}): RuntimeConfig {
  if (payload && typeof payload === 'object' && 'config' in payload) {
    return ((payload as { config?: RuntimeConfig }).config || fallback);
  }
  return payload && typeof payload === 'object' ? payload as RuntimeConfig : fallback;
}
