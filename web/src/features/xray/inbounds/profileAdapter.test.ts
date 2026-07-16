import { describe, expect, it } from 'vitest';
import { defaultInboundSettings } from './defaults';
import {
  inboundSettingsFromWire,
  inboundSettingsToWire,
  inboundStreamFromWire,
  inboundStreamToWire,
  restoreInboundFallbacks,
  takeInboundFallbacks,
} from './profileAdapter';

describe('inbound protocol defaults', () => {
  it.each([
    ['vless', 'clients'],
    ['vmess', 'clients'],
    ['trojan', 'clients'],
    ['shadowsocks', 'method'],
    ['hysteria', 'version'],
    ['http', 'accounts'],
    ['mixed', 'auth'],
    ['mtproto', 'secret'],
    ['tunnel', 'allowedNetwork'],
    ['tun', 'name'],
    ['wireguard', 'secretKey'],
  ])('creates %s settings with %s', (protocol, requiredKey) => {
    expect(defaultInboundSettings(protocol)).toHaveProperty(requiredKey);
  });
});

describe('inbound wire adapter', () => {
  it('stores fallbacks separately and restores them', () => {
    const source = {
      clients: [],
      decryption: 'none',
      fallbacks: [{ name: 'edge.example', dest: 8080, xver: 1 }],
    };
    const separated = takeInboundFallbacks(inboundSettingsToWire('vless', source));
    expect(separated.settings).not.toHaveProperty('fallbacks');
    expect(separated.fallbacks).toEqual(source.fallbacks);
    expect(restoreInboundFallbacks(separated.settings, separated.fallbacks)).toEqual(source);
  });

  it('removes XHTTP view-only fields without losing enabled XMUX', () => {
    const wire = inboundStreamToWire({
      network: 'xhttp',
      security: 'none',
      xhttpSettings: {
        path: '/tapx',
        enableXmux: true,
        uplinkChunkSize: 4096,
        xmux: { maxConcurrency: '16-32' },
      },
    });
    expect(wire).toEqual({
      network: 'xhttp',
      security: 'none',
      xhttpSettings: { path: '/tapx', xmux: { maxConcurrency: '16-32' } },
    });
    expect(inboundStreamFromWire(wire)).toMatchObject({
      xhttpSettings: { path: '/tapx', enableXmux: true, xmux: { maxConcurrency: '16-32' } },
    });
  });

  it('keeps TLS certificates while stripping form-only file mode', () => {
    const wire = inboundStreamToWire({
      network: 'tcp',
      security: 'tls',
      tlsSettings: {
        certificates: [{ useFile: true, certificateFile: '/etc/tapx/fullchain.pem', keyFile: '/etc/tapx/key.pem' }],
      },
    });
    expect(wire).toMatchObject({
      tlsSettings: { certificates: [{ certificateFile: '/etc/tapx/fullchain.pem', keyFile: '/etc/tapx/key.pem' }] },
    });
    expect(inboundStreamFromWire(wire)).toMatchObject({
      tlsSettings: { certificates: [{ useFile: true }] },
    });
  });

  it('does not manufacture or persist a WireGuard public-key settings field', () => {
    const secretKey = 'e8KZz4Yv1CyUw4T6Jf9jXjY7bUHGtNV1YcB8B8xF2lA=';
    const hydrated = inboundSettingsFromWire('wireguard', { secretKey, peers: [] });
    expect(hydrated).not.toHaveProperty('pubKey');
    expect(inboundSettingsToWire('wireguard', hydrated)).not.toHaveProperty('pubKey');
  });

  it.each([
    ['hysteria masquerade', 'hysteria', {
      version: 2,
      udpIdleTimeout: 60,
      masquerade: {
        type: 'proxy',
        url: 'https://example.com',
        rewriteHost: true,
        insecure: false,
        headers: { 'X-TapX': ['listener'] },
      },
    }],
    ['mtproto route', 'mtproto', {
      secret: 'ee00112233445566778899aabbccddeeff6578616d706c652e636f6d',
      fakeTlsDomain: 'example.com',
      domainFronting: { ip: '127.0.0.1', port: 443, proxyProtocol: true },
      proxyProtocolListener: true,
      preferIp: 'prefer-ipv4',
      debug: false,
      routeThroughXray: true,
      outboundTag: 'edge-out',
    }],
    ['tun routing', 'tun', {
      name: 'xray0',
      mtu: 1500,
      gateway: ['10.0.0.1/24', 'fd00::1/64'],
      dns: ['1.1.1.1'],
      userLevel: 0,
      autoSystemRoutingTable: ['10.0.0.0/8'],
      autoOutboundsInterface: 'auto',
    }],
    ['shadowsocks 2022', 'shadowsocks', {
      method: '2022-blake3-aes-256-gcm',
      password: 'test-password',
      network: 'tcp,udp',
      ivCheck: true,
    }],
  ])('round-trips %s protocol settings', (_name, protocol, wire) => {
    expect(inboundSettingsToWire(protocol, inboundSettingsFromWire(protocol, wire))).toEqual(wire);
  });

  it.each([
    ['raw HTTP camouflage', {
      network: 'tcp',
      security: 'none',
      tcpSettings: {
        acceptProxyProtocol: true,
        header: {
          type: 'http',
          request: { version: '1.1', method: 'GET', path: ['/tapx'], headers: { Host: ['example.com'] } },
          response: { version: '1.1', status: '200', reason: 'OK', headers: { Server: ['TapX'] } },
        },
      },
    }],
    ['mKCP and UDP mask', {
      network: 'kcp',
      security: 'none',
      kcpSettings: { mtu: 1350, tti: 20, uplinkCapacity: 5, downlinkCapacity: 20, congestion: true, seed: 'tapx' },
      finalmask: { udp: [{ type: 'noise', settings: { noise: 'base64:AA==', delay: '10-20' } }] },
    }],
    ['WebSocket and Sockopt', {
      network: 'ws',
      security: 'none',
      wsSettings: { path: '/ws', host: 'example.com', headers: { 'X-Test': 'TapX' }, heartbeatPeriod: 30 },
      sockopt: { mark: 7, tcpFastOpen: true, trustedXForwardedFor: ['CF-Connecting-IP'] },
    }],
    ['gRPC with TLS', {
      network: 'grpc',
      security: 'tls',
      grpcSettings: { serviceName: 'tapx', authority: 'example.com', multiMode: true },
      tlsSettings: {
        serverName: 'example.com',
        alpn: ['h2'],
        certificates: [{ certificateFile: '/etc/tapx/fullchain.pem', keyFile: '/etc/tapx/privkey.pem' }],
      },
    }],
    ['HTTPUpgrade', {
      network: 'httpupgrade',
      security: 'none',
      httpupgradeSettings: { path: '/upgrade', host: 'example.com', headers: { 'X-Test': 'TapX' }, acceptProxyProtocol: true },
    }],
    ['XHTTP with XMUX', {
      network: 'xhttp',
      security: 'none',
      xhttpSettings: {
        host: 'example.com',
        path: '/xhttp',
        mode: 'packet-up',
        xPaddingBytes: '100-1000',
        xmux: { maxConcurrency: '16-32', hMaxRequestTimes: '600-900' },
      },
    }],
    ['Hysteria with Reality-style transport extras', {
      network: 'hysteria',
      security: 'tls',
      hysteriaSettings: { version: 2, udpIdleTimeout: 60 },
      tlsSettings: { alpn: ['h3'], certificates: [{ certificateFile: '/etc/tapx/cert.pem', keyFile: '/etc/tapx/key.pem' }] },
      sockopt: { mark: 9 },
      finalmask: { udp: [{ type: 'noise', settings: { noise: 'base64:AQ==', delay: '5' } }] },
    }],
    ['Reality', {
      network: 'tcp',
      security: 'reality',
      tcpSettings: { header: { type: 'none' } },
      realitySettings: {
        show: false,
        target: 'example.com:443',
        serverNames: ['example.com'],
        privateKey: 'private-key',
        shortIds: ['00112233'],
        settings: { publicKey: 'public-key', fingerprint: 'chrome', spiderX: '/tapx' },
      },
    }],
  ])('round-trips %s stream settings', (_name, wire) => {
    expect(inboundStreamToWire(inboundStreamFromWire(wire))).toEqual(wire);
  });
});
