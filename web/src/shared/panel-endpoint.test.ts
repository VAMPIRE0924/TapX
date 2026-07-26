import { describe, expect, it } from 'vitest';
import { panelRestartURL } from './panel-endpoint';

describe('panelRestartURL', () => {
  it('uses the saved port and path while keeping the reachable browser host', () => {
    expect(panelRestartURL(
      { listenPort: 24443, uriPath: '/tapx-lab/', certPublicPath: '/cert.pem', certPrivatePath: '/key.pem' },
      { protocol: 'http:', hostname: '118.25.47.217' },
    )).toBe('https://118.25.47.217:24443/tapx-lab/');
  });

  it('uses the configured host restriction and omits the default port', () => {
    expect(panelRestartURL(
      { listenDomain: 'panel.example.com', listenPort: 443, uriPath: 'secret', panelHTTPS: true },
      { protocol: 'http:', hostname: '10.10.0.255' },
    )).toBe('https://panel.example.com/secret/');
  });
});
