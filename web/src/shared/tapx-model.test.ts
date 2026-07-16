import { describe, expect, it } from 'vitest';
import { buildIndex, resolveBinding } from './tapx-model';

describe('TapX binding resolution', () => {
  it('fills missing direct fields from the referenced route', () => {
    const idx = buildIndex({
      Routes: [{
        ID: 'route-a',
        VKeyID: 'vkey-a',
        ClientID: 'user-a',
        DeviceID: 'tap-a',
        ConnectorID: 'connector-a',
        AddressID: 'address-a',
      }],
    });

    expect(resolveBinding({ RouteID: 'route-a' }, idx)).toEqual({
      RouteID: 'route-a',
      VKeyID: 'vkey-a',
      ClientID: 'user-a',
      DeviceID: 'tap-a',
      ConnectorID: 'connector-a',
      AddressID: 'address-a',
    });
  });

  it('keeps direct values ahead of route values', () => {
    const idx = buildIndex({
      Routes: [{ ID: 'route-a', DeviceID: 'route-device', AddressID: 'route-address' }],
    });

    expect(resolveBinding({
      RouteID: 'route-a',
      DeviceID: 'direct-device',
      AddressID: 'direct-address',
    }, idx)).toMatchObject({
      DeviceID: 'direct-device',
      AddressID: 'direct-address',
    });
  });

  it('preserves direct fields when the route is missing', () => {
    const idx = buildIndex({});
    expect(resolveBinding({ RouteID: 'missing', DeviceID: 'tap-a' }, idx)).toMatchObject({
      RouteID: 'missing',
      DeviceID: 'tap-a',
    });
  });
});
