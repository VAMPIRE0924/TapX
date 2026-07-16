import { describe, expect, it } from 'vitest';
import { safeID, uniqueID } from './ids';
import { booleanValue, numberValue, stringValue } from './values';

describe('wire-value helpers', () => {
  it('uses values only when their wire type is valid', () => {
    expect(numberValue(0, 9)).toBe(0);
    expect(numberValue(Number.NaN, 9)).toBe(9);
    expect(stringValue('', 'fallback')).toBe('');
    expect(stringValue(3, 'fallback')).toBe('fallback');
    expect(booleanValue(false, true)).toBe(false);
    expect(booleanValue('true', true)).toBe(true);
  });

  it('normalizes and de-duplicates imported IDs', () => {
    expect(safeID('  node / hk  ')).toBe('node-hk');
    expect(safeID('***')).toBe('-');
    expect(safeID('')).toBe('item');
    expect(uniqueID('node', new Set(['node', 'node-2', 'node-3']))).toBe('node-4');
  });
});
