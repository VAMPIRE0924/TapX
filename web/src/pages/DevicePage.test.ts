import { describe, expect, it } from 'vitest';
import { normalizeInterfaceNames } from './DevicePage';

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
});
