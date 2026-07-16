import { describe, expect, it } from 'vitest';
import { resolveTcpLengthMode } from './tcpLengthMode';

describe('Raw TCP length mode', () => {
  it('defaults to the uint16 framing required by the TapX raw TCP protocol', () => {
    expect(resolveTcpLengthMode({})).toBe('uint16');
  });

  it('preserves explicit modes and stored runtime values', () => {
    expect(resolveTcpLengthMode({ mode: 'uint32', stored: 'uint16' })).toBe('uint32');
    expect(resolveTcpLengthMode({ stored: 'uint32' })).toBe('uint32');
  });

  it('migrates the old boolean control without changing existing behavior', () => {
    expect(resolveTcpLengthMode({ legacyPrefix: true })).toBe('uint32');
    expect(resolveTcpLengthMode({ legacyPrefix: false })).toBe('uint16');
  });
});
