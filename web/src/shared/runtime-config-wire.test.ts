import { describe, expect, it } from 'vitest';
import { runtimeConfigForAPI } from './runtime-config-wire';

describe('runtime config API projection', () => {
  it('removes endpoint and binding editor fields while preserving runtime fields', () => {
    const projected = runtimeConfigForAPI({
      Listeners: [{
        ID: 'listener-1', Transport: 'tcp', RuntimeMode: 'tapx', Protocol: 'raw-tcp', Security: 'tls',
        Binding: { DeviceID: 'device-1', DeviceBindMode: 'existing', DeviceName: 'tap0' },
        RawTCP: { LengthMode: 'uint32', TLS: { Enabled: true, CertFile: '/cert.pem' } },
      } as never],
      Connectors: [{ ID: 'connector-1', Transport: 'udp', VKey: 'secret', Binding: { VKeyID: 'vkey-1' } } as never],
      Clients: [{ ID: 'client-1', VKey: 'secret', Binding: { VKeyID: 'vkey-1' } } as never],
    });

    expect(projected.Listeners?.[0]).not.toHaveProperty('RuntimeMode');
    expect(projected.Listeners?.[0]).not.toHaveProperty('Protocol');
    expect(projected.Listeners?.[0].Binding).toEqual({ DeviceID: 'device-1' });
    expect(projected.Listeners?.[0].RawTCP).toMatchObject({ LengthMode: 'uint32', TLS: { Enabled: true, CertFile: '/cert.pem' } });
    expect(projected.Connectors?.[0]).not.toHaveProperty('VKey');
    expect(projected.Clients?.[0]).not.toHaveProperty('VKey');
  });

  it('removes device form helpers and stale relationship snapshots', () => {
    const projected = runtimeConfigForAPI({
      Devices: [{
        ID: 'device-1', Type: 'tun', BridgeName: 'ui-only', LinkedListenerIDs: ['listener-1'],
        TUNDHCP: { Mode: 'server', PoolStart: '10.0.0.2', PoolEnd: '10.0.0.254' },
      } as never],
    });

    expect(projected.Devices?.[0]).not.toHaveProperty('BridgeName');
    expect(projected.Devices?.[0]).not.toHaveProperty('LinkedListenerIDs');
    expect(projected.Devices?.[0].TUNDHCP).toMatchObject({ Mode: 'server', PoolStart: '10.0.0.2' });
  });
});
