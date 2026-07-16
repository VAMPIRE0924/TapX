import { describe, expect, it } from 'vitest';
import type { RuntimeConfig } from '../../shared/api';
import { buildLinkTestRows, filterLinkTestRows } from './linkDiagnostics';

function routeConfig(deviceType: 'tun' | 'tap'): RuntimeConfig {
  return {
    Devices: [{ ID: 'device-a', Type: deviceType, Name: 'device-a' }],
    VKeys: [{ ID: 'vkey-a', Name: 'office-key', Value: 'secret-vkey' }],
    Addresses: [{
      ID: 'address-a',
      IPv4CIDRs: ['10.8.0.0/24'],
      IPv6CIDRs: ['fd00::/64'],
      MACs: ['02:00:00:00:00:01'],
    }],
    Connectors: [{ ID: 'connector-a', Name: 'edge-a', Remote: '198.51.100.10', Port: 46000 }],
    Routes: [{
      ID: 'route-a',
      Enabled: true,
      VKeyID: 'vkey-a',
      DeviceID: 'device-a',
      ConnectorID: 'connector-a',
      AddressID: 'address-a',
    }],
    Listeners: [{
      ID: 'listener-a',
      Name: 'listener-a',
      BindPort: 45000,
      Binding: { RouteID: 'route-a' },
    }],
    Clients: [{ ID: 'user-a', Name: 'user-a', ListenerID: 'listener-a' }],
  };
}

describe('link diagnostics', () => {
  it('shows route-inherited listener and user relationships', () => {
    const rows = buildLinkTestRows(routeConfig('tap'));
    const listener = rows.find((row) => row.key === 'listener-listener-a');
    const user = rows.find((row) => row.key === 'user-user-a-listener-a-device-a');

    expect(listener).toMatchObject({
      vkey: 'office-key',
      connector: 'edge-a:46000',
      device: 'device-a (TAP)',
      allowedIPs: '10.8.0.0/24, fd00::/64',
      allowedMACs: '02:00:00:00:00:01',
    });
    expect(user).toMatchObject({
      vkey: 'office-key',
      connector: 'edge-a:46000',
      device: 'device-a (TAP)',
      allowedIPs: '10.8.0.0/24, fd00::/64',
    });
  });

  it('hides MAC limits for TUN while preserving IP limits', () => {
    const listener = buildLinkTestRows(routeConfig('tun')).find((row) => row.key === 'listener-listener-a');
    expect(listener).toMatchObject({ allowedIPs: '10.8.0.0/24, fd00::/64', allowedMACs: '' });
  });

  it('finds inherited bindings by the vKey value without displaying the secret', () => {
    const rows = buildLinkTestRows(routeConfig('tap'));
    const matches = filterLinkTestRows(rows, 'vkey', 'secret-vkey');
    expect(matches.map((row) => row.key)).toContain('listener-listener-a');
    expect(matches.find((row) => row.key === 'listener-listener-a')?.vkey).toBe('office-key');
  });

  it('keeps direct endpoint binding values ahead of the route', () => {
    const config = routeConfig('tap');
    config.Devices?.push({ ID: 'device-direct', Type: 'tap', Name: 'device-direct' });
    config.Listeners![0].Binding = { RouteID: 'route-a', DeviceID: 'device-direct' };
    const listener = buildLinkTestRows(config).find((row) => row.key === 'listener-listener-a');
    expect(listener?.device).toBe('device-direct (TAP)');
  });
});
