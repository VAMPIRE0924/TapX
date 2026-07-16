import { describe, expect, it } from 'vitest';
import { formatBytes, formatSeconds, formatSpeed } from './format';

describe('display formatting', () => {
  it('uses consistent readable precision for byte values', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(1536)).toBe('1.50 KB');
    expect(formatBytes(12.46 * 1024)).toBe('12.5 KB');
    expect(formatBytes(128 * 1024)).toBe('128 KB');
    expect(formatSpeed(1536)).toBe('1.50 KB/s');
  });

  it('formats elapsed time without fractional units', () => {
    expect(formatSeconds(0)).toBe('0s');
    expect(formatSeconds(90)).toBe('1m');
    expect(formatSeconds(90061)).toBe('1d 1h');
  });
});
