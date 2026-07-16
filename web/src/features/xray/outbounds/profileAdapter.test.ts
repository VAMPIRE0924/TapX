import { describe, expect, it } from 'vitest';
import { defaultOutboundSettings } from './defaults';
import {
  outboundSettingsFromWire,
  outboundSettingsToWire,
  outboundStreamFromWire,
  outboundStreamToWire,
} from './profileAdapter';

const protocols = [
  'freedom',
  'blackhole',
  'dns',
  'vmess',
  'vless',
  'trojan',
  'shadowsocks',
  'wireguard',
  'hysteria',
  'socks',
  'http',
  'loopback',
] as const;

describe('outbound settings wire adapter', () => {
  for (const protocol of protocols) {
    it(`round-trips ${protocol} defaults`, () => {
      const defaults = defaultOutboundSettings(protocol);
      const wire = outboundSettingsToWire(protocol, defaults);
      const restored = outboundSettingsFromWire(protocol, wire);

      expect(restored).toEqual(expect.objectContaining(expectedStableFields(protocol, defaults)));
    });
  }

  it('stores VLESS reverse sniffing in the Xray wire shape', () => {
    const wire = outboundSettingsToWire('vless', {
      ...defaultOutboundSettings('vless'),
      address: 'edge.example.com',
      port: 8443,
      id: '95ae7d77-e438-4b38-8d05-100ac56cf2d6',
      reverseTag: 'reverse-a',
      reverseSniffing: {
        enabled: true,
        destOverride: ['http', 'tls'],
        metadataOnly: true,
        routeOnly: false,
        ipsExcluded: ['10.0.0.0/8'],
        domainsExcluded: ['example.org'],
      },
    });

    expect(wire).toMatchObject({
      address: 'edge.example.com',
      port: 8443,
      reverse: {
        tag: 'reverse-a',
        sniffing: {
          enabled: true,
          destOverride: ['http', 'tls'],
          metadataOnly: true,
          ipsExcluded: ['10.0.0.0/8'],
          domainsExcluded: ['example.org'],
        },
      },
    });
  });

  it('removes XHTTP view-only flags and restores them from XMUX', () => {
    const wire = outboundStreamToWire({
      network: 'xhttp',
      security: 'none',
      xhttpSettings: {
        path: '/tapx',
        enableXmux: true,
        xmux: { maxConcurrency: '16-32' },
      },
    });

    expect(wire).toEqual({
      network: 'xhttp',
      security: 'none',
      xhttpSettings: {
        path: '/tapx',
        xmux: { maxConcurrency: '16-32' },
      },
    });
    expect(outboundStreamFromWire(wire)).toMatchObject({
      xhttpSettings: { enableXmux: true },
    });
  });

  it.each([
    ['raw HTTP camouflage', {
      network: 'tcp',
      security: 'none',
      tcpSettings: {
        header: {
          type: 'http',
          request: { version: '1.1', method: 'GET', path: ['/tapx'], headers: { Host: ['example.com'] } },
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
      sockopt: { mark: 7, tcpFastOpen: true, dialerProxy: 'edge-out' },
    }],
    ['gRPC with client TLS', {
      network: 'grpc',
      security: 'tls',
      grpcSettings: { serviceName: 'tapx', authority: 'example.com', multiMode: true },
      tlsSettings: {
        serverName: 'example.com',
        alpn: ['h2'],
        fingerprint: 'chrome',
        verifyPeerCertByName: 'example.com',
        pinnedPeerCertSha256: 'sha256-value',
      },
    }],
    ['HTTPUpgrade', {
      network: 'httpupgrade',
      security: 'none',
      httpupgradeSettings: { path: '/upgrade', host: 'example.com', headers: { 'X-Test': 'TapX' } },
    }],
    ['Hysteria with transport extras', {
      network: 'hysteria',
      security: 'tls',
      hysteriaSettings: { version: 2, auth: 'secret', udpIdleTimeout: 60 },
      tlsSettings: { serverName: 'example.com', alpn: ['h3'], fingerprint: 'chrome' },
      sockopt: { mark: 9 },
      finalmask: { udp: [{ type: 'noise', settings: { noise: 'base64:AQ==', delay: '5' } }] },
    }],
    ['Reality', {
      network: 'tcp',
      security: 'reality',
      tcpSettings: { header: { type: 'none' } },
      realitySettings: {
        serverName: 'example.com',
        fingerprint: 'chrome',
        publicKey: 'public-key',
        shortId: '00112233',
        spiderX: '/tapx',
      },
    }],
  ])('round-trips %s stream settings', (_name, wire) => {
    expect(outboundStreamToWire(outboundStreamFromWire(wire))).toEqual(wire);
  });

  it('round-trips WireGuard peers without leaking form-only keys', () => {
    const settings = {
      mtu: 1380,
      secretKey: 'private-key',
      pubKey: 'derived-display-only',
      address: '10.7.0.2/32,fd00:7::2/128',
      domainStrategy: 'ForceIPv4',
      reserved: '1, 2, 3',
      noKernelTun: true,
      peers: [{
        publicKey: 'peer-public-key',
        psk: 'peer-psk',
        allowedIPs: ['0.0.0.0/0', '::/0'],
        endpoint: 'edge.example.com:51820',
        keepAlive: 25,
      }],
    };

    const wire = outboundSettingsToWire('wireguard', settings);
    expect(wire).toEqual({
      mtu: 1380,
      secretKey: 'private-key',
      address: ['10.7.0.2/32', 'fd00:7::2/128'],
      domainStrategy: 'ForceIPv4',
      reserved: [1, 2, 3],
      noKernelTun: true,
      peers: [{
        publicKey: 'peer-public-key',
        preSharedKey: 'peer-psk',
        allowedIPs: ['0.0.0.0/0', '::/0'],
        endpoint: 'edge.example.com:51820',
        keepAlive: 25,
      }],
    });
    expect(outboundSettingsFromWire('wireguard', wire)).toMatchObject({
      mtu: 1380,
      secretKey: 'private-key',
      address: '10.7.0.2/32,fd00:7::2/128',
      domainStrategy: 'ForceIPv4',
      reserved: '1,2,3',
      noKernelTun: true,
      peers: [{
        publicKey: 'peer-public-key',
        psk: 'peer-psk',
        allowedIPs: ['0.0.0.0/0', '::/0'],
        endpoint: 'edge.example.com:51820',
        keepAlive: 25,
      }],
    });
  });

  it('preserves every enabled Freedom composition block', () => {
    const wire = outboundSettingsToWire('freedom', {
      domainStrategy: 'UseIPv4',
      redirect: '127.0.0.1:8080',
      userLevel: 3,
      proxyProtocol: 2,
      fragment: { packets: 'tlshello', length: '100-200', interval: '10-20', maxSplit: '300-400' },
      noises: [{ type: 'rand', packet: '10-20', delay: '10-16', applyTo: 'ipv4' }],
      finalRules: [
        { action: 'allow', network: 'udp', port: '53', ip: ['1.1.1.1'], blockDelay: 'ignored' },
        { action: 'block', network: 'tcp', port: '443', ip: ['geoip:private'], blockDelay: '5000-10000' },
      ],
    });

    expect(wire).toEqual({
      domainStrategy: 'UseIPv4',
      redirect: '127.0.0.1:8080',
      userLevel: 3,
      proxyProtocol: 2,
      fragment: { packets: 'tlshello', length: '100-200', interval: '10-20', maxSplit: '300-400' },
      noises: [{ type: 'rand', packet: '10-20', delay: '10-16', applyTo: 'ipv4' }],
      finalRules: [
        { action: 'allow', network: 'udp', port: '53', ip: ['1.1.1.1'] },
        { action: 'block', network: 'tcp', port: '443', ip: ['geoip:private'], blockDelay: '5000-10000' },
      ],
    });
    expect(outboundSettingsFromWire('freedom', wire)).toMatchObject({
      fragment: { packets: 'tlshello', length: '100-200', interval: '10-20', maxSplit: '300-400' },
      noises: [{ type: 'rand', packet: '10-20', delay: '10-16', applyTo: 'ipv4' }],
      finalRules: [
        { action: 'allow', network: 'udp', port: '53', ip: ['1.1.1.1'], blockDelay: '' },
        { action: 'block', network: 'tcp', port: '443', ip: ['geoip:private'], blockDelay: '5000-10000' },
      ],
    });
  });

  it('converts DNS rule lists between editable and Xray shapes', () => {
    const wire = outboundSettingsToWire('dns', {
      rewriteNetwork: 'tcp',
      rewriteAddress: '1.1.1.1',
      rewritePort: 5353,
      userLevel: 2,
      rules: [
        { action: 'direct', qType: '1', domain: 'domain:example.com,regexp:^api', rCode: 0 },
        { action: 'drop', qType: '23-24', domain: '', rCode: 3 },
      ],
    });

    expect(wire).toEqual({
      rewriteNetwork: 'tcp',
      rewriteAddress: '1.1.1.1',
      rewritePort: 5353,
      userLevel: 2,
      rules: [
        { action: 'direct', qType: 1, domain: ['domain:example.com', 'regexp:^api'] },
        { action: 'drop', qType: '23-24', rCode: 3 },
      ],
    });
    expect(outboundSettingsFromWire('dns', wire)).toMatchObject({
      rules: [
        { action: 'direct', qType: '1', domain: 'domain:example.com,regexp:^api', rCode: 0 },
        { action: 'drop', qType: '23-24', domain: '', rCode: 3 },
      ],
    });
  });

  it('keeps HTTP authentication and request headers in their Xray locations', () => {
    const wire = outboundSettingsToWire('http', {
      address: 'proxy.example.com',
      port: 8080,
      user: 'alice',
      pass: 'secret',
      headers: { 'User-Agent': ['TapX'], 'X-Trace': ['one', 'two'] },
    });

    expect(wire).toEqual({
      servers: [{
        address: 'proxy.example.com',
        port: 8080,
        users: [{ user: 'alice', pass: 'secret' }],
      }],
      headers: { 'User-Agent': ['TapX'], 'X-Trace': ['one', 'two'] },
    });
    expect(outboundSettingsFromWire('http', wire)).toMatchObject({
      address: 'proxy.example.com',
      port: 8080,
      user: 'alice',
      pass: 'secret',
      headers: { 'User-Agent': ['TapX'], 'X-Trace': ['one', 'two'] },
    });
  });

  it('emits Loopback sniffing only when enabled', () => {
    const disabled = outboundSettingsToWire('loopback', {
      inboundTag: 'listener-a',
      sniffing: { enabled: false, destOverride: ['http'] },
    });
    expect(disabled).toEqual({ inboundTag: 'listener-a' });

    const enabled = outboundSettingsToWire('loopback', {
      inboundTag: 'listener-a',
      sniffing: {
        enabled: true,
        destOverride: ['http', 'tls'],
        metadataOnly: true,
        routeOnly: false,
        ipsExcluded: ['10.0.0.0/8'],
        domainsExcluded: ['example.org'],
      },
    });
    expect(enabled).toMatchObject({
      inboundTag: 'listener-a',
      sniffing: {
        enabled: true,
        destOverride: ['http', 'tls'],
        metadataOnly: true,
        ipsExcluded: ['10.0.0.0/8'],
        domainsExcluded: ['example.org'],
      },
    });
    expect(outboundSettingsFromWire('loopback', enabled)).toMatchObject({
      inboundTag: 'listener-a',
      sniffing: { enabled: true, destOverride: ['http', 'tls'] },
    });
  });
});

function expectedStableFields(protocol: string, defaults: Record<string, unknown>): Record<string, unknown> {
  switch (protocol) {
    case 'vmess':
      return pick(defaults, ['address', 'port', 'id', 'security']);
    case 'vless':
      return pick(defaults, ['address', 'port', 'id', 'flow', 'encryption', 'reverseTag', 'testpre', 'testseed']);
    case 'trojan':
      return pick(defaults, ['address', 'port', 'password']);
    case 'shadowsocks':
      return pick(defaults, ['address', 'port', 'password', 'method', 'uot', 'UoTVersion']);
    case 'socks':
    case 'http':
      return pick(defaults, ['address', 'port', 'user', 'pass']);
    case 'wireguard':
      return pick(defaults, ['mtu', 'secretKey', 'address', 'domainStrategy', 'reserved', 'peers', 'noKernelTun']);
    case 'freedom':
      return pick(defaults, ['domainStrategy', 'redirect', 'userLevel', 'proxyProtocol', 'noises', 'finalRules']);
    case 'blackhole':
      return pick(defaults, ['type']);
    case 'dns':
      return pick(defaults, ['rewriteNetwork', 'rewriteAddress', 'rewritePort', 'userLevel', 'rules']);
    case 'loopback':
      return pick(defaults, ['inboundTag']);
    default:
      return defaults;
  }
}

function pick(input: Record<string, unknown>, keys: string[]): Record<string, unknown> {
  return Object.fromEntries(keys.map((key) => [key, input[key]]));
}
