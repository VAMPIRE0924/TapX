import type { RuntimeConfig, TapxAddressLimit, TapxDevice, TapxRoute } from '../../shared/api';
import {
  buildIndex,
  emptyConfig,
  labelClient,
  labelDevice,
  labelEndpoint,
  labelVKey,
  resolveBinding,
  routeAddress,
  routeDevice,
  sourceGuardSummary,
} from '../../shared/tapx-model';

export type LinkQueryMode = 'device' | 'connector' | 'listener' | 'user' | 'vkey' | 'ip' | 'mac';

export interface LinkTestRow {
  key: string;
  kind: 'connector' | 'listener' | 'user' | 'rule';
  listener?: string;
  user?: string;
  vkey?: string;
  connector?: string;
  device?: string;
  allowedIPs?: string;
  allowedMACs?: string;
  endpoint?: string;
  action?: 'bind-device' | 'allow' | 'drop' | 'disabled';
  vkeyQuery?: string;
}

export function buildLinkTestRows(config: RuntimeConfig): LinkTestRow[] {
  const normalized = emptyConfig(config);
  const idx = buildIndex(config);
  const rows: LinkTestRow[] = [];

  for (const connector of normalized.Connectors) {
    const binding = resolveBinding(connector.Binding, idx);
    const route = binding.RouteID ? idx.routes.get(binding.RouteID) : undefined;
    const device = binding.DeviceID ? idx.devices.get(binding.DeviceID) : undefined;
    const address = binding.AddressID ? idx.addresses.get(binding.AddressID) : undefined;
    const guard = sourceGuardForDevice(address, device);
    rows.push({
      key: `connector-${connector.ID}`,
      kind: 'connector',
      listener: route?.ListenerID ? labelEndpoint(idx.listeners.get(route.ListenerID)) : '',
      user: binding.ClientID ? labelClient(idx.clients.get(binding.ClientID)) : '',
      vkey: binding.VKeyID ? labelVKey(idx.vkeys.get(binding.VKeyID)) : '',
      vkeyQuery: buildVKeyQuery(binding.VKeyID, idx),
      connector: labelEndpoint(connector),
      device: device ? labelDevice(device) : '',
      allowedIPs: guard.ips,
      allowedMACs: guard.macs,
      endpoint: connector.Remote ? `${connector.Remote}:${connector.Port || ''}` : connector.Transport || '',
    });
  }

  for (const listener of normalized.Listeners) {
    const binding = resolveBinding(listener.Binding, idx);
    const device = binding.DeviceID ? idx.devices.get(binding.DeviceID) : undefined;
    const address = binding.AddressID ? idx.addresses.get(binding.AddressID) : undefined;
    const guard = sourceGuardForDevice(address, device);
    rows.push({
      key: `listener-${listener.ID}`,
      kind: 'listener',
      listener: labelEndpoint(listener),
      user: binding.ClientID ? labelClient(idx.clients.get(binding.ClientID)) : '',
      vkey: binding.VKeyID ? labelVKey(idx.vkeys.get(binding.VKeyID)) : '',
      vkeyQuery: buildVKeyQuery(binding.VKeyID, idx),
      connector: binding.ConnectorID ? labelEndpoint(idx.connectors.get(binding.ConnectorID)) : '',
      device: device ? labelDevice(device) : '',
      allowedIPs: guard.ips,
      allowedMACs: guard.macs,
      endpoint: `${listener.BindHost || '0.0.0.0'}:${listener.BindPort || ''}`,
    });
  }

  for (const client of normalized.Clients) {
    const clientBinding = resolveBinding(client.Binding, idx);
    const clientAddress = client.AddressID
      ? idx.addresses.get(client.AddressID)
      : clientBinding.AddressID
        ? idx.addresses.get(clientBinding.AddressID)
        : undefined;
    const listenerIDs = client.ListenerIDs?.length ? client.ListenerIDs : (client.ListenerID ? [client.ListenerID] : ['']);
    for (const listenerID of listenerIDs) {
      const listener = listenerID ? idx.listeners.get(listenerID) : undefined;
      const listenerBinding = resolveBinding(listener?.Binding, idx);
      const address = clientAddress || (listenerBinding.AddressID ? idx.addresses.get(listenerBinding.AddressID) : undefined);
      const deviceIDs = client.AllowedDeviceIDs?.length
        ? client.AllowedDeviceIDs
        : clientBinding.DeviceID
          ? [clientBinding.DeviceID]
          : listenerBinding.DeviceID
            ? [listenerBinding.DeviceID]
            : [''];
      for (const deviceID of deviceIDs) {
        const device = deviceID ? idx.devices.get(deviceID) : undefined;
        const guard = sourceGuardForDevice(address, device);
        rows.push({
          key: `user-${client.ID}-${listenerID || 'none'}-${deviceID || 'none'}`,
          kind: 'user',
          listener: listener ? labelEndpoint(listener) : '',
          user: labelClient(client),
          vkey: client.VKey
            || (clientBinding.VKeyID
              ? labelVKey(idx.vkeys.get(clientBinding.VKeyID))
              : listenerBinding.VKeyID
                ? labelVKey(idx.vkeys.get(listenerBinding.VKeyID))
                : ''),
          vkeyQuery: `${client.VKey || ''} ${buildVKeyQuery(clientBinding.VKeyID, idx)} ${buildVKeyQuery(listenerBinding.VKeyID, idx)}`.trim(),
          connector: clientBinding.ConnectorID
            ? labelEndpoint(idx.connectors.get(clientBinding.ConnectorID))
            : listenerBinding.ConnectorID
              ? labelEndpoint(idx.connectors.get(listenerBinding.ConnectorID))
              : '',
          device: device ? labelDevice(device) : '',
          allowedIPs: guard.ips,
          allowedMACs: guard.macs,
          endpoint: client.CredentialType || '',
        });
      }
    }
  }

  for (const route of normalized.Routes as TapxRoute[]) {
    const address = routeAddress(route, idx);
    const device = routeDevice(route, idx);
    const guard = sourceGuardForDevice(address, device);
    rows.push({
      key: `rule-${route.ID}`,
      kind: 'rule',
      listener: route.ListenerID ? labelEndpoint(idx.listeners.get(route.ListenerID)) : '',
      user: route.ClientID ? labelClient(idx.clients.get(route.ClientID)) : '',
      vkey: route.VKeyID ? labelVKey(idx.vkeys.get(route.VKeyID)) : '',
      vkeyQuery: buildVKeyQuery(route.VKeyID, idx),
      connector: route.ConnectorID ? labelEndpoint(idx.connectors.get(route.ConnectorID)) : '',
      device: device ? labelDevice(device) : '',
      allowedIPs: guard.ips,
      allowedMACs: guard.macs,
      action: route.Enabled === false ? 'disabled' : route.Action || 'bind-device',
    });
  }

  return rows;
}

export function filterLinkTestRows(rows: LinkTestRow[], mode: LinkQueryMode, query: string): LinkTestRow[] {
  const needle = query.trim().toLowerCase();
  if (!needle) return rows;
  return rows.filter((row) => {
    const value = mode === 'device'
      ? row.device
      : mode === 'connector'
        ? `${row.connector || ''} ${row.endpoint || ''}`
        : mode === 'listener'
          ? `${row.listener || ''} ${row.endpoint || ''}`
          : mode === 'user'
            ? row.user
            : mode === 'vkey'
              ? `${row.vkey || ''} ${row.vkeyQuery || ''}`
              : mode === 'ip'
                ? row.allowedIPs
                : row.allowedMACs;
    return (value || '').toLowerCase().includes(needle);
  });
}

function buildVKeyQuery(id: string | undefined, idx: ReturnType<typeof buildIndex>): string {
  if (!id) return '';
  const vkey = idx.vkeys.get(id);
  return `${id} ${vkey?.Name || ''} ${vkey?.Value || ''}`.trim();
}

export function sourceGuardForDevice(address?: TapxAddressLimit, device?: TapxDevice): { ips: string; macs: string } {
  const guard = sourceGuardSummary(address);
  return {
    ips: guard.ips === '-' ? '' : guard.ips,
    macs: device?.Type === 'tun' || guard.macs === '-' ? '' : guard.macs,
  };
}
