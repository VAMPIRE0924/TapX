import { LocalizedError } from './localized-error';
import { responseError } from './http-error';
import { panelFetch as fetch } from '../app/runtime-path';

export interface DashboardReport {
  generatedAt?: string;
  runtime?: {
    running?: boolean;
    generation?: number;
    udpPipes?: unknown[];
    tcpPipes?: unknown[];
    xrayPipes?: unknown[];
    xrayRuntimes?: Array<{
      running?: boolean;
      runtime?: string;
      adapter?: string;
      endpointCount?: number;
      lastError?: string;
    }>;
  };
  stats?: {
    totals?: DashboardStatsCounters;
    byTransport?: DashboardStatsBucket[];
    byDevice?: DashboardStatsBucket[];
    byRoute?: DashboardStatsBucket[];
    byClient?: DashboardStatsBucket[];
    byEndpoint?: DashboardStatsBucket[];
    clients?: ClientQuotaState[];
  };
  rates?: {
    rxBytesPerSecond?: number;
    txBytesPerSecond?: number;
    rxPacketsPerSecond?: number;
    txPacketsPerSecond?: number;
  };
  objectCounts?: Record<string, number>;
  process?: {
    heapAlloc?: number;
    heapSys?: number;
    heapObjects?: number;
    numGC?: number;
    lastGCPauseNs?: number;
    goroutines?: number;
    uptimeSecond?: number;
  };
  system?: {
    cpuPercent?: number;
    cpuCores?: number;
    memoryUsed?: number;
    memoryTotal?: number;
    swapUsed?: number;
    swapTotal?: number;
    storageUsed?: number;
    storageTotal?: number;
    runningPipes?: number;
    dropCount?: number;
    tcpConnections?: number;
    udpConnections?: number;
	 diskReadBytesPerSecond?: number;
	 diskWriteBytesPerSecond?: number;
	 load1?: number;
	 load5?: number;
	 load15?: number;
  };
  recentLogs?: Array<{ ts?: string; at?: string; level?: string; message?: string }>;
  history?: Array<{
    at: number;
    cpu: number;
    memory: number;
    embeddedXray: number;
    externalXray: number;
    tapx: number;
    rx: number;
    tx: number;
	 rxPackets?: number;
	 txPackets?: number;
	 swap?: number;
	 diskRead?: number;
	 diskWrite?: number;
	 diskUsage?: number;
	 tcpConnections?: number;
	 udpConnections?: number;
	 online?: number;
	 load1?: number;
	 load5?: number;
	 load15?: number;
    drops: number;
	 tapxHeap?: number;
	 tapxSys?: number;
	 tapxObjects?: number;
	 tapxGC?: number;
	 tapxGCPause?: number;
	 tapxObservatory?: number;
	 embeddedHeap?: number;
	 embeddedSys?: number;
	 embeddedObjects?: number;
	 embeddedGC?: number;
	 embeddedGCPause?: number;
	 embeddedObservatory?: number;
	 externalObservatory?: number;
  }>;
}

export interface DashboardStatsCounters {
  rxPackets?: number;
  txPackets?: number;
  rxBytes?: number;
  txBytes?: number;
  dropsGuard?: number;
  dropsIO?: number;
}

export interface DashboardStatsBucket {
  id: string;
  name?: string;
  kind?: string;
  endpoint?: string;
  pipes?: number;
  counters?: DashboardStatsCounters;
}

export interface PanelLogEvent {
  seq: number;
  time: string;
  level: string;
  action: string;
  message: string;
}

export interface XrayBinaryStatus {
  path: string;
  exists: boolean;
  isRegular: boolean;
  executable: boolean;
  size: number;
  mode?: string;
  modifiedAt?: string;
  version?: string;
  error?: string;
}

export interface DiagnosticReport {
  product?: string;
  version?: string;
  components?: {
    panel?: string;
    tapx?: string;
    embeddedXray?: string;
  };
  process?: {
    goos?: string;
    goarch?: string;
    goVersion?: string;
  };
  fastpath?: {
    abi?: number;
  };
}

export type UpdateComponent = 'panel' | 'tapx' | 'embedded-xray' | 'external-xray';

export interface ComponentUpdateVersion {
  version: string;
  current: boolean;
  latest: boolean;
  installable: boolean;
}

export interface ComponentUpdateCatalog {
  component: UpdateComponent;
  currentVersion: string;
  channel: 'stable' | 'development';
  source: string;
  delivery: string;
  platform: string;
  versions: ComponentUpdateVersion[];
  relatedVersions?: Record<string, string>;
  installReady: boolean;
  message?: string;
}

export interface AuthSession {
  authEnabled: boolean;
  authenticated: boolean;
  username?: string;
  twoFactorEnabled?: boolean;
}

export interface PanelAPIToken {
  id: string;
  name: string;
  prefix: string;
  createdAt: string;
  expiresAt?: string;
}

export interface PanelSecurityStatus {
  twoFactorEnabled: boolean;
  apiTokens: PanelAPIToken[];
}

export interface TOTPSetup {
  secret: string;
  uri: string;
}

export interface StatsCounters {
  rxPackets?: number;
  txPackets?: number;
  rxBytes?: number;
  txBytes?: number;
  dropsGuard?: number;
  dropsIO?: number;
}

export interface StatsBucket {
  id: string;
  name?: string;
  kind?: string;
  endpoint?: string;
  pipes?: number;
  counters?: StatsCounters;
}

export interface StatsReport {
  generatedAt?: string;
  totals?: StatsCounters;
  byEndpoint?: StatsBucket[];
  byClient?: StatsBucket[];
  clients?: ClientQuotaState[];
}

export interface ClientQuotaState {
  id: string;
  name?: string;
  email?: string;
  enabled?: boolean;
  expiresAt?: number;
  expired?: boolean;
  trafficCap?: number;
  trafficResetAt?: number;
  usedBytes?: number;
  remainingBytes?: number;
  overQuota?: boolean;
  activePipes?: number;
  counters?: StatsCounters;
}

export type DeviceType = 'tun' | 'tap';
export type Transport = 'udp' | 'tcp' | 'xray';

export interface TapxBridgeConfig {
  Enabled?: boolean;
  Name?: string;
  IfName?: string;
  MTU?: number;
}

export interface TapxDeviceRoute {
  Enabled?: boolean;
  Destination?: string;
  Gateway?: string;
  Source?: string;
  IfName?: string;
  Metric?: number;
  Table?: string;
}

export interface TapxDNSConfig {
  Enabled?: boolean;
  Nameservers?: string[];
  SearchDomains?: string[];
  Options?: string[];
  OutputPath?: string;
}

export interface TapxBinding {
  VKeyID?: string;
  ClientID?: string;
  RouteID?: string;
  DeviceID?: string;
  ConnectorID?: string;
  AddressID?: string;
}

export interface TapxDevice {
  ID: string;
  Enabled?: boolean;
  Name?: string;
  Type?: DeviceType;
  IfName?: string;
  MTU?: number;
  MSSClamp?: number;
  LinkAutoOptimize?: boolean;
  AddressConfigEnabled?: boolean;
  AddressAssignMode?: 'auto' | 'manual';
  IPv4CIDR?: string;
  IPv6CIDR?: string;
  Gateway?: string;
  Bridge?: TapxBridgeConfig;
  DNS?: TapxDNSConfig;
  DNSSearch?: string[];
  Routes?: TapxDeviceRoute[];
  AllowDefaultRoute?: boolean;
  BridgeEnabled?: boolean;
  BridgeName?: string;
  BridgeMember?: string;
  Source?: 'manual' | 'listener-auto' | 'connector-auto';
  LinkedListenerIDs?: string[];
  LinkedListenerNames?: string[];
  LinkedConnectorIDs?: string[];
  LinkedConnectorNames?: string[];
  UpdatedAt?: number;
  Remark?: string;
}

export interface TapxEndpoint {
  ID: string;
  Enabled?: boolean;
  Name?: string;
  BindHost?: string;
  BindPort?: number;
  Remote?: string;
  Port?: number;
  Transport?: Transport;
  XrayProfileID?: string;
  RawUDP?: {
    KeepAliveSecond?: number;
    Workers?: number;
    QueueSize?: number;
    ZeroCopy?: boolean;
    ConnectTimeout?: number;
    IdleTimeout?: number;
    DTLS?: {
      Enabled?: boolean;
      CertFile?: string;
      KeyFile?: string;
      CAFile?: string;
      ServerName?: string;
      ALPN?: string[];
      MinVersion?: string;
      MaxVersion?: string;
      AllowInsecure?: boolean;
      MTU?: number;
      ReplayWindow?: number;
    };
  };
  RawTCP?: {
    LengthMode?: 'uint16' | 'uint32' | string;
    NoDelay?: boolean;
    KeepAliveSecond?: number;
    FastOpen?: boolean;
    Workers?: number;
    ConnectTimeout?: number;
    ReconnectSecond?: number;
    QueueSize?: number;
    ZeroCopy?: boolean;
    IdleTimeout?: number;
    TLS?: {
      Enabled?: boolean;
      CertFile?: string;
      KeyFile?: string;
      CAFile?: string;
      ServerName?: string;
      ALPN?: string[];
      MinVersion?: string;
      MaxVersion?: string;
      AllowInsecure?: boolean;
    };
  };
  Binding?: TapxBinding;
  ExpiresAt?: number;
  TrafficCap?: number;
  TrafficReset?: string;
  TrafficResetAt?: number;
  TrafficResetGeneration?: number;
  TrafficRXOffset?: number;
  TrafficTXOffset?: number;
  ShareAddressStrategy?: 'listen' | 'custom' | string;
  ShareAddress?: string;
  Remark?: string;
}

export interface TapxClient {
  ID: string;
  Enabled?: boolean;
  Name?: string;
  Email?: string;
  ListenerID?: string;
  ListenerIDs?: string[];
  CredentialType?: string;
  CredentialValue?: string;
  UUID?: string;
  Password?: string;
  Auth?: string;
  Flow?: string;
  /** @deprecated Legacy import compatibility only; the user editor strips this field. */
  Security?: string;
  /** @deprecated Legacy import compatibility only; reverse routing belongs to connectors. */
  ReverseTag?: string;
  VKey?: string;
  WireguardPrivateKey?: string;
  WireguardPublicKey?: string;
  WireguardPreSharedKey?: string;
  WireguardAllowedIPs?: string | string[];
  AllowedDeviceIDs?: string[];
  Binding?: TapxBinding;
  AddressID?: string;
  ExpiresAt?: number;
  TrafficCap?: number;
  UploadRateLimit?: number;
  DownloadRateLimit?: number;
  TrafficReset?: string;
  TrafficResetAt?: number;
  TrafficResetGeneration?: number;
  TrafficRXOffset?: number;
  TrafficTXOffset?: number;
  AllowedDevices?: string[];
  AllowedIPs?: string[];
  AllowedMACs?: string[];
  Remark?: string;
}

export interface TapxRoute {
  ID: string;
  Enabled?: boolean;
  Priority?: number;
  Action?: 'bind-device' | 'allow' | 'drop';
  VKeyID?: string;
  ListenerID?: string;
  DeviceID?: string;
  ConnectorID?: string;
  ClientID?: string;
  AddressID?: string;
}

export interface TapxVKey {
  ID: string;
  Enabled?: boolean;
  Name?: string;
  Value?: string;
  Remark?: string;
}

export interface TapxAddressLimit {
  ID: string;
  Enabled?: boolean;
  Name?: string;
  DeviceID?: string;
  ClientID?: string;
  MACs?: string[];
  IPv4CIDRs?: string[];
  IPv6CIDRs?: string[];
  IPv4Gateway?: string;
  IPv6Gateway?: string;
  DNS?: string[];
  Routes?: string[];
  AllowDefaultRoute?: boolean;
  Remark?: string;
}

export interface TapxXrayProfile {
  ID: string;
  Enabled?: boolean;
  Name?: string;
  Runtime?: 'embedded' | 'external' | string;
  InboundProtocol?: string;
  InboundSettingsJSON?: string;
  OutboundProtocol?: string;
  OutboundSettingsJSON?: string;
  SendThrough?: string;
  TargetStrategy?: string;
  Network?: string;
  Security?: string;
  StreamSettingsJSON?: string;
  SniffingJSON?: string;
  MuxJSON?: string;
  SockoptJSON?: string;
  FallbacksJSON?: string;
  RoutingJSON?: string;
  DNSJSON?: string;
  PolicyJSON?: string;
  AdvancedJSON?: string;
  Remark?: string;
}

export interface RuntimeConfig {
  Devices?: TapxDevice[];
  Listeners?: TapxEndpoint[];
  Connectors?: TapxEndpoint[];
  Clients?: TapxClient[];
  Routes?: TapxRoute[];
  VKeys?: TapxVKey[];
  Addresses?: TapxAddressLimit[];
  XrayProfiles?: TapxXrayProfile[];
  Settings?: unknown[];
}

export interface ClientShare {
  clientId: string;
  type: string;
  link: string;
  links?: string[];
  warnings?: string[];
  payload?: unknown;
}

export interface VlessEncryptionAuth {
  id?: string;
  label?: string;
  decryption: string;
  encryption: string;
}

export async function getDashboard(): Promise<DashboardReport> {
  const response = await fetch('/api/dashboard', {
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw new Error(`dashboard ${response.status}`);
  return response.json() as Promise<DashboardReport>;
}

export async function getSystemInterfaces(managedNodeID?: string): Promise<unknown> {
  if (isRemoteManagedNode(managedNodeID)) {
    const response = await fetch(`/api/nodes/${encodeURIComponent(managedNodeID)}/system/interfaces`, {
      headers: { Accept: 'application/json' },
      credentials: 'same-origin',
    });
    if (!response.ok) throw await responseError(response, 'managed node system interfaces');
    return response.json();
  }
  const urls = ['/api/server/interfaces', '/panel/api/server/interfaces'];
  for (const url of urls) {
    try {
      const response = await fetch(url, { headers: { Accept: 'application/json' }, credentials: 'same-origin' });
      if (!response.ok) continue;
      const payload = await response.json() as { obj?: unknown } | unknown;
      return payload && typeof payload === 'object' && 'obj' in payload ? (payload as { obj?: unknown }).obj : payload;
    } catch {
      // Continue to the compatibility endpoint.
    }
  }
  return [];
}

export async function getAuthSession(): Promise<AuthSession> {
  const response = await fetch('/api/auth/session', {
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'auth session');
  return response.json() as Promise<AuthSession>;
}

export async function loginPanel(username: string, password: string, twoFactorCode = ''): Promise<void> {
  const response = await fetch('/api/auth/login', {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ Username: username, Password: password, TwoFactorCode: twoFactorCode }),
  });
  if (!response.ok) {
    const error = await responseError(response, 'panel login');
    if (response.status === 401 && /two-factor/i.test(error.message)) throw new LocalizedError('login.invalidTwoFactor');
    if (response.status === 401) throw new LocalizedError('login.invalidCredentials');
    throw error;
  }
}

export async function getPanelSecurity(): Promise<PanelSecurityStatus> {
  const response = await fetch('/api/security', {
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'panel security');
  const payload = await response.json() as Partial<PanelSecurityStatus>;
  return {
    twoFactorEnabled: payload.twoFactorEnabled === true,
    apiTokens: Array.isArray(payload.apiTokens) ? payload.apiTokens : [],
  };
}

export async function preparePanelTOTP(): Promise<TOTPSetup> {
  const response = await fetch('/api/security/totp/prepare', {
    method: 'POST',
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'prepare TOTP');
  return response.json() as Promise<TOTPSetup>;
}

export async function enablePanelTOTP(secret: string, code: string): Promise<void> {
  const response = await fetch('/api/security/totp/enable', {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ Secret: secret, Code: code }),
  });
  if (!response.ok) throw await responseError(response, 'enable TOTP');
}

export async function disablePanelTOTP(code: string): Promise<void> {
  const response = await fetch('/api/security/totp/disable', {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ Code: code }),
  });
  if (!response.ok) throw await responseError(response, 'disable TOTP');
}

export async function createPanelAPIToken(name: string, expiresAt?: string): Promise<{ token: string; item: PanelAPIToken }> {
  const response = await fetch('/api/security/tokens', {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ Name: name, ExpiresAt: expiresAt || '' }),
  });
  if (!response.ok) throw await responseError(response, 'create API token');
  const payload = await response.json() as { token?: string; item?: PanelAPIToken };
  if (!payload.token || !payload.item) throw new LocalizedError('api.tokenIncomplete');
  return { token: payload.token, item: payload.item };
}

export async function deletePanelAPIToken(id: string): Promise<void> {
  const response = await fetch(`/api/security/tokens/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'delete API token');
}

export async function logoutPanel(): Promise<void> {
  const response = await fetch('/api/auth/logout', {
    method: 'POST',
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'panel logout');
}

export async function getStats(managedNodeID?: string): Promise<StatsReport> {
  const path = isRemoteManagedNode(managedNodeID)
    ? `/api/nodes/${encodeURIComponent(managedNodeID)}/stats`
    : '/api/stats';
  const response = await fetch(path, {
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw new Error(`stats ${response.status}`);
  return response.json() as Promise<StatsReport>;
}

export async function getRuntimeConfig(): Promise<RuntimeConfig> {
  const response = await fetch('/api/config', {
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw new Error(`config ${response.status}`);
  return unwrapConfig(await response.json());
}

export async function getClientShare(clientId: string, managedNodeID?: string): Promise<ClientShare> {
  const path = isRemoteManagedNode(managedNodeID)
    ? `/api/nodes/${encodeURIComponent(managedNodeID)}/share/clients/${encodeURIComponent(clientId)}`
    : `/api/share/clients/${encodeURIComponent(clientId)}`;
  const response = await fetch(path, {
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'client share');
  const payload = await response.json() as { share?: ClientShare };
  if (!payload.share) throw new LocalizedError('api.clientShareMissing');
  return payload.share;
}

export async function saveRuntimeConfig(config: RuntimeConfig): Promise<RuntimeConfig> {
  const response = await fetch('/api/config', {
    method: 'PUT',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    },
    credentials: 'same-origin',
    body: JSON.stringify(config),
  });
  if (!response.ok) throw await responseError(response, 'config save');
  return unwrapConfig(await response.json(), config);
}

export async function applyRuntimeConfig(): Promise<void> {
  const response = await fetch('/api/runtime/apply', {
    method: 'POST',
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'runtime apply');
}

export async function stopRuntime(): Promise<void> {
  const response = await fetch('/api/runtime/stop', {
    method: 'POST',
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'runtime stop');
}

export type RuntimeComponent = 'tapx' | 'embedded-xray' | 'external-xray';

export async function restartRuntimeComponent(component: RuntimeComponent): Promise<void> {
  await runtimeComponentAction(component, 'restart');
}

export async function stopRuntimeComponent(component: RuntimeComponent): Promise<void> {
  await runtimeComponentAction(component, 'stop');
}

async function runtimeComponentAction(component: RuntimeComponent, action: 'restart' | 'stop'): Promise<void> {
  const response = await fetch(`/api/runtime/components/${encodeURIComponent(component)}/${action}`, {
    method: 'POST',
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, `runtime component ${action}`);
}

export async function getPanelLogs(): Promise<PanelLogEvent[]> {
  const response = await fetch('/api/logs', {
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'logs');
  const payload = await response.json() as { events?: PanelLogEvent[] };
  return Array.isArray(payload.events) ? payload.events : [];
}

export async function clearPanelLogs(): Promise<void> {
  const response = await fetch('/api/logs', {
    method: 'DELETE',
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'clear logs');
}

export async function downloadBackupDatabase(): Promise<{ blob: Blob; filename: string }> {
  const response = await fetch('/api/backup', {
    headers: { Accept: 'application/vnd.sqlite3' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'backup');
  const disposition = response.headers.get('content-disposition') || '';
  const filename = /filename="?([^";]+)"?/i.exec(disposition)?.[1] || `tapx-backup-${Date.now()}.db`;
  return { blob: await response.blob(), filename };
}

export async function restoreBackupDatabase(file: Blob): Promise<RuntimeConfig> {
  const response = await fetch('/api/backup/restore', {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/vnd.sqlite3' },
    credentials: 'same-origin',
    body: file,
  });
  if (!response.ok) throw await responseError(response, 'backup restore');
  return unwrapConfig(await response.json());
}

export async function getExternalXrayStatus(path?: string): Promise<XrayBinaryStatus> {
  const query = path?.trim() ? `?path=${encodeURIComponent(path.trim())}` : '';
  const response = await fetch(`/api/xray/external/status${query}`, {
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'external xray status');
  const payload = await response.json() as { binary?: XrayBinaryStatus };
  if (!payload.binary) throw new LocalizedError('api.externalXrayStatusMissing');
  return payload.binary;
}

export async function getDiagnostics(): Promise<DiagnosticReport> {
  const response = await fetch('/api/diagnostics', {
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'diagnostics');
  return await response.json() as DiagnosticReport;
}

export async function getComponentUpdateCatalog(
  component: UpdateComponent,
  options?: { channel?: 'stable' | 'development'; path?: string },
): Promise<ComponentUpdateCatalog> {
  const query = new URLSearchParams();
  if (options?.channel) query.set('channel', options.channel);
  if (options?.path?.trim()) query.set('path', options.path.trim());
  const suffix = query.size ? `?${query.toString()}` : '';
  const response = await fetch(`/api/updates/${component}${suffix}`, {
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'component update catalog');
  return await response.json() as ComponentUpdateCatalog;
}

export async function installComponentUpdate(
  component: UpdateComponent,
  version: string,
  path?: string,
): Promise<{ binary?: XrayBinaryStatus; restarting?: boolean; version?: string; runtimeApplied?: boolean; runtimeWarning?: string }> {
  const response = await fetch(`/api/updates/${component}`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ version, path: path || '' }),
  });
  if (!response.ok) throw await responseError(response, 'component update');
  return await response.json() as {
    binary?: XrayBinaryStatus;
    restarting?: boolean;
    version?: string;
    runtimeApplied?: boolean;
    runtimeWarning?: string;
  };
}

export interface ExternalXrayDownloadInput {
  url: string;
  path?: string;
  sha256?: string;
  timeoutSecond?: number;
  retryCount?: number;
  overwriteStrategy?: 'backup' | 'overwrite' | 'skip';
}

export async function downloadExternalXray(input: ExternalXrayDownloadInput): Promise<XrayBinaryStatus> {
  const response = await fetch('/api/xray/external/download', {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({
      url: input.url,
      path: input.path || '',
      sha256: input.sha256 || '',
      timeoutSecond: input.timeoutSecond || 0,
      retryCount: input.retryCount || 0,
      overwriteStrategy: input.overwriteStrategy || 'backup',
    }),
  });
  if (!response.ok) throw await responseError(response, 'external xray download');
  const payload = await response.json() as { binary?: XrayBinaryStatus };
  if (!payload.binary) throw new LocalizedError('api.externalXrayDownloadMissing');
  return payload.binary;
}

export async function uploadExternalXray(file: File, path?: string): Promise<XrayBinaryStatus> {
  const query = path?.trim() ? `?path=${encodeURIComponent(path.trim())}` : '';
  const body = new FormData();
  body.append('file', file);
  const response = await fetch(`/api/xray/external/upload${query}`, {
    method: 'POST',
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
    body,
  });
  if (!response.ok) throw await responseError(response, 'external xray upload');
  const payload = await response.json() as { binary?: XrayBinaryStatus };
  if (!payload.binary) throw new LocalizedError('api.externalXrayUploadMissing');
  return payload.binary;
}

export async function getGeneratedRuntime(): Promise<unknown> {
  const response = await fetch('/api/runtime', {
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'generated runtime');
  const payload = await response.json() as { runtime?: unknown };
  return payload.runtime ?? payload;
}

export async function restartPanelService(): Promise<void> {
  const response = await fetch('/api/panel/restart', {
    method: 'POST',
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'panel restart');
}

export interface AdminCredentialsInput {
  oldUsername: string;
  oldPassword: string;
  newUsername: string;
  newPassword: string;
}

export async function updateAdminCredentials(input: AdminCredentialsInput): Promise<void> {
  const response = await fetch('/api/panel/credentials', {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({
      OldUsername: input.oldUsername,
      OldPassword: input.oldPassword,
      NewUsername: input.newUsername,
      NewPassword: input.newPassword,
    }),
  });
  if (!response.ok) throw await responseError(response, 'administrator credentials');
}

export interface ConnectorTestResult {
  id: string;
  kind: ConnectorTestKind;
  target: string;
  network: string;
  delayMs?: number;
  confirmed: boolean;
  active: boolean;
  message: string;
  deviceName?: string;
  confirmedPathMtu?: number;
  effectiveNetworkMtu?: number;
  maxDatagramPayload?: number;
  tcpMssIpv4?: number;
  tcpMssIpv6?: number;
  uploadBytes?: number;
  downloadBytes?: number;
  uploadBps?: number;
  downloadBps?: number;
  durationMs?: number;
}

export type ConnectorTestKind = 'channel' | 'path-mtu' | 'throughput';

export async function testConnector(id: string, kind: ConnectorTestKind, durationSeconds?: number, managedNodeID?: string): Promise<ConnectorTestResult> {
  const path = isRemoteManagedNode(managedNodeID)
    ? `/api/nodes/${encodeURIComponent(managedNodeID)}/connectors/test`
    : '/api/connectors/test';
  const response = await fetch(path, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ id, kind, durationSeconds }),
  });
  if (!response.ok) throw await responseError(response, 'connector test');
  const payload = await response.json() as { result?: ConnectorTestResult };
  if (!payload.result) throw new LocalizedError('api.connectorTestMissing');
  return payload.result;
}

export async function resetConnectorTraffic(id: string, managedNodeID?: string): Promise<RuntimeConfig> {
  const path = managedTrafficResetPath(managedNodeID, 'connectors', id);
  const response = await fetch(path, {
    method: 'POST',
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'connector traffic reset');
  return unwrapConfig(await response.json());
}

export async function resetListenerTraffic(id: string, managedNodeID?: string): Promise<RuntimeConfig> {
  const path = managedTrafficResetPath(managedNodeID, 'listeners', id);
  const response = await fetch(path, {
    method: 'POST',
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'listener traffic reset');
  return unwrapConfig(await response.json());
}

export async function resetClientTraffic(id: string, managedNodeID?: string): Promise<RuntimeConfig> {
  const path = managedTrafficResetPath(managedNodeID, 'clients', id);
  const response = await fetch(path, {
    method: 'POST',
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw await responseError(response, 'client traffic reset');
  return unwrapConfig(await response.json());
}

function isRemoteManagedNode(managedNodeID?: string): managedNodeID is string {
  return Boolean(managedNodeID && managedNodeID !== 'local');
}

function managedTrafficResetPath(managedNodeID: string | undefined, kind: 'clients' | 'connectors' | 'listeners', id: string): string {
  const suffix = `/${kind}/${encodeURIComponent(id)}/traffic/reset`;
  return isRemoteManagedNode(managedNodeID)
    ? `/api/nodes/${encodeURIComponent(managedNodeID)}${suffix}`
    : `/api${suffix}`;
}

export async function getVlessEncryptionAuths(): Promise<VlessEncryptionAuth[]> {
  const response = await fetch('/api/xray/vless-encryption', {
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  });
  if (!response.ok) throw new LocalizedError('api.vlessAuthStatus', { status: response.status });
  if (!String(response.headers.get('content-type') || '').includes('application/json')) {
    throw new LocalizedError('api.vlessAuthNotIntegrated');
  }
  const payload = await response.json() as {
    success?: boolean;
    msg?: string;
    obj?: { auths?: VlessEncryptionAuth[] };
    auths?: VlessEncryptionAuth[];
  };
  if (payload.success === false) throw new Error(payload.msg || 'VLESS key generation failed');
  const auths = payload.obj?.auths || payload.auths || [];
  if (!Array.isArray(auths) || auths.length === 0) throw new LocalizedError('api.vlessAuthMissing');
  return auths;
}

function unwrapConfig(payload: unknown, fallback: RuntimeConfig = {}): RuntimeConfig {
  if (payload && typeof payload === 'object' && 'config' in payload) {
    const boxed = payload as { config?: RuntimeConfig };
    return boxed.config || fallback;
  }
  if (payload && typeof payload === 'object') {
    const candidate = payload as RuntimeConfig;
    const runtimeKeys: Array<keyof RuntimeConfig> = [
      'Devices',
      'Listeners',
      'Connectors',
      'Clients',
      'Routes',
      'VKeys',
      'Addresses',
      'XrayProfiles',
      'Settings',
    ];
    if (runtimeKeys.some((key) => key in candidate)) return candidate;
  }
  return fallback;
}
