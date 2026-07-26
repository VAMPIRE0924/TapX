import { describe, expect, it } from 'vitest';
import { objectMatchesSearch } from './object-search';

describe('objectMatchesSearch', () => {
  const record = {
    ID: 'connector-public',
    Name: 'Hong Kong Listener',
    Binding: { DeviceID: 'tapx-tun0' },
    Addresses: ['10.20.0.1/30'],
  };

  it('matches nested fields case-insensitively', () => {
    expect(objectMatchesSearch(record, 'HONG KONG')).toBe(true);
    expect(objectMatchesSearch(record, 'tapx-tun0')).toBe(true);
    expect(objectMatchesSearch(record, '10.20.0.1')).toBe(true);
  });

  it('matches supplied rendered values and handles empty searches', () => {
    expect(objectMatchesSearch(record, 'external-xray', ['External-Xray'])).toBe(true);
    expect(objectMatchesSearch(record, '   ')).toBe(true);
    expect(objectMatchesSearch(record, 'missing')).toBe(false);
  });
});
