import { describe, expect, it } from 'vitest';
import { parseOutboundLink } from './linkParser';

describe('outbound share-link parser', () => {
  it('parses VMess JSON links', () => {
    const payload = btoa(JSON.stringify({
      v: '2',
      ps: 'vmess-edge',
      add: 'vmess.example.com',
      port: '443',
      id: 'bfa0f92f-4ce7-4b6d-8944-f2f808c77f8b',
      scy: 'auto',
      net: 'ws',
      host: 'cdn.example.com',
      path: '/tapx',
      tls: 'tls',
      sni: 'vmess.example.com',
    }));

    expect(parseOutboundLink(`vmess://${payload}`)).toMatchObject({
      protocol: 'vmess',
      name: 'vmess-edge',
      address: 'vmess.example.com',
      port: 443,
      settings: { id: 'bfa0f92f-4ce7-4b6d-8944-f2f808c77f8b' },
      streamSettings: {
        network: 'ws',
        security: 'tls',
        wsSettings: { host: 'cdn.example.com', path: '/tapx' },
        tlsSettings: { serverName: 'vmess.example.com' },
      },
    });
  });

  it('parses VLESS Reality and XHTTP parameters', () => {
    const link = 'vless://uuid-a@edge.example.com:8443?type=xhttp&security=reality&sni=edge.example.com&pbk=public-a&sid=abcd&fp=chrome&path=%2Fapi&mode=packet-up#edge';

    expect(parseOutboundLink(link)).toMatchObject({
      protocol: 'vless',
      name: 'edge',
      address: 'edge.example.com',
      port: 8443,
      settings: { id: 'uuid-a', encryption: 'none' },
      streamSettings: {
        network: 'xhttp',
        security: 'reality',
        xhttpSettings: { path: '/api', mode: 'packet-up' },
        realitySettings: { serverName: 'edge.example.com', publicKey: 'public-a', shortId: 'abcd' },
      },
    });
  });

  it('parses Trojan TLS links', () => {
    expect(parseOutboundLink('trojan://secret@example.com:443?type=grpc&security=tls&sni=example.com&serviceName=tapx#trojan')).toMatchObject({
      protocol: 'trojan',
      name: 'trojan',
      settings: { password: 'secret' },
      streamSettings: {
        network: 'grpc',
        security: 'tls',
        grpcSettings: { serviceName: 'tapx' },
      },
    });
  });

  it('parses Shadowsocks SIP002 links', () => {
    const credentials = btoa('aes-256-gcm:password');
    expect(parseOutboundLink(`ss://${credentials}@ss.example.com:8388#ss-edge`)).toMatchObject({
      protocol: 'shadowsocks',
      name: 'ss-edge',
      address: 'ss.example.com',
      port: 8388,
      settings: { method: 'aes-256-gcm', password: 'password' },
    });
  });

  it('parses Hysteria2 links', () => {
    expect(parseOutboundLink('hysteria2://token@hy.example.com:443?sni=hy.example.com&alpn=h3#hy')).toMatchObject({
      protocol: 'hysteria',
      name: 'hy',
      streamSettings: {
        network: 'hysteria',
        security: 'tls',
        hysteriaSettings: { auth: 'token' },
        tlsSettings: { serverName: 'hy.example.com', alpn: ['h3'] },
      },
    });
  });

  it('parses WireGuard links', () => {
    expect(parseOutboundLink('wireguard://private-key@wg.example.com:51820?publickey=peer-key&address=10.0.0.2%2F32&allowedips=0.0.0.0%2F0&mtu=1380#wg')).toMatchObject({
      protocol: 'wireguard',
      name: 'wg',
      address: 'wg.example.com',
      port: 51820,
      settings: {
        secretKey: 'private-key',
        address: '10.0.0.2/32',
        mtu: 1380,
        peers: [{ publicKey: 'peer-key', endpoint: 'wg.example.com:51820', allowedIPs: ['0.0.0.0/0'] }],
      },
    });
  });

  it('returns undefined for unsupported schemes', () => {
    expect(parseOutboundLink('https://example.com')).toBeUndefined();
  });
});
