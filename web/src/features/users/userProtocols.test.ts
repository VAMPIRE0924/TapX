import { describe, expect, it } from 'vitest';
import { userProtocols } from './userProtocols';

describe('user protocols', () => {
  it('derives every protocol from the listeners attached to one user', () => {
    expect(userProtocols(
      { ID: 'user-a', ListenerIDs: ['raw', 'xray', 'raw'] },
      [
        { ID: 'raw', Transport: 'udp' },
        { ID: 'xray', Transport: 'xray', XrayProfileID: 'profile-a' },
      ],
      [{ ID: 'profile-a', InboundProtocol: 'wireguard' }],
    )).toEqual(['raw-udp', 'wireguard']);
  });

  it('uses the legacy credential type only when no attached protocol resolves', () => {
    expect(userProtocols({ ID: 'legacy', CredentialType: 'trojan' }, [], [])).toEqual(['trojan']);
    expect(userProtocols(
      { ID: 'mixed', CredentialType: 'vless', ListenerID: 'raw' },
      [{ ID: 'raw', Transport: 'tcp' }],
      [],
    )).toEqual(['raw-tcp']);
  });
});
