export type SettingRow = {
  ID?: string;
  Enabled?: boolean;
  Name?: string;
  PanelListen?: string;
  PanelDomain?: string;
  PanelBasePath?: string;
  PanelHTTPS?: boolean;
  PanelCertFile?: string;
  PanelKeyFile?: string;
  PanelAuthEnabled?: boolean;
  AdminUsername?: string;
  AdminPasswordHash?: string;
  SessionTTLSecond?: number;
  Timezone?: string;
  PanelOutbound?: string;
  ExternalXrayPath?: string;
  ExternalXrayConfigFile?: string;
  ExternalXrayWorkDir?: string;
  ExternalXrayArgs?: string;
  LogLevel?: string;
  StatsIntervalSecond?: number;
  BackupDir?: string;
  DataDir?: string;
  OpenWrtBuildTarget?: string;
  AdvancedJSON?: string;
  Remark?: string;
  Key?: string;
  key?: string;
  Value?: unknown;
  value?: unknown;
};

export function settingsToObject<T extends Record<string, unknown> = Record<string, unknown>>(settings: unknown): Partial<T> {
  if (!Array.isArray(settings)) return {};
  const output: Record<string, unknown> = {};

  for (const item of settings as SettingRow[]) {
    const legacyKey = item.Key || item.key;
    if (legacyKey) output[legacyKey] = item.Value ?? item.value;
  }

  const row = (settings as SettingRow[]).find((item) => isTypedSettingsRow(item));
  if (!row) return output as Partial<T>;
  Object.assign(output, parseAdvancedSettings(row.AdvancedJSON));

  output.settingsID = row.ID || 'settings';
  output.settingsName = row.Name || 'TapX';
  output.settingsEnabled = row.Enabled !== false;
  output.panelHTTPS = row.PanelHTTPS === true;
  output.panelAuthEnabled = row.PanelAuthEnabled === true;
  output.adminUsername = row.AdminUsername || '';
  output._adminPasswordHash = row.AdminPasswordHash || '';
  output.certPublicPath = row.PanelCertFile || output.certPublicPath || '';
  output.certPrivatePath = row.PanelKeyFile || output.certPrivatePath || '';
  output.listenDomain = row.PanelDomain || output.listenDomain || '';
  output.uriPath = row.PanelBasePath || output.uriPath || '/';
  output.timezone = row.Timezone || output.timezone || '';
  output.panelOutbound = row.PanelOutbound || output.panelOutbound || 'direct';
  output.externalXrayPath = row.ExternalXrayPath || output.externalXrayPath || '';
  output.externalXrayConfigFile = row.ExternalXrayConfigFile || output.externalXrayConfigFile || '';
  output.externalXrayWorkDir = row.ExternalXrayWorkDir || output.externalXrayWorkDir || '';
  output.externalXrayArgs = row.ExternalXrayArgs || output.externalXrayArgs || '';
  output.runtimeLogLevel = row.LogLevel || output.runtimeLogLevel || '';
  output.backupDir = row.BackupDir || output.backupDir || '';
  output.dataDir = row.DataDir || output.dataDir || '';
  output.openWrtBuildTarget = row.OpenWrtBuildTarget || output.openWrtBuildTarget || '';
  output.settingsRemark = row.Remark || '';
  if (row.SessionTTLSecond && output.sessionMinutes == null) output.sessionMinutes = Math.max(1, Math.round(row.SessionTTLSecond / 60));
  if (row.StatsIntervalSecond && output.tapxStatsInterval == null) output.tapxStatsInterval = row.StatsIntervalSecond;
  if (row.PanelListen && (output.listenIP == null || output.listenPort == null)) {
    const listen = splitPanelListen(row.PanelListen);
    if (output.listenIP == null) output.listenIP = listen.host;
    if (output.listenPort == null && listen.port > 0) output.listenPort = listen.port;
  }
  return output as Partial<T>;
}

export function objectToSettings(values: Record<string, unknown>): SettingRow[] {
  const advanced = { ...values };
  for (const key of [
    'settingsID',
    'settingsName',
    'settingsEnabled',
    'panelHTTPS',
    'listenDomain',
    'uriPath',
    'timezone',
    'panelOutbound',
    'panelAuthEnabled',
    'adminUsername',
    '_adminPasswordHash',
    'certPublicPath',
    'certPrivatePath',
    'externalXrayPath',
    'externalXrayConfigFile',
    'externalXrayWorkDir',
    'externalXrayArgs',
    'runtimeLogLevel',
    'backupDir',
    'dataDir',
    'openWrtBuildTarget',
    'settingsRemark',
    'oldPassword',
    'newPassword',
    'oldUsername',
    'newUsername',
  ]) {
    delete advanced[key];
  }

  const listenIP = stringValue(values.listenIP);
  const listenPort = numberValue(values.listenPort);
  const certFile = stringValue(values.certPublicPath);
  const keyFile = stringValue(values.certPrivatePath);
  return [{
    ID: stringValue(values.settingsID) || 'settings',
    Enabled: values.settingsEnabled !== false,
    Name: stringValue(values.settingsName) || 'TapX',
    PanelListen: joinPanelListen(listenIP, listenPort),
    PanelDomain: stringValue(values.listenDomain),
    PanelBasePath: stringValue(values.uriPath) || '/',
    PanelHTTPS: values.panelHTTPS === true || Boolean(certFile && keyFile),
    PanelCertFile: certFile,
    PanelKeyFile: keyFile,
    PanelAuthEnabled: values.panelAuthEnabled === true,
    AdminUsername: stringValue(values.adminUsername),
    AdminPasswordHash: stringValue(values._adminPasswordHash),
    SessionTTLSecond: Math.max(0, numberValue(values.sessionMinutes) * 60),
    Timezone: stringValue(values.timezone),
    PanelOutbound: stringValue(values.panelOutbound) || 'direct',
    ExternalXrayPath: stringValue(values.externalXrayPath),
    ExternalXrayConfigFile: stringValue(values.externalXrayConfigFile),
    ExternalXrayWorkDir: stringValue(values.externalXrayWorkDir),
    ExternalXrayArgs: stringValue(values.externalXrayArgs),
    LogLevel: stringValue(values.runtimeLogLevel) || 'info',
    StatsIntervalSecond: Math.max(0, numberValue(values.tapxStatsInterval)),
    BackupDir: stringValue(values.backupDir),
    DataDir: stringValue(values.dataDir),
    OpenWrtBuildTarget: stringValue(values.openWrtBuildTarget),
    AdvancedJSON: JSON.stringify(advanced),
    Remark: stringValue(values.settingsRemark),
  }];
}

export function stableSettingsSnapshot(values: Record<string, unknown>): string {
  return JSON.stringify(
    Object.entries(values)
      .filter(([, value]) => value !== undefined)
      .sort(([left], [right]) => left.localeCompare(right)),
  );
}

function isTypedSettingsRow(item: SettingRow): boolean {
  return Boolean(item.ID || item.PanelListen || item.AdvancedJSON || 'PanelHTTPS' in item || 'ExternalXrayPath' in item);
}

function parseAdvancedSettings(value?: string): Record<string, unknown> {
  if (!value) return {};
  try {
    const parsed = JSON.parse(value) as unknown;
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed as Record<string, unknown> : {};
  } catch {
    return {};
  }
}

function splitPanelListen(value: string): { host: string; port: number } {
  const input = value.trim();
  if (!input) return { host: '', port: 0 };
  if (input.startsWith('[')) {
    const end = input.indexOf(']');
    if (end >= 0) return { host: input.slice(1, end), port: Number(input.slice(end + 2)) || 0 };
  }
  const separator = input.lastIndexOf(':');
  if (separator < 0) return { host: input, port: 0 };
  return { host: input.slice(0, separator), port: Number(input.slice(separator + 1)) || 0 };
}

function joinPanelListen(host: string, port: number): string {
  if (!host) return port > 0 ? `:${port}` : '';
  const normalizedHost = host.includes(':') && !host.startsWith('[') ? `[${host}]` : host;
  return port > 0 ? `${normalizedHost}:${port}` : normalizedHost;
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

function numberValue(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}
