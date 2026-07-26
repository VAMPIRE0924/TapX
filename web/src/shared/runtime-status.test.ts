import { describe, expect, it } from 'vitest';
import { activeExternalXrayPipes, activeRawTCPPipes, activeTapXPipeCount } from './runtime-status';

describe('runtime component status helpers', () => {
  it('does not report TapX running when only external Xray bridges remain', () => {
    const runtime = {
      udpPipes: [],
      tcpPipes: [
        { xrayRuntime: 'external' },
        { xrayRuntime: 'external', inactive: true },
      ],
    };

    expect(activeTapXPipeCount(runtime)).toBe(0);
    expect(activeExternalXrayPipes(runtime)).toHaveLength(1);
  });

  it('counts only active raw TCP and all active runtime UDP pipes for TapX', () => {
    const runtime = {
      udpPipes: [{ transport: 'udp' }],
      tcpPipes: [
        { transport: 'tcp' },
        { transport: 'tcp', inactive: true },
        { transport: 'xray', xrayRuntime: 'external' },
      ],
    };

    expect(activeRawTCPPipes(runtime)).toHaveLength(1);
    expect(activeTapXPipeCount(runtime)).toBe(2);
  });
});
