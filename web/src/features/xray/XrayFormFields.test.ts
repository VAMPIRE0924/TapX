import { describe, expect, it } from 'vitest';
import {
  applyNetworkChange,
  canEnableMux,
  canEnableFinalMask,
  canEnableReality,
  canEnableStream,
  canEnableTls,
  canEnableTlsFlow,
  inboundXrayProtocolOptions,
  hasSelectableXrayTransport,
  networkOptions,
  outboundXrayProtocolOptions,
  newStreamSlice,
  newInboundStreamSlice,
  newInboundHysteriaStreamSlice,
  shouldShowInboundProtocolTab,
} from './XrayFormFields';
import { mtprotoOutboundTagOptions } from './protocols/MtprotoInboundFields';
import { newInboundRealitySettings, newInboundTlsSettings, newOutboundTlsSettings } from './security/defaults';

describe('Xray form capability matrix', () => {
  it('keeps the agreed listener protocol set stable', () => {
    expect(inboundXrayProtocolOptions.map((item) => item.value)).toEqual([
      'vmess',
      'vless',
      'trojan',
      'shadowsocks',
      'wireguard',
      'hysteria',
      'http',
      'mixed',
      'tunnel',
      'tun',
      'mtproto',
    ]);
  });

  it('keeps the agreed connector protocol set stable', () => {
    expect(outboundXrayProtocolOptions.map((item) => item.value)).toEqual([
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
    ]);
  });

  it('keeps the selectable Xray transport set stable', () => {
    expect(networkOptions.map((item) => item.value)).toEqual([
      'tcp',
      'kcp',
      'ws',
      'grpc',
      'httpupgrade',
      'xhttp',
    ]);
  });

  it('creates XHTTP with the valid auto mode instead of an empty selection', () => {
    expect(newStreamSlice('xhttp')).toMatchObject({
      network: 'xhttp',
      security: 'none',
      xhttpSettings: {
        mode: 'auto',
        path: '/',
        xPaddingBytes: '100-1000',
      },
    });
  });

  it('creates every listener XHTTP field with the reference defaults', () => {
    expect(newInboundStreamSlice('xhttp')).toMatchObject({
      network: 'xhttp',
      security: 'none',
      xhttpSettings: {
        mode: 'auto',
        headers: {},
        xPaddingObfsMode: false,
        sessionIDPlacement: '',
        sessionIDTable: '',
        scMaxBufferedPosts: 30,
        scStreamUpServerSecs: '20-80',
        noSSEHeader: false,
        enableXmux: false,
      },
    });
  });

  it('adds and removes the listener mKCP compatibility mask on transport changes', () => {
    const kcp = applyNetworkChange('vless', newInboundStreamSlice('tcp'), 'kcp', 'inbound');
    expect(kcp).toMatchObject({
      network: 'kcp',
      finalmask: { udp: [{ type: 'mkcp-legacy', settings: { header: '', value: '' } }] },
    });
    const ws = applyNetworkChange('vless', kcp, 'ws', 'inbound');
    expect((ws.finalmask as { udp: unknown[] }).udp).toEqual([]);
  });

  it('creates the listener Hysteria stream with the reference server defaults', () => {
    const stream = newInboundHysteriaStreamSlice();
    expect(stream).toMatchObject({
      network: 'hysteria',
      security: 'tls',
      hysteriaSettings: { version: 2, udpIdleTimeout: 60 },
      tlsSettings: {
        alpn: ['h3'],
        certificates: [{ useFile: true, usage: 'encipherment' }],
        settings: { fingerprint: '' },
      },
      finalmask: {
        tcp: [],
        udp: [{ type: 'salamander' }],
      },
    });
    expect((stream.finalmask as { udp: Array<{ settings: { password: string } }> }).udp[0].settings.password).toHaveLength(16);
  });

  it('creates the reference TLS and Reality defaults when security is enabled', () => {
    expect(newInboundTlsSettings()).toMatchObject({
      minVersion: '1.2',
      maxVersion: '1.3',
      alpn: ['h2', 'http/1.1'],
      settings: { fingerprint: 'chrome' },
      certificates: [{ useFile: true, usage: 'encipherment' }],
    });
    expect(newOutboundTlsSettings()).toEqual({
      serverName: '',
      alpn: [],
      fingerprint: '',
      echConfigList: '',
      verifyPeerCertByName: '',
      pinnedPeerCertSha256: '',
    });
    const reality = newInboundRealitySettings();
    expect((reality.shortIds as string[]).map((value) => value.length).sort((a, b) => a - b)).toEqual([2, 4, 6, 8, 10, 12, 14, 16]);
    expect(reality.settings.spiderX).toMatch(/^\/[a-z0-9]{15}$/);
  });

  it('exposes stream settings only for protocols that use them', () => {
    const enabled = ['vmess', 'vless', 'trojan', 'shadowsocks', 'hysteria', 'wireguard', 'tunnel'];
    const disabled = ['http', 'mixed', 'tun', 'mtproto', 'freedom', 'blackhole', 'dns', 'loopback'];
    for (const protocol of enabled) expect(canEnableStream(protocol), protocol).toBe(true);
    for (const protocol of disabled) expect(canEnableStream(protocol), protocol).toBe(false);
  });

  it('matches TLS and Reality protocol/network restrictions', () => {
    const protocols = [...new Set([
      ...inboundXrayProtocolOptions.map((item) => item.value),
      ...outboundXrayProtocolOptions.map((item) => item.value),
    ])];
    const networks = [...networkOptions.map((item) => item.value), 'hysteria'];
    const tlsProtocols = new Set(['vmess', 'vless', 'trojan', 'shadowsocks']);
    const tlsNetworks = new Set(['tcp', 'ws', 'http', 'grpc', 'httpupgrade', 'xhttp']);
    const realityProtocols = new Set(['vless', 'trojan']);
    const realityNetworks = new Set(['tcp', 'http', 'grpc', 'xhttp']);

    for (const protocol of protocols) {
      for (const network of networks) {
        const tlsExpected = protocol === 'hysteria'
          || (tlsProtocols.has(protocol) && tlsNetworks.has(network));
        const realityExpected = realityProtocols.has(protocol) && realityNetworks.has(network);
        expect(canEnableTls(protocol, network), `${protocol}/${network} TLS`).toBe(tlsExpected);
        expect(canEnableReality(protocol, network), `${protocol}/${network} Reality`).toBe(realityExpected);
      }
    }
  });

  it('limits Vision flow and Mux to compatible combinations', () => {
    expect(canEnableTlsFlow('vless', 'tcp', 'tls')).toBe(true);
    expect(canEnableTlsFlow('vless', 'tcp', 'reality')).toBe(true);
    expect(canEnableTlsFlow('vless', 'xhttp', 'none', 'mlkem768x25519plus.native.0rtt.test')).toBe(true);
    expect(canEnableTlsFlow('vless', 'xhttp', 'none', 'none')).toBe(false);
    expect(canEnableTlsFlow('vmess', 'tcp', 'tls')).toBe(false);

    expect(canEnableMux('vmess', '', 'tcp')).toBe(true);
    expect(canEnableMux('vless', 'xtls-rprx-vision', 'tcp')).toBe(false);
    expect(canEnableMux('vless', '', 'xhttp')).toBe(false);
    expect(canEnableMux('wireguard', '', 'tcp')).toBe(false);
  });

  it('shows the listener protocol tab only when it has usable fields', () => {
    const alwaysVisible = new Set(['vless', 'shadowsocks', 'http', 'mixed', 'wireguard', 'tunnel', 'tun', 'mtproto']);
    for (const runtime of ['embedded-xray', 'external-xray']) {
      for (const protocol of inboundXrayProtocolOptions.map((item) => item.value)) {
        const expected = alwaysVisible.has(protocol);
        expect(shouldShowInboundProtocolTab(runtime, protocol, 'ws', 'none'), `${runtime}/${protocol}`).toBe(expected);
      }
      expect(shouldShowInboundProtocolTab(runtime, 'trojan', 'tcp', 'tls')).toBe(true);
      expect(shouldShowInboundProtocolTab(runtime, 'trojan', 'tcp', 'reality')).toBe(true);
      expect(shouldShowInboundProtocolTab(runtime, 'trojan', 'tcp', 'none')).toBe(false);
      expect(shouldShowInboundProtocolTab(runtime, 'trojan', 'ws', 'tls')).toBe(false);
    }
    for (const protocol of inboundXrayProtocolOptions.map((item) => item.value)) {
      expect(shouldShowInboundProtocolTab('tapx', protocol, 'tcp', 'tls'), protocol).toBe(false);
    }
  });

  it('keeps dedicated transports and transport extras aligned with the listener form', () => {
    const dedicated = new Set(['hysteria', 'wireguard', 'tunnel']);
    for (const protocol of inboundXrayProtocolOptions.map((item) => item.value)) {
      expect(hasSelectableXrayTransport(protocol), protocol).toBe(!dedicated.has(protocol));
      expect(canEnableFinalMask(protocol), protocol).toBe(protocol !== 'tunnel');
    }
  });

});

describe('MTProto outbound tags', () => {
  it('normalizes connector tags without blank or duplicate entries', () => {
    expect(mtprotoOutboundTagOptions(['edge-a', ' edge-b ', '', 'edge-a'])).toEqual([
      { value: 'edge-a', label: 'edge-a' },
      { value: 'edge-b', label: 'edge-b' },
    ]);
  });
});
