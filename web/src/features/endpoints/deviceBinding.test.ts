import { describe, expect, it } from 'vitest';
import { DeviceTypeConflictError, hydrateSavedDeviceBinding, materializeEndpointAutoDevice, normalizeDeviceBinding } from './deviceBinding';

describe('endpoint device binding', () => {
  it('normalizes existing-device bindings without carrying auto-create address fields', () => {
    expect(normalizeDeviceBinding({ DeviceBindMode: 'existing', AddressConfigEnabled: true }, { mode: 'autoCreate', addressMode: 'auto' })).toMatchObject({
      DeviceBindMode: 'existing',
      AutoCreateDevice: false,
      AddressConfigEnabled: false,
    });
    expect(normalizeDeviceBinding({ DeviceBindMode: 'autoCreate', AddressAssignMode: 'manual' }, { mode: 'autoCreate', addressMode: 'auto' }))
      .toMatchObject({ AddressAssignMode: 'manual' });
    expect(normalizeDeviceBinding({ DeviceBindMode: 'autoCreate', LinkAutoOptimize: true, MSSClamp: 1360 }, { mode: 'autoCreate', addressMode: 'auto' }))
      .toMatchObject({ LinkAutoOptimize: true, MSSClamp: 0 });
  });

  it('materializes a listener device and rewrites the endpoint binding', () => {
    const result = materializeEndpointAutoDevice({
      ID: 'listener-1',
      Name: 'edge',
      Binding: { DeviceBindMode: 'autoCreate', DeviceName: 'tapx-tun0', InterfaceType: 'tun', MTU: 1400 },
    }, [], { role: 'listener', defaultMode: 'existing', defaultAddressMode: 'manual', now: 123 });
    expect(result.endpoint.Binding).toMatchObject({ DeviceID: 'device-tapx-tun0', DeviceBindMode: 'existing', AutoCreateDevice: false });
    expect(result.devices[0]).toMatchObject({
      ID: 'device-tapx-tun0', Source: 'listener-auto', LinkedListenerIDs: ['listener-1'], LinkedListenerNames: ['edge'], UpdatedAt: 123,
    });
  });

  it('materializes automatic link optimization as a device-owned setting', () => {
    const result = materializeEndpointAutoDevice({
      ID: 'listener-1',
      Binding: {
        DeviceBindMode: 'autoCreate', DeviceName: 'tapx-tun0', InterfaceType: 'tun',
        MTU: 1500, MSSClamp: 1360, LinkAutoOptimize: true,
      },
    }, [], { role: 'listener', defaultMode: 'existing', defaultAddressMode: 'manual' });
    expect(result.devices[0]).toMatchObject({ LinkAutoOptimize: true, MSSClamp: 0, MTU: 1500 });
  });

  it('hydrates an existing device without changing its identity', () => {
    expect(hydrateSavedDeviceBinding({ DeviceID: 'device-1' }, {
      ID: 'device-1', Type: 'tap', IfName: 'tap0', MTU: 1500, AddressAssignMode: 'manual', IPv4CIDR: '10.0.0.1/24',
    })).toMatchObject({ DeviceID: 'device-1', DeviceBindMode: 'existing', AutoCreateDevice: false, InterfaceType: 'tap', DeviceName: 'tap0' });
  });

  it('rejects auto-creating an interface over a device of another type', () => {
    expect(() => materializeEndpointAutoDevice({
      ID: 'listener-1',
      Binding: { DeviceBindMode: 'autoCreate', DeviceName: 'edge0', InterfaceType: 'tap' },
    }, [{ ID: 'device-1', Type: 'tun', IfName: 'edge0' }], {
      role: 'listener', defaultMode: 'existing', defaultAddressMode: 'manual',
    })).toThrow(DeviceTypeConflictError);
  });

  it('links an existing device without replacing its device settings', () => {
    const result = materializeEndpointAutoDevice({
      ID: 'connector-1',
      Name: 'remote',
      Binding: {
        DeviceBindMode: 'autoCreate', DeviceName: 'tap0', InterfaceType: 'tap',
        MTU: 1200, IPv4CIDR: '192.0.2.1/24',
      },
    }, [{
      ID: 'device-1', Type: 'tap', IfName: 'tap0', MTU: 9000,
      IPv4CIDR: '10.0.0.1/24', DNS: { Enabled: true, Nameservers: ['1.1.1.1'] },
      Bridge: { Enabled: true, Name: 'br0', IfName: 'eth0' },
    }], { role: 'connector', defaultMode: 'autoCreate', defaultAddressMode: 'auto', now: 456 });

    expect(result.devices[0]).toMatchObject({
      ID: 'device-1', Type: 'tap', IfName: 'tap0', MTU: 9000,
      IPv4CIDR: '10.0.0.1/24', LinkedConnectorIDs: ['connector-1'],
      LinkedConnectorNames: ['remote'], UpdatedAt: 456,
    });
    expect(result.devices[0].DNS).toEqual({ Enabled: true, Nameservers: ['1.1.1.1'] });
    expect(result.devices[0].Bridge).toEqual({ Enabled: true, Name: 'br0', IfName: 'eth0' });
  });

  it('keeps same-name devices isolated by managed node', () => {
    const result = materializeEndpointAutoDevice({
      ID: 'listener-1',
      ManagedNodeID: 'node-edge',
      Binding: { DeviceBindMode: 'autoCreate', DeviceName: 'tap0', InterfaceType: 'tap' },
    } as never, [{ ID: 'device-tap0', Type: 'tun', IfName: 'tap0', ManagedNodeID: 'local' } as never], {
      role: 'listener', defaultMode: 'existing', defaultAddressMode: 'manual',
    });

    expect(result.devices).toHaveLength(2);
    expect(result.devices[0]).toMatchObject({ Type: 'tun', ManagedNodeID: 'local' });
    expect(result.devices[1]).toMatchObject({ Type: 'tap', ManagedNodeID: 'node-edge' });
  });
});
