import { describe, expect, it } from 'vitest';
import { normalizeInterfaceNames, uniqueDeviceID } from './DevicePage';

describe('device interface helpers', () => {
  it('reads the panel API response envelope', () => {
    expect(normalizeInterfaceNames({
      success: true,
      obj: [
        { name: 'lan5', up: false },
        { name: 'br-lan', up: true },
      ],
    })).toEqual(['br-lan', 'lan5']);
  });

  it('keeps compatibility with direct arrays and removes empty duplicates', () => {
    expect(normalizeInterfaceNames([
      ' eth0 ',
      { Name: 'eth0' },
      { IfName: 'tap0' },
      { name: '' },
    ])).toEqual(['eth0', 'tap0']);
  });

  it('does not reuse a hidden device ID after the original interface was renamed', () => {
    const devices = [
      { ID: 'dev-tapx-tun1', IfName: 'regtun26', ManagedNodeID: 'local' },
      { ID: 'dev-regtap26', IfName: 'regtap26-old', ManagedNodeID: 'local' },
      { ID: 'dev-regtap26-2', IfName: 'regtap26-older', ManagedNodeID: 'local' },
    ];
    expect(uniqueDeviceID(devices, 'dev-regtap26', 'local')).toBe('dev-regtap26-3');
    expect(uniqueDeviceID(devices, 'dev-regtap26', 'remote-node')).toBe('dev-regtap26');
  });
});
