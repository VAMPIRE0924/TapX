import { describe, expect, it } from 'vitest';
import { connectorIDConflicts, mergeConnectorJson } from './connectorJson';

describe('connector JSON save boundary', () => {
  it('accepts only an object and merges it over the current draft', () => {
    expect(mergeConnectorJson({ ID: 'a', RuntimeMode: 'embedded-xray' }, '{"RuntimeMode":"external-xray"}')).toEqual({
      ID: 'a',
      RuntimeMode: 'external-xray',
    });
    expect(() => mergeConnectorJson({ ID: 'a' }, '[]')).toThrow('must be an object');
  });

  it('allows the current id but rejects another connector id', () => {
    expect(connectorIDConflicts('a', 'a', ['a', 'b'])).toBe(false);
    expect(connectorIDConflicts('b', 'a', ['a', 'b'])).toBe(true);
  });
});
