import { describe, expect, it } from 'vitest';
import { exportUserBundle, importUserBundle, sanitizeUserCredentials } from './userTransfer';

describe('user transfer bundle', () => {
  it('exports referenced address limits and vkeys', () => {
    const client = { ID: 'user-a', AddressID: 'addr-a', Binding: { AddressID: 'addr-a', VKeyID: 'vk-a' } };
    const bundle = exportUserBundle([client], {
      Clients: [client],
      Addresses: [{ ID: 'addr-a' }, { ID: 'addr-other' }],
      VKeys: [{ ID: 'vk-a' }, { ID: 'vk-other' }],
    });
    expect(bundle.Addresses.map((item) => item.ID)).toEqual(['addr-a']);
    expect(bundle.VKeys.map((item) => item.ID)).toEqual(['vk-a']);
  });

  it('remaps colliding dependency ids together with client references', () => {
    const result = importUserBundle(JSON.stringify({
      Clients: [{ ID: 'user-b', Email: 'b@tapx', AddressID: 'addr-a', Binding: { AddressID: 'addr-a', VKeyID: 'vk-a' } }],
      Addresses: [{ ID: 'addr-a', ClientID: 'user-b' }],
      VKeys: [{ ID: 'vk-a', Value: 'secret' }],
    }), { Addresses: [{ ID: 'addr-a' }], VKeys: [{ ID: 'vk-a' }] });
    const imported = result.clients[0];
    expect(imported.AddressID).toBe('addr-a-2');
    expect(imported.Binding?.VKeyID).toBe('vk-a-2');
    expect(result.addresses.at(-1)?.ClientID).toBe('user-b');
  });

  it('skips duplicate client ids or emails', () => {
    const result = importUserBundle(JSON.stringify([{ ID: 'user-a', Email: 'same@tapx' }, { ID: 'user-b', Email: 'same@tapx' }]), {
      Clients: [{ ID: 'user-a', Email: 'existing@tapx' }],
    });
    expect(result.skipped).toBe(1);
    expect(result.clients.map((item) => item.ID)).toEqual(['user-a', 'user-b']);
  });

  it('keeps same IDs independent when importing into another node', () => {
    const result = importUserBundle(JSON.stringify({
      Clients: [{ ID: 'user-a', Email: 'same@tapx', AddressID: 'addr-a', Binding: { AddressID: 'addr-a', VKeyID: 'vk-a' } }],
      Addresses: [{ ID: 'addr-a' }],
      VKeys: [{ ID: 'vk-a', Value: 'remote-secret' }],
    }), {
      Clients: [{ ID: 'user-a', Email: 'same@tapx' }],
      Addresses: [{ ID: 'addr-a' }],
      VKeys: [{ ID: 'vk-a', Value: 'local-secret' }],
    }, 'node-edge');

    expect(result.skipped).toBe(0);
    expect(result.clients.at(-1)).toMatchObject({ ID: 'user-a', ManagedNodeID: 'node-edge', AddressID: 'addr-a' });
    expect(result.addresses.at(-1)).toMatchObject({ ID: 'addr-a', ManagedNodeID: 'node-edge', ClientID: 'user-a' });
    expect(result.vkeys.at(-1)).toMatchObject({ ID: 'vk-a', ManagedNodeID: 'node-edge', Value: 'remote-secret' });
  });

  it('exports dependencies from the client source node only', () => {
    const remoteClient = { ID: 'user-a', ManagedNodeID: 'node-edge', AddressID: 'addr-a', Binding: { AddressID: 'addr-a', VKeyID: 'vk-a' } };
    const bundle = exportUserBundle([remoteClient], {
      Addresses: [{ ID: 'addr-a', Name: 'local' }, { ID: 'addr-a', Name: 'remote', ManagedNodeID: 'node-edge' } as never],
      VKeys: [{ ID: 'vk-a', Name: 'local' }, { ID: 'vk-a', Name: 'remote', ManagedNodeID: 'node-edge' } as never],
    });
    expect(bundle.Addresses).toHaveLength(1);
    expect(bundle.Addresses[0]).toMatchObject({ Name: 'remote', ManagedNodeID: 'node-edge' });
    expect(bundle.VKeys).toHaveLength(1);
    expect(bundle.VKeys[0]).toMatchObject({ Name: 'remote', ManagedNodeID: 'node-edge' });
  });

  it('removes unsupported user credential fields at every transfer boundary', () => {
    const legacy = {
      ID: 'legacy',
      Security: 'auto',
      ReverseTag: 'reverse-old',
      Flow: 'xtls-rprx-vision',
      WireguardPrivateKey: 'private',
      WireguardPublicKey: 'public',
      WireguardPreSharedKey: 'psk',
      WireguardAllowedIPs: ['10.0.0.2/32'],
      UUID: 'uuid-a',
    };
    expect(sanitizeUserCredentials(legacy)).toEqual({ ID: 'legacy', UUID: 'uuid-a' });
    expect(exportUserBundle([legacy], {}).Clients).toEqual([{ ID: 'legacy', UUID: 'uuid-a' }]);
    expect(importUserBundle(JSON.stringify([legacy]), {}).clients).toEqual([{ ID: 'legacy', UUID: 'uuid-a' }]);
  });
});
