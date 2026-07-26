import { describe, expect, it } from 'vitest';
import { DeviceTypeConflictError, hydrateSavedDeviceBinding, materializeEndpointAutoDevice, normalizeDeviceBinding } from './deviceBinding';

describe('endpoint device binding', () => {
  it('normalizes existing-device bindings without carrying auto-create address fields', () => {
    expect(normalizeDeviceBinding({ DeviceBindingEnabled: true, DeviceBindMode: 'existing', AddressConfigEnabled: true }, { mode: 'autoCreate', addressMode: 'auto' })).toMatchObject({
      DeviceBindMode: 'existing',
      AutoCreateDevice: false,
      AddressConfigEnabled: false,
    });
    expect(normalizeDeviceBinding({ DeviceBindingEnabled: true, DeviceBindMode: 'autoCreate', AddressAssignMode: 'manual' }, { mode: 'autoCreate', addressMode: 'auto' }))
      .toMatchObject({ AddressAssignMode: 'manual' });
    expect(normalizeDeviceBinding({ DeviceBindingEnabled: true, DeviceBindMode: 'autoCreate', LinkAutoOptimize: true, MSSClamp: 1360 }, { mode: 'autoCreate', addressMode: 'auto' }))
      .toMatchObject({ LinkAutoOptimize: true, MSSClamp: 0 });
  });

  it('keeps non-device bindings and skips device creation when device binding is disabled', () => {
    const result = materializeEndpointAutoDevice({
      ID: 'listener-1',
      Binding: {
        DeviceBindingEnabled: false,
        DeviceBindMode: 'autoCreate',
        DeviceName: 'tapx-tun0',
        DeviceID: 'device-old',
        VKeyID: 'vkey-1',
      },
    }, [], { role: 'listener', defaultMode: 'existing', defaultAddressMode: 'manual' });
    expect(result.devices).toEqual([]);
    expect(result.endpoint.Binding).toEqual({ VKeyID: 'vkey-1' });
  });

  it('materializes a listener device and rewrites the endpoint binding', () => {
    const result = materializeEndpointAutoDevice({
      ID: 'listener-1',
      Name: 'edge',
      Binding: { DeviceBindMode: 'autoCreate', DeviceName: 'tapx-tun0', InterfaceType: 'tun', MTU: 1400 },
    }, [], { role: 'listener', defaultMode: 'existing', defaultAddressMode: 'manual' });
    expect(result.endpoint.Binding).toEqual({ DeviceID: 'device-tapx-tun0' });
    expect(result.devices[0]).toMatchObject({
      ID: 'device-tapx-tun0', Source: 'listener-auto',
    });
  });

  it('persists an existing-device binding without UI-only fields', () => {
    const result = materializeEndpointAutoDevice({
      ID: 'listener-1',
      Binding: {
        DeviceBindingEnabled: true,
        DeviceBindMode: 'existing',
        DeviceID: 'device-1',
        InterfaceType: 'tap',
        DeviceName: 'tap0',
        MTU: 1500,
        VKeyID: 'vkey-1',
      },
    }, [{ ID: 'device-1', Type: 'tap', IfName: 'tap0' }], {
      role: 'listener', defaultMode: 'existing', defaultAddressMode: 'manual',
    });

    expect(result.endpoint.Binding).toEqual({ DeviceID: 'device-1', VKeyID: 'vkey-1' });
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
      ID: 'device-1', Type: 'tap', IfName: 'tap0', MTU: 1500,
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
      DHCP: { Mode: 'server', DNS: ['1.1.1.1'] },
      Bridge: { Enabled: true, Name: 'br0', IfName: 'eth0' },
    }], { role: 'connector', defaultMode: 'autoCreate', defaultAddressMode: 'auto' });

    expect(result.devices[0]).toMatchObject({
      ID: 'device-1', Type: 'tap', IfName: 'tap0', MTU: 9000,
    });
    expect(result.devices[0].DHCP).toEqual({ Mode: 'server', DNS: ['1.1.1.1'] });
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
