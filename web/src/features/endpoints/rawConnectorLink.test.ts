import { describe, expect, it } from 'vitest';
import { buildRawConnectorLink, parseRawConnectorLink, type RawConnectorLink } from './rawConnectorLink';

describe('TapX Raw connector links', () => {
  it.each<RawConnectorLink>([
    {
      protocol: 'raw-tcp',
      name: '东京 TCP',
      address: 'tcp.example.com',
      port: 443,
      security: 'tls',
      vkey: 'vkey-a+b',
      serverName: 'tcp.example.com',
      lengthMode: 'uint32',
    },
    {
      protocol: 'raw-udp',
      name: 'IPv6 UDP',
      address: '2001:db8::10',
      port: 46000,
      security: 'dtls',
      vkey: '',
      serverName: 'udp.example.com',
    },
    {
      protocol: 'raw-udp',
      name: '裸跑',
      address: '198.51.100.10',
      port: 41000,
      security: 'none',
      vkey: '',
      serverName: '',
    },
  ])('round-trips $protocol with $security', (input) => {
    expect(parseRawConnectorLink(buildRawConnectorLink(input))).toEqual(input);
  });

  it('accepts the legacy username vKey form', () => {
    expect(parseRawConnectorLink('raw://legacy-key@edge.example.com:45000?network=tcp&security=none#edge')).toMatchObject({
      protocol: 'raw-tcp',
      vkey: 'legacy-key',
      address: 'edge.example.com',
      port: 45000,
      lengthMode: 'uint16',
    });
  });

  it('returns undefined for non-Raw links', () => {
    expect(parseRawConnectorLink('vless://uuid@example.com:443')).toBeUndefined();
  });

  it.each([
    'raw://edge.example.com:443?network=tcp&security=dtls',
    'raw://edge.example.com:443?network=udp&security=tls',
    'raw://edge.example.com:443?network=quic&security=none',
    'raw://edge.example.com?network=tcp&security=none',
    'raw://edge.example.com:443?network=tcp&security=none&length=uint64',
  ])('rejects invalid Raw combinations: %s', (link) => {
    expect(() => parseRawConnectorLink(link)).toThrow();
  });
});
