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

  it('does not infer protocols from credentials without an attached listener', () => {
    expect(userProtocols({ ID: 'detached' }, [], [])).toEqual([]);
    expect(userProtocols(
      { ID: 'mixed', ListenerID: 'raw' },
      [{ ID: 'raw', Transport: 'tcp' }],
      [],
    )).toEqual(['raw-tcp']);
  });
});
