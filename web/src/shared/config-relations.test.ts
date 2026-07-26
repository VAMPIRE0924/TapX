import { describe, expect, it } from 'vitest';
import { clearBindingReferences } from './config-relations';

describe('clearBindingReferences', () => {
  it('clears only matching references on the same managed node', () => {
    const records = [
      { ID: 'a', ManagedNodeID: 'local', Binding: { RouteID: 'route-1' } },
      { ID: 'b', ManagedNodeID: 'remote', Binding: { RouteID: 'route-1' } },
      { ID: 'c', ManagedNodeID: 'local', Binding: { RouteID: 'route-2' } },
    ];
    const result = clearBindingReferences(records, 'RouteID', new Set(['local:route-1']));
    expect(result[0].Binding.RouteID).toBe('');
    expect(result[1].Binding.RouteID).toBe('route-1');
    expect(result[2].Binding.RouteID).toBe('route-2');
  });

  it('preserves unrelated binding fields', () => {
    const records = [{ ID: 'a', Binding: { ConnectorID: 'connector-1', DeviceID: 'tap-1' } }];
    const result = clearBindingReferences(records, 'ConnectorID', new Set(['local:connector-1']));
    expect(result[0].Binding).toEqual({ ConnectorID: '', DeviceID: 'tap-1' });
  });
});
