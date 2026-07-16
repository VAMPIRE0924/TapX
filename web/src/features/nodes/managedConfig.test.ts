import { describe, expect, it } from 'vitest';
import type { RuntimeConfig } from '../../shared/api';
import { mergeManagedStats, nodeObjectKey, propagateManagedNodeOwnership, sameNodeObject } from './managedConfig';

describe('managed node configuration', () => {
  it('propagates endpoint ownership to generated related objects', () => {
    const config: RuntimeConfig = {
      Listeners: [{
        ID: 'listener-1',
        ManagedNodeID: 'node-edge',
        Binding: { DeviceID: 'device-1', VKeyID: 'vkey-1' },
        XrayProfileID: 'profile-1',
      } as never],
      Devices: [{ ID: 'device-1' }],
      VKeys: [{ ID: 'vkey-1' }],
      XrayProfiles: [{ ID: 'profile-1' }],
    };

    const result = propagateManagedNodeOwnership(config);
    expect(result.Devices?.[0]).toMatchObject({ ManagedNodeID: 'node-edge' });
    expect(result.VKeys?.[0]).toMatchObject({ ManagedNodeID: 'node-edge' });
    expect(result.XrayProfiles?.[0]).toMatchObject({ ManagedNodeID: 'node-edge' });
    expect(config.Devices?.[0]).not.toHaveProperty('ManagedNodeID');
  });

  it('propagates listener ownership through users, limits and routes', () => {
    const config: RuntimeConfig = {
      Listeners: [{ ID: 'listener-1', ManagedNodeID: 'node-edge' } as never],
      Clients: [{ ID: 'user-1', ListenerID: 'listener-1', AddressID: 'address-1' }],
      Addresses: [{ ID: 'address-1', ClientID: 'user-1' }],
      Routes: [{ ID: 'route-1', ClientID: 'user-1', AddressID: 'address-1' }],
    };

    const result = propagateManagedNodeOwnership(config);
    expect(result.Clients?.[0]).toMatchObject({ ManagedNodeID: 'node-edge' });
    expect(result.Addresses?.[0]).toMatchObject({ ManagedNodeID: 'node-edge' });
    expect(result.Routes?.[0]).toMatchObject({ ManagedNodeID: 'node-edge' });
  });

  it('uses node and object id together as the managed identity', () => {
    const local = { ID: 'route-1', ManagedNodeID: 'local' };
    const remote = { ID: 'route-1', ManagedNodeID: 'node-edge' };
    expect(nodeObjectKey(local)).toBe('local:route-1');
    expect(sameNodeObject(local, remote)).toBe(false);
  });

  it('never assigns a generated relation to a same-id object on another node', () => {
    const config: RuntimeConfig = {
      Listeners: [{
        ID: 'listener-1', ManagedNodeID: 'node-edge', Binding: { DeviceID: 'device-shared' },
      } as never],
      Devices: [
        { ID: 'device-shared', ManagedNodeID: 'local', Name: 'local-device' } as never,
        { ID: 'device-shared', Name: 'generated-remote' },
      ],
    };

    const result = propagateManagedNodeOwnership(config);
    expect(result.Devices?.[0]).toMatchObject({ ManagedNodeID: 'local', Name: 'local-device' });
    expect(result.Devices?.[1]).toMatchObject({ ManagedNodeID: 'node-edge', Name: 'generated-remote' });
  });

  it('keeps statistics from same-id objects isolated by node', () => {
    const result = mergeManagedStats([
      {
        nodeID: 'local',
        report: {
          totals: { rxBytes: 10, txBytes: 20 },
          byEndpoint: [{ id: 'connector:same', name: 'same', kind: 'connector', counters: { rxBytes: 10 } }],
          clients: [{ id: 'same', counters: { rxBytes: 4 } }],
        },
      },
      {
        nodeID: 'node-edge',
        report: {
          totals: { rxBytes: 30, txBytes: 40 },
          byEndpoint: [{ id: 'connector:same', name: 'same', kind: 'connector', counters: { rxBytes: 30 } }],
          clients: [{ id: 'same', counters: { rxBytes: 8 } }],
        },
      },
    ]);

    expect(result.totals).toMatchObject({ rxBytes: 40, txBytes: 60 });
    expect(result.byEndpoint?.map((item) => `${item.ManagedNodeID}:${item.name}`)).toEqual([
      'local:same',
      'node-edge:same',
    ]);
    expect(result.clients?.map((item) => `${item.ManagedNodeID}:${item.id}`)).toEqual([
      'local:same',
      'node-edge:same',
    ]);
  });
});
