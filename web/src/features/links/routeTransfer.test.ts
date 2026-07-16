import { describe, expect, it } from 'vitest';
import type { RuntimeConfig } from '../../shared/api';
import { buildRouteTransferBundle, importRouteTransferBundle } from './routeTransfer';

describe('route transfer bundle', () => {
  it('exports only address limits referenced by exported routes', () => {
    const bundle = buildRouteTransferBundle({
      Routes: [
        { ID: 'route-a', AddressID: 'addr-a' },
        { ID: 'route-b' },
      ],
      Addresses: [
        { ID: 'addr-a', IPv4CIDRs: ['10.0.0.0/24'] },
        { ID: 'addr-unused', IPv4CIDRs: ['192.0.2.0/24'] },
      ],
    });

    expect(bundle).toEqual({
      Version: 1,
      Routes: [
        { ID: 'route-a', AddressID: 'addr-a' },
        { ID: 'route-b' },
      ],
      Addresses: [{ ID: 'addr-a', IPv4CIDRs: ['10.0.0.0/24'], IPv6CIDRs: [], MACs: [] }],
    });
  });

  it('remaps colliding route and address IDs together', () => {
    const current: RuntimeConfig = {
      Routes: [{ ID: 'route-a', AddressID: 'addr-a' }],
      Addresses: [{ ID: 'addr-a', IPv4CIDRs: ['10.0.0.0/24'] }],
    };
    const imported = importRouteTransferBundle({
      Routes: [{ ID: 'route-a', AddressID: 'addr-a', DeviceID: 'tap-a' }],
      Addresses: [{ ID: 'addr-a', IPv4CIDRs: ['10.8.0.0/24'], MACs: ['02:00:00:00:00:01'] }],
    }, current);

    expect(imported.routes).toEqual([
      { ID: 'route-a-2', AddressID: 'addr-a-2', DeviceID: 'tap-a' },
    ]);
    expect(imported.addresses).toEqual([
      { ID: 'addr-a', IPv4CIDRs: ['10.0.0.0/24'] },
      {
        ID: 'addr-a-2',
        IPv4CIDRs: ['10.8.0.0/24'],
        IPv6CIDRs: [],
        MACs: ['02:00:00:00:00:01'],
      },
    ]);
  });

  it('rejects a dangling address-limit reference', () => {
    expect(() => importRouteTransferBundle(
      [{ ID: 'route-a', AddressID: 'missing-address' }],
      {},
    )).toThrow('Link binding route-a references missing address limit missing-address');
  });

  it('keeps same route and address IDs independent on a remote target', () => {
    const imported = importRouteTransferBundle({
      Routes: [{ ID: 'route-a', AddressID: 'addr-a' }],
      Addresses: [{ ID: 'addr-a', IPv4CIDRs: ['10.8.0.0/24'] }],
    }, {
      Routes: [{ ID: 'route-a', AddressID: 'addr-a' }],
      Addresses: [{ ID: 'addr-a', IPv4CIDRs: ['10.0.0.0/24'] }],
    }, 'node-edge');

    expect(imported.routes).toEqual([{ ID: 'route-a', AddressID: 'addr-a', ManagedNodeID: 'node-edge' }]);
    expect(imported.addresses.at(-1)).toMatchObject({
      ID: 'addr-a', ManagedNodeID: 'node-edge', IPv4CIDRs: ['10.8.0.0/24'],
    });
  });
});
