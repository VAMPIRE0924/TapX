import { describe, expect, it } from 'vitest';
import { hashFromPath } from './hash-route';

describe('hash route parsing', () => {
  it('returns the complete hash or an empty fallback', () => {
    expect(hashFromPath('/settings#certificate')).toBe('#certificate');
    expect(hashFromPath('/kernels')).toBe('');
  });
});
