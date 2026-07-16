import type {
  RuntimeConfig,
  TapxAddressLimit,
  TapxBinding,
  TapxClient,
  TapxDevice,
  TapxEndpoint,
  TapxRoute,
  TapxVKey,
} from './api';

export interface OptionItem {
  id: string;
  label: string;
  detail?: string;
}

export interface TapxIndex {
  devices: Map<string, TapxDevice>;
  listeners: Map<string, TapxEndpoint>;
  connectors: Map<string, TapxEndpoint>;
  clients: Map<string, TapxClient>;
  routes: Map<string, TapxRoute>;
  vkeys: Map<string, TapxVKey>;
  addresses: Map<string, TapxAddressLimit>;
}

export function emptyConfig(config: RuntimeConfig = {}): Required<Pick<RuntimeConfig, 'Devices' | 'Listeners' | 'Connectors' | 'Clients' | 'Routes' | 'VKeys' | 'Addresses'>> {
  return {
    Devices: config.Devices || [],
    Listeners: config.Listeners || [],
    Connectors: config.Connectors || [],
    Clients: config.Clients || [],
    Routes: config.Routes || [],
    VKeys: config.VKeys || [],
    Addresses: config.Addresses || [],
  };
}

export function buildIndex(config: RuntimeConfig): TapxIndex {
  const normalized = emptyConfig(config);
  return {
    devices: byId(normalized.Devices),
    listeners: byId(normalized.Listeners),
    connectors: byId(normalized.Connectors),
    clients: byId(normalized.Clients),
    routes: byId(normalized.Routes),
    vkeys: byId(normalized.VKeys),
    addresses: byId(normalized.Addresses),
  };
}

export function labelDevice(device?: TapxDevice): string {
  if (!device) return '-';
  const name = first(device.Name, device.IfName, device.ID);
  return `${name}${device.Type ? ` (${device.Type.toUpperCase()})` : ''}`;
}

export function labelEndpoint(item?: TapxEndpoint): string {
  if (!item) return '-';
  const name = first(item.Name, item.ID);
  const port = item.BindPort || item.Port;
  return port ? `${name}:${port}` : name;
}

export function labelClient(item?: TapxClient): string {
  if (!item) return '-';
  return first(item.Email, item.Name, item.ID);
}

export function labelVKey(item?: TapxVKey): string {
  if (!item) return '-';
  return first(item.Name, item.ID);
}

export function labelAddress(item?: TapxAddressLimit): string {
  if (!item) return '-';
  return first(item.Name, item.ID);
}

export function routeAddress(route: TapxRoute, idx: TapxIndex): TapxAddressLimit | undefined {
  if (route.AddressID) return idx.addresses.get(route.AddressID);
  const client = route.ClientID ? idx.clients.get(route.ClientID) : undefined;
  if (client?.AddressID) return idx.addresses.get(client.AddressID);
  if (client?.Binding?.AddressID) return idx.addresses.get(client.Binding.AddressID);
  return undefined;
}

export function routeDevice(route: TapxRoute, idx: TapxIndex): TapxDevice | undefined {
  if (route.DeviceID) return idx.devices.get(route.DeviceID);
  const client = route.ClientID ? idx.clients.get(route.ClientID) : undefined;
  if (client?.Binding?.DeviceID) return idx.devices.get(client.Binding.DeviceID);
  const listener = route.ListenerID ? idx.listeners.get(route.ListenerID) : undefined;
  if (listener?.Binding?.DeviceID) return idx.devices.get(listener.Binding.DeviceID);
  const connector = route.ConnectorID ? idx.connectors.get(route.ConnectorID) : undefined;
  if (connector?.Binding?.DeviceID) return idx.devices.get(connector.Binding.DeviceID);
  return undefined;
}

// Runtime generation uses the direct binding first and fills only missing
// fields from the referenced route. Keep diagnostics aligned with that shape.
export function resolveBinding(binding: TapxBinding | undefined, idx: TapxIndex): TapxBinding {
  const direct = binding || {};
  const route = direct.RouteID ? idx.routes.get(direct.RouteID) : undefined;
  return {
    VKeyID: direct.VKeyID || route?.VKeyID || '',
    ClientID: direct.ClientID || route?.ClientID || '',
    RouteID: direct.RouteID || '',
    DeviceID: direct.DeviceID || route?.DeviceID || '',
    ConnectorID: direct.ConnectorID || route?.ConnectorID || '',
    AddressID: direct.AddressID || route?.AddressID || '',
  };
}

export function optionList<T extends { ID: string }>(items: T[], label: (item: T) => string, detail?: (item: T) => string): OptionItem[] {
  return items.map((item) => ({ id: item.ID, label: label(item), detail: detail?.(item) }));
}

export function sourceGuardSummary(address?: TapxAddressLimit): { ips: string; macs: string } {
  if (!address) return { ips: '-', macs: '-' };
  const ips = [...(address.IPv4CIDRs || []), ...(address.IPv6CIDRs || [])].filter(Boolean).join(', ');
  const macs = (address.MACs || []).filter(Boolean).join(', ');
  return { ips: ips || '-', macs: macs || '-' };
}

export function splitList(value: string): string[] {
  return value
    .split(/[\n,，\s]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function joinList(value?: string[]): string {
  return (value || []).join('\n');
}

export function nextId(prefix: string, existing: Set<string>): string {
  let index = existing.size + 1;
  let id = `${prefix}-${index}`;
  while (existing.has(id)) {
    index += 1;
    id = `${prefix}-${index}`;
  }
  return id;
}

function byId<T extends { ID: string }>(items: T[]): Map<string, T> {
  return new Map(items.filter((item) => item.ID).map((item) => [item.ID, item]));
}

function first(...values: Array<string | undefined>): string {
  return values.find((value) => value && value.trim()) || '-';
}
