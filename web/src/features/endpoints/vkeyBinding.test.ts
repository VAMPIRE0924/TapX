import { describe, expect, it } from 'vitest';
import type { TapxVKey } from '../../shared/api';
import { materializeClientVKey, materializeConnectorVKey, type VKeyReferences } from './vkeyBinding';

const emptyReferences: VKeyReferences = { listeners: [], connectors: [], clients: [], routes: [] };
const vkey = (ID: string, Value: string): TapxVKey => ({ ID, Value, Enabled: true });

describe('connector vKey materialization', () => {
  it('removes an unreferenced managed value when the connector clears it', () => {
    const result = materializeConnectorVKey({ ID: 'c1', RuntimeMode: 'tapx', VKey: '', Binding: { VKeyID: 'old' } }, [vkey('old', 'secret')], emptyReferences);
    expect(result.connector.Binding?.VKeyID).toBe('');
    expect(result.vkeys).toEqual([]);
  });

  it('does not mutate a vKey shared with a user', () => {
    const references: VKeyReferences = { ...emptyReferences, clients: [{ ID: 'u1', Binding: { VKeyID: 'shared' } }] };
    const result = materializeConnectorVKey({ ID: 'c1', Name: 'edge', RuntimeMode: 'tapx', VKey: 'new', Binding: { VKeyID: 'shared' } }, [vkey('shared', 'old')], references);
    expect(result.vkeys.find((item) => item.ID === 'shared')?.Value).toBe('old');
    expect(result.connector.Binding?.VKeyID).not.toBe('shared');
    expect(result.vkeys.find((item) => item.ID === result.connector.Binding?.VKeyID)?.Value).toBe('new');
  });

  it('reuses an existing object with the same value', () => {
    const result = materializeConnectorVKey({ ID: 'c1', RuntimeMode: 'tapx', VKey: 'same' }, [vkey('shared', 'same')], emptyReferences);
    expect(result.connector.Binding?.VKeyID).toBe('shared');
    expect(result.vkeys).toHaveLength(1);
  });

  it('keeps a user-shared vKey unchanged when another user edits its value', () => {
    const references: VKeyReferences = {
      ...emptyReferences,
      clients: [{ ID: 'u1', Binding: { VKeyID: 'shared' } }, { ID: 'u2', Binding: { VKeyID: 'shared' } }],
    };
    const result = materializeClientVKey({ ID: 'u1', Email: 'u1@example.com', VKey: 'new', Binding: { VKeyID: 'shared' } }, [vkey('shared', 'old')], references);
    expect(result.vkeys.find((item) => item.ID === 'shared')?.Value).toBe('old');
    expect(result.client.Binding?.VKeyID).not.toBe('shared');
  });

  it('does not reuse or remove a same-id vKey from another node', () => {
    const local = { ...vkey('shared', 'local-secret'), ManagedNodeID: 'local' } as never;
    const result = materializeConnectorVKey({
      ID: 'c1', ManagedNodeID: 'node-edge', RuntimeMode: 'tapx', VKey: 'remote-secret', Binding: { VKeyID: 'shared' },
    } as never, [local], emptyReferences);

    expect(result.vkeys.find((item) => (item as never as { ManagedNodeID?: string }).ManagedNodeID === 'local')).toEqual(local);
    expect(result.vkeys.find((item) => (item as never as { ManagedNodeID?: string }).ManagedNodeID === 'node-edge')).toMatchObject({
      Value: 'remote-secret',
    });
  });
});
