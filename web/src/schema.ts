import type { AnyRecord, RuntimeConfig } from './api';

export type FieldType = 'text' | 'number' | 'switch' | 'select' | 'textarea' | 'list' | 'json';

export interface FieldDef {
  path: string;
  label: string;
  type?: FieldType;
  options?: string[];
  span?: 1 | 2;
  placeholder?: string;
}

export interface FieldGroup {
  title: string;
  fields: FieldDef[];
}

export interface KindDef {
  key: string;
  configKey: keyof RuntimeConfig;
  title: string;
  menuKey: string;
  summary: string;
  defaultPrefix: string;
  primaryFields: string[];
  groups: FieldGroup[];
  template: (id: string) => AnyRecord;
}

const common: FieldDef[] = [
  { path: 'ID', label: 'ID' },
  { path: 'Enabled', label: 'Enabled', type: 'switch' },
  { path: 'Name', label: 'Name' },
];

const remark: FieldDef = { path: 'Remark', label: 'Remark', type: 'textarea', span: 2 };

const binding: FieldDef[] = [
  { path: 'Binding.RouteID', label: 'Binding Route' },
  { path: 'Binding.DeviceID', label: 'Binding Device' },
  { path: 'Binding.ConnectorID', label: 'Binding Connector' },
  { path: 'Binding.ClientID', label: 'Binding Client' },
  { path: 'Binding.VKeyID', label: 'Binding vKey' },
  { path: 'Binding.AddressID', label: 'Binding Address' },
];

const rawUDP: FieldDef[] = [
  { path: 'RawUDP.PeerMode', label: 'Peer Mode', type: 'select', options: ['', 'any', 'fixed', 'learn'] },
  { path: 'RawUDP.FixedPeer', label: 'Fixed Peer' },
  { path: 'RawUDP.BindInterface', label: 'Bind Interface' },
  { path: 'RawUDP.BindAddress', label: 'Bind Address' },
  { path: 'RawUDP.ReceiveBuffer', label: 'Receive Buffer', type: 'number' },
  { path: 'RawUDP.SendBuffer', label: 'Send Buffer', type: 'number' },
  { path: 'RawUDP.ReuseAddr', label: 'SO_REUSEADDR', type: 'switch' },
  { path: 'RawUDP.ReusePort', label: 'SO_REUSEPORT', type: 'switch' },
  { path: 'RawUDP.KeepAliveSecond', label: 'Keepalive Seconds', type: 'number' },
  { path: 'RawUDP.Workers', label: 'Workers', type: 'number' },
  { path: 'RawUDP.QueueSize', label: 'Queue Size', type: 'number' },
  { path: 'RawUDP.DTLS.Enabled', label: 'DTLS Enabled', type: 'switch' },
  { path: 'RawUDP.DTLS.CertFile', label: 'DTLS Cert File' },
  { path: 'RawUDP.DTLS.KeyFile', label: 'DTLS Key File' },
  { path: 'RawUDP.DTLS.CAFile', label: 'DTLS CA File' },
  { path: 'RawUDP.DTLS.ServerName', label: 'DTLS Server Name' },
  { path: 'RawUDP.DTLS.ALPN', label: 'DTLS ALPN', type: 'list', span: 2 },
  { path: 'RawUDP.DTLS.MinVersion', label: 'DTLS Min Version', type: 'select', options: ['', '1.0', '1.1', '1.2', '1.3'] },
  { path: 'RawUDP.DTLS.MaxVersion', label: 'DTLS Max Version', type: 'select', options: ['', '1.0', '1.1', '1.2', '1.3'] },
  { path: 'RawUDP.DTLS.AllowInsecure', label: 'DTLS Allow Insecure', type: 'switch' },
  { path: 'RawUDP.DTLS.MTU', label: 'DTLS MTU', type: 'number' },
  { path: 'RawUDP.DTLS.ReplayWindow', label: 'DTLS Replay Window', type: 'number' },
];

const rawTCP: FieldDef[] = [
  { path: 'RawTCP.LengthMode', label: 'Length Mode', type: 'select', options: ['', 'uint16', 'uint32'] },
  { path: 'RawTCP.BindInterface', label: 'Bind Interface' },
  { path: 'RawTCP.BindAddress', label: 'Bind Address' },
  { path: 'RawTCP.ReceiveBuffer', label: 'Receive Buffer', type: 'number' },
  { path: 'RawTCP.SendBuffer', label: 'Send Buffer', type: 'number' },
  { path: 'RawTCP.NoDelay', label: 'TCP_NODELAY', type: 'switch' },
  { path: 'RawTCP.KeepAliveSecond', label: 'Keepalive Seconds', type: 'number' },
  { path: 'RawTCP.FastOpen', label: 'TCP Fast Open', type: 'switch' },
  { path: 'RawTCP.ConnectTimeout', label: 'Connect Timeout', type: 'number' },
  { path: 'RawTCP.ReconnectSecond', label: 'Reconnect Seconds', type: 'number' },
  { path: 'RawTCP.Workers', label: 'Workers', type: 'number' },
  { path: 'RawTCP.ReadBuffer', label: 'Read Buffer', type: 'number' },
  { path: 'RawTCP.WriteBuffer', label: 'Write Buffer', type: 'number' },
  { path: 'RawTCP.TLS.Enabled', label: 'TLS Enabled', type: 'switch' },
  { path: 'RawTCP.TLS.CertFile', label: 'TLS Cert File' },
  { path: 'RawTCP.TLS.KeyFile', label: 'TLS Key File' },
  { path: 'RawTCP.TLS.CAFile', label: 'TLS CA File' },
  { path: 'RawTCP.TLS.ServerName', label: 'TLS Server Name' },
  { path: 'RawTCP.TLS.ALPN', label: 'TLS ALPN', type: 'list', span: 2 },
  { path: 'RawTCP.TLS.MinVersion', label: 'TLS Min Version', type: 'select', options: ['', '1.0', '1.1', '1.2', '1.3'] },
  { path: 'RawTCP.TLS.MaxVersion', label: 'TLS Max Version', type: 'select', options: ['', '1.0', '1.1', '1.2', '1.3'] },
  { path: 'RawTCP.TLS.AllowInsecure', label: 'TLS Allow Insecure', type: 'switch' },
];

export const kindDefs: KindDef[] = [
  {
    key: 'listeners',
    configKey: 'Listeners',
    title: 'Inbounds',
    menuKey: 'listeners',
    summary: 'Raw UDP/TCP and Xray inbound endpoints.',
    defaultPrefix: 'listener',
    primaryFields: ['BindHost', 'BindPort', 'Transport', 'XrayProfileID'],
    groups: [
      { title: 'Inbound', fields: [...common, { path: 'BindHost', label: 'Bind Host' }, { path: 'BindPort', label: 'Bind Port', type: 'number' }, { path: 'Transport', label: 'Transport', type: 'select', options: ['udp', 'tcp', 'xray'] }, { path: 'XrayProfileID', label: 'Xray Profile' }, remark] },
      { title: 'Binding', fields: binding },
      { title: 'Raw UDP', fields: rawUDP },
      { title: 'Raw TCP', fields: rawTCP },
    ],
    template: (id) => ({
      ID: id, Enabled: true, Name: '', BindHost: '127.0.0.1', BindPort: 40000, Transport: 'udp', XrayProfileID: '',
      RawUDP: { PeerMode: 'learn', FixedPeer: '', BindInterface: '', BindAddress: '', ReceiveBuffer: 0, SendBuffer: 0, ReuseAddr: true, ReusePort: false, KeepAliveSecond: 0, Workers: 0, QueueSize: 0, DTLS: { Enabled: false, CertFile: '', KeyFile: '', CAFile: '', ServerName: '', ALPN: [], MinVersion: '', MaxVersion: '', AllowInsecure: false, MTU: 0, ReplayWindow: 0 } },
      RawTCP: { LengthMode: 'uint16', BindInterface: '', BindAddress: '', ReceiveBuffer: 0, SendBuffer: 0, NoDelay: true, KeepAliveSecond: 30, FastOpen: false, ConnectTimeout: 3, ReconnectSecond: 0, Workers: 0, ReadBuffer: 0, WriteBuffer: 0, TLS: { Enabled: false, CertFile: '', KeyFile: '', CAFile: '', ServerName: '', ALPN: [], MinVersion: '', MaxVersion: '', AllowInsecure: false } },
      Binding: {}, Remark: '',
    }),
  },
  {
    key: 'connectors',
    configKey: 'Connectors',
    title: 'Outbounds',
    menuKey: 'connectors',
    summary: 'Remote raw UDP/TCP and Xray outbound endpoints.',
    defaultPrefix: 'connector',
    primaryFields: ['Remote', 'Port', 'Transport', 'XrayProfileID'],
    groups: [
      { title: 'Outbound', fields: [...common, { path: 'Remote', label: 'Remote' }, { path: 'Port', label: 'Port', type: 'number' }, { path: 'Transport', label: 'Transport', type: 'select', options: ['udp', 'tcp', 'xray'] }, { path: 'XrayProfileID', label: 'Xray Profile' }, remark] },
      { title: 'Binding', fields: binding },
      { title: 'Raw UDP', fields: rawUDP },
      { title: 'Raw TCP', fields: rawTCP },
    ],
    template: (id) => ({
      ID: id, Enabled: true, Name: '', Remote: '127.0.0.1', Port: 40000, Transport: 'udp', XrayProfileID: '',
      RawUDP: { PeerMode: 'fixed', FixedPeer: '', BindInterface: '', BindAddress: '', ReceiveBuffer: 0, SendBuffer: 0, ReuseAddr: true, ReusePort: false, KeepAliveSecond: 0, Workers: 0, QueueSize: 0, DTLS: { Enabled: false, CertFile: '', KeyFile: '', CAFile: '', ServerName: '', ALPN: [], MinVersion: '', MaxVersion: '', AllowInsecure: false, MTU: 0, ReplayWindow: 0 } },
      RawTCP: { LengthMode: 'uint16', BindInterface: '', BindAddress: '', ReceiveBuffer: 0, SendBuffer: 0, NoDelay: true, KeepAliveSecond: 30, FastOpen: false, ConnectTimeout: 3, ReconnectSecond: 0, Workers: 0, ReadBuffer: 0, WriteBuffer: 0, TLS: { Enabled: false, CertFile: '', KeyFile: '', CAFile: '', ServerName: '', ALPN: [], MinVersion: '', MaxVersion: '', AllowInsecure: false } },
      Binding: {}, Remark: '',
    }),
  },
  {
    key: 'clients',
    configKey: 'Clients',
    title: 'Clients',
    menuKey: 'clients',
    summary: 'Optional identities, credentials, binding and traffic limits.',
    defaultPrefix: 'client',
    primaryFields: ['Email', 'ListenerID', 'CredentialType', 'AddressID'],
    groups: [
      { title: 'Client', fields: [...common, { path: 'Email', label: 'Email' }, { path: 'ListenerID', label: 'Listener' }, { path: 'CredentialType', label: 'Credential Type', type: 'select', options: ['', 'uuid', 'password', 'vkey'] }, { path: 'CredentialValue', label: 'Credential Value' }, { path: 'AddressID', label: 'Address Limit' }, { path: 'ExpiresAt', label: 'Expires At', type: 'number' }, { path: 'TrafficCap', label: 'Traffic Cap', type: 'number' }, { path: 'TrafficResetAt', label: 'Traffic Reset At', type: 'number' }, { path: 'TrafficRXOffset', label: 'Traffic RX Offset', type: 'number' }, { path: 'TrafficTXOffset', label: 'Traffic TX Offset', type: 'number' }, remark] },
      { title: 'Binding', fields: binding },
    ],
    template: (id) => ({ ID: id, Enabled: true, Name: '', Email: '', ListenerID: '', CredentialType: '', CredentialValue: '', Binding: {}, AddressID: '', ExpiresAt: 0, TrafficCap: 0, TrafficResetAt: 0, TrafficRXOffset: 0, TrafficTXOffset: 0, Remark: '' }),
  },
  {
    key: 'routes',
    configKey: 'Routes',
    title: 'Routes',
    menuKey: 'routes',
    summary: 'Composable binding rules between listeners, connectors, devices, clients and vKeys.',
    defaultPrefix: 'route',
    primaryFields: ['VKeyID', 'ListenerID', 'DeviceID', 'ConnectorID', 'ClientID', 'AddressID'],
    groups: [
      { title: 'Route', fields: [{ path: 'ID', label: 'ID' }, { path: 'Enabled', label: 'Enabled', type: 'switch' }, { path: 'VKeyID', label: 'vKey' }, { path: 'ListenerID', label: 'Listener' }, { path: 'DeviceID', label: 'Device' }, { path: 'ConnectorID', label: 'Connector' }, { path: 'ClientID', label: 'Client' }, { path: 'AddressID', label: 'Address Limit' }] },
    ],
    template: (id) => ({ ID: id, Enabled: true, VKeyID: '', ListenerID: '', DeviceID: '', ConnectorID: '', ClientID: '', AddressID: '' }),
  },
  {
    key: 'devices',
    configKey: 'Devices',
    title: 'Devices',
    menuKey: 'devices',
    summary: 'TUN/TAP interfaces, route injection, DNS and bridge settings.',
    defaultPrefix: 'device',
    primaryFields: ['Type', 'IfName', 'MTU', 'IPv4CIDR', 'IPv6CIDR'],
    groups: [
      { title: 'Device', fields: [...common, { path: 'Type', label: 'Type', type: 'select', options: ['tun', 'tap'] }, { path: 'IfName', label: 'Interface Name' }, { path: 'MTU', label: 'MTU', type: 'number' }, { path: 'MSSClamp', label: 'MSS Clamp', type: 'number' }, { path: 'IPv4CIDR', label: 'IPv4 CIDR' }, { path: 'IPv6CIDR', label: 'IPv6 CIDR' }, remark] },
      { title: 'Bridge', fields: [{ path: 'Bridge.Enabled', label: 'Enabled', type: 'switch' }, { path: 'Bridge.Name', label: 'Bridge Name' }, { path: 'Bridge.IfName', label: 'Member Interface' }, { path: 'Bridge.MTU', label: 'Bridge MTU', type: 'number' }] },
      { title: 'Routes and DNS', fields: [{ path: 'Routes', label: 'Static Routes', type: 'json', span: 2 }, { path: 'DNS', label: 'DNS Config', type: 'json', span: 2 }] },
    ],
    template: (id) => ({ ID: id, Enabled: true, Name: '', Type: 'tun', IfName: 'tapx%d', MTU: 1500, MSSClamp: 0, IPv4CIDR: '', IPv6CIDR: '', Bridge: null, Routes: [], DNS: null, Remark: '' }),
  },
  {
    key: 'vkeys',
    configKey: 'VKeys',
    title: 'vKeys',
    menuKey: 'vkeys',
    summary: 'Optional raw transport admission keys. Empty references add no hot-path work.',
    defaultPrefix: 'vkey',
    primaryFields: ['Value'],
    groups: [{ title: 'vKey', fields: [...common, { path: 'Value', label: 'Value', type: 'textarea', span: 2 }, remark] }],
    template: (id) => ({ ID: id, Enabled: true, Name: '', Value: '', Remark: '' }),
  },
  {
    key: 'addresses',
    configKey: 'Addresses',
    title: 'Address Limits',
    menuKey: 'addressLimits',
    summary: 'Allowed TAP/TUN IP, MAC, gateway, DNS and pushed route limits.',
    defaultPrefix: 'addr',
    primaryFields: ['DeviceID', 'ClientID', 'IPv4CIDRs', 'IPv6CIDRs'],
    groups: [
      { title: 'Address Limit', fields: [...common, { path: 'DeviceID', label: 'Device' }, { path: 'ClientID', label: 'Client' }, { path: 'MACs', label: 'MACs', type: 'list', span: 2 }, { path: 'IPv4CIDRs', label: 'IPv4 CIDRs', type: 'list', span: 2 }, { path: 'IPv6CIDRs', label: 'IPv6 CIDRs', type: 'list', span: 2 }, { path: 'IPv4Gateway', label: 'IPv4 Gateway' }, { path: 'IPv6Gateway', label: 'IPv6 Gateway' }, { path: 'DNS', label: 'DNS', type: 'list', span: 2 }, { path: 'Routes', label: 'Pushed Routes', type: 'list', span: 2 }, { path: 'AllowDefaultRoute', label: 'Allow Default Route', type: 'switch' }, remark] },
    ],
    template: (id) => ({ ID: id, Enabled: true, Name: '', DeviceID: '', ClientID: '', MACs: [], IPv4CIDRs: [], IPv6CIDRs: [], IPv4Gateway: '', IPv6Gateway: '', DNS: [], Routes: [], AllowDefaultRoute: false, Remark: '' }),
  },
  {
    key: 'xrayProfiles',
    configKey: 'XrayProfiles',
    title: 'Xray Profiles',
    menuKey: 'xray',
    summary: 'Same-process embedded Xray or external xray-core transport profiles.',
    defaultPrefix: 'xray',
    primaryFields: ['Runtime', 'InboundProtocol', 'OutboundProtocol', 'Network', 'Security'],
    groups: [
      { title: 'Profile', fields: [...common, { path: 'Runtime', label: 'Runtime', type: 'select', options: ['', 'embedded', 'external'] }, { path: 'InboundProtocol', label: 'Inbound Protocol' }, { path: 'OutboundProtocol', label: 'Outbound Protocol' }, { path: 'Network', label: 'Network' }, { path: 'Security', label: 'Security' }, remark] },
      { title: 'Endpoint JSON', fields: [{ path: 'InboundSettingsJSON', label: 'Inbound Settings JSON', type: 'textarea', span: 2 }, { path: 'OutboundSettingsJSON', label: 'Outbound Settings JSON', type: 'textarea', span: 2 }] },
      { title: 'Xray Template JSON', fields: [{ path: 'StreamSettingsJSON', label: 'Stream Settings JSON', type: 'textarea', span: 2 }, { path: 'SniffingJSON', label: 'Sniffing JSON', type: 'textarea', span: 2 }, { path: 'MuxJSON', label: 'Mux JSON', type: 'textarea', span: 2 }, { path: 'SockoptJSON', label: 'Sockopt JSON', type: 'textarea', span: 2 }, { path: 'FallbacksJSON', label: 'Fallbacks JSON', type: 'textarea', span: 2 }, { path: 'RoutingJSON', label: 'Routing JSON', type: 'textarea', span: 2 }, { path: 'DNSJSON', label: 'DNS JSON', type: 'textarea', span: 2 }, { path: 'PolicyJSON', label: 'Policy JSON', type: 'textarea', span: 2 }, { path: 'AdvancedJSON', label: 'Advanced JSON', type: 'textarea', span: 2 }] },
    ],
    template: (id) => ({ ID: id, Enabled: true, Name: '', Runtime: 'embedded', InboundProtocol: '', InboundSettingsJSON: '{}', OutboundProtocol: '', OutboundSettingsJSON: '{}', Network: '', Security: '', StreamSettingsJSON: '{}', SniffingJSON: '', MuxJSON: '', SockoptJSON: '', FallbacksJSON: '', RoutingJSON: '', DNSJSON: '', PolicyJSON: '', AdvancedJSON: '', Remark: '' }),
  },
  {
    key: 'settings',
    configKey: 'Settings',
    title: 'Settings',
    menuKey: 'settings',
    summary: 'Panel, authentication, runtime, Xray binary and OpenWrt defaults.',
    defaultPrefix: 'settings',
    primaryFields: ['PanelListen', 'PanelHTTPS', 'PanelAuthEnabled', 'ExternalXrayPath'],
    groups: [
      { title: 'Panel', fields: [...common, { path: 'PanelListen', label: 'Panel Listen' }, { path: 'PanelHTTPS', label: 'Panel HTTPS', type: 'switch' }, { path: 'PanelCertFile', label: 'Panel Cert File' }, { path: 'PanelKeyFile', label: 'Panel Key File' }, { path: 'PanelAuthEnabled', label: 'Panel Auth', type: 'switch' }, { path: 'AdminUsername', label: 'Admin Username' }, { path: 'AdminPasswordHash', label: 'Admin Password Hash', type: 'textarea', span: 2 }, { path: 'SessionTTLSecond', label: 'Session TTL', type: 'number' }, remark] },
      { title: 'Runtime', fields: [{ path: 'ExternalXrayPath', label: 'External Xray Path' }, { path: 'LogLevel', label: 'Log Level', type: 'select', options: ['', 'debug', 'info', 'warn', 'error'] }, { path: 'StatsIntervalSecond', label: 'Stats Interval', type: 'number' }, { path: 'BackupDir', label: 'Backup Dir' }, { path: 'DataDir', label: 'Data Dir' }, { path: 'OpenWrtBuildTarget', label: 'OpenWrt Build Target', type: 'select', options: ['', 'x86-64'] }, { path: 'AdvancedJSON', label: 'Advanced JSON', type: 'textarea', span: 2 }] },
    ],
    template: (id) => ({ ID: id, Enabled: true, Name: '', PanelListen: '127.0.0.1:8080', PanelHTTPS: false, PanelCertFile: '', PanelKeyFile: '', PanelAuthEnabled: true, AdminUsername: 'admin', AdminPasswordHash: '', SessionTTLSecond: 86400, ExternalXrayPath: '/usr/local/bin/xray', LogLevel: 'info', StatsIntervalSecond: 5, BackupDir: '/var/lib/tapx/backups', DataDir: '/var/lib/tapx', OpenWrtBuildTarget: 'x86-64', AdvancedJSON: '', Remark: '' }),
  },
];

export const kindByKey = Object.fromEntries(kindDefs.map((kind) => [kind.key, kind])) as Record<string, KindDef>;

export function getItems(config: RuntimeConfig | null | undefined, kind: KindDef): AnyRecord[] {
  const value = config?.[kind.configKey];
  return Array.isArray(value) ? value : [];
}

export function generatedID(kind: KindDef) {
  return `${kind.defaultPrefix}-${Math.random().toString(16).slice(2, 10)}`;
}

export function allFields(kind: KindDef) {
  return kind.groups.flatMap((group) => group.fields);
}

export function getPath(value: AnyRecord, path: string) {
  return path.split('.').reduce<any>((acc, part) => (acc == null ? undefined : acc[part]), value);
}

export function setPath(value: AnyRecord, path: string, next: any) {
  const parts = path.split('.');
  let target = value;
  for (let i = 0; i < parts.length - 1; i += 1) {
    const part = parts[i];
    if (!target[part] || typeof target[part] !== 'object') target[part] = {};
    target = target[part];
  }
  target[parts[parts.length - 1]] = next;
}

export function normalizeObject(value: AnyRecord) {
  if (value.Binding && Object.values(value.Binding).every((item) => item === '' || item == null)) {
    value.Binding = {};
  }
  if (value.Bridge) {
    const bridge = value.Bridge;
    if (!bridge.Enabled && !bridge.Name && !bridge.IfName && !bridge.MTU) value.Bridge = null;
  }
  if (value.DNS) {
    const dns = value.DNS;
    if (!dns.Enabled && !dns.OutputPath && !dns.Nameservers?.length && !dns.SearchDomains?.length && !dns.Options?.length) value.DNS = null;
  }
  return value;
}
