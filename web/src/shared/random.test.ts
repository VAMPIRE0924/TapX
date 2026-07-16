import { describe, expect, it } from 'vitest';
import { randomBase64, randomHex, randomInteger, randomLowerAndNumber, randomShortIds, randomText, randomUUID } from './random';

describe('secure random helpers', () => {
  it('generates values with the requested format', () => {
    expect(randomLowerAndNumber(24)).toMatch(/^[a-z0-9]{24}$/);
    expect(randomHex(16)).toMatch(/^[0-9a-f]{16}$/);
    expect(randomShortIds().map((value) => value.length).sort((a, b) => a - b)).toEqual([2, 4, 6, 8, 10, 12, 14, 16]);
    expect(atob(randomBase64(32))).toHaveLength(32);
    expect(randomUUID()).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
  });

  it('uses custom alphabets and bounded integer ranges', () => {
    expect(randomText(40, 'ab')).toMatch(/^[ab]{40}$/);
    for (let index = 0; index < 20; index += 1) expect(randomInteger(10, 12)).toBeGreaterThanOrEqual(10);
    for (let index = 0; index < 20; index += 1) expect(randomInteger(10, 12)).toBeLessThanOrEqual(12);
  });

  it('rejects invalid input', () => {
    expect(() => randomText(-1, 'ab')).toThrow(RangeError);
    expect(() => randomText(1, 'x')).toThrow(RangeError);
    expect(() => randomInteger(2, 1)).toThrow(RangeError);
  });
});
