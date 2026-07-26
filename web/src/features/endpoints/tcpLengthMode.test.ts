import { describe, expect, it } from 'vitest';
import { resolveTcpLengthMode } from './tcpLengthMode';

describe('Raw TCP length mode', () => {
  it('defaults to the uint16 framing required by the TapX raw TCP protocol', () => {
    expect(resolveTcpLengthMode({})).toBe('uint16');
  });

  it('preserves explicit modes', () => {
    expect(resolveTcpLengthMode({ mode: 'uint32' })).toBe('uint32');
    expect(resolveTcpLengthMode({ mode: 'uint16' })).toBe('uint16');
  });
});
