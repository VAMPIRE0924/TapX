import { describe, expect, it } from 'vitest';
import { stripTapxSocketOverrides } from './tapxRawSettings';

describe('TapX raw settings boundary', () => {
  it('removes unapproved socket overrides and keeps approved fast-path controls', () => {
    expect(stripTapxSocketOverrides({
      PeerMode: 'learn',
      BindInterface: 'eth0',
      ReceiveBuffer: 65536,
      ReusePort: true,
      Workers: 4,
      QueueSize: 2048,
      ZeroCopy: true,
      LengthMode: 'uint16',
    })).toEqual({ Workers: 4, QueueSize: 2048, ZeroCopy: true, LengthMode: 'uint16' });
  });
});
