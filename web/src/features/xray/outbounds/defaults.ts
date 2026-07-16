export const defaultSniffing = {
  enabled: false,
  destOverride: ['http', 'tls', 'quic', 'fakedns'],
  metadataOnly: false,
  routeOnly: false,
  ipsExcluded: [],
  domainsExcluded: [],
};

export function defaultOutboundSettings(protocol: string): Record<string, unknown> {
  switch (protocol) {
    case 'vmess':
      return { address: '', port: 443, id: '', security: 'auto' };
    case 'vless':
      return {
        address: '',
        port: 443,
        id: '',
        flow: '',
        encryption: 'none',
        reverseTag: '',
        reverseSniffing: { ...defaultSniffing, destOverride: [...defaultSniffing.destOverride] },
        testpre: 0,
        testseed: [900, 500, 900, 256],
      };
    case 'trojan':
      return { address: '', port: 443, password: '' };
    case 'shadowsocks':
      return {
        address: '',
        port: 443,
        password: '',
        method: '2022-blake3-aes-128-gcm',
        uot: false,
        UoTVersion: 1,
      };
    case 'socks':
      return { address: '', port: 1080, user: '', pass: '' };
    case 'http':
      return { address: '', port: 8080, user: '', pass: '', headers: {} };
    case 'wireguard':
      return {
        mtu: 1420,
        secretKey: '',
        pubKey: '',
        address: '',
        domainStrategy: '',
        reserved: '',
        peers: [],
        noKernelTun: false,
      };
    case 'hysteria':
      return { address: '', port: 443, version: 2 };
    case 'freedom':
      return {
        domainStrategy: '',
        redirect: '',
        userLevel: 0,
        proxyProtocol: 0,
        fragment: { packets: '', length: '', interval: '', maxSplit: '' },
        noises: [],
        finalRules: [],
      };
    case 'blackhole':
      return { type: '' };
    case 'dns':
      return { rewriteNetwork: '', rewriteAddress: '', rewritePort: 53, userLevel: 0, rules: [] };
    case 'loopback':
      return {
        inboundTag: '',
        sniffing: { ...defaultSniffing, destOverride: [...defaultSniffing.destOverride] },
      };
    default:
      return {};
  }
}

export function defaultTapxConnectorFields() {
  return {
    VKey: '',
    RawUDP: {
      KeepAliveSecond: 0,
      Workers: 0,
      QueueSize: 0,
      ZeroCopy: true,
      ConnectTimeout: 10,
      IdleTimeout: 60,
    },
    RawTCP: {
      LengthMode: 'uint16',
      NoDelay: true,
      KeepAliveSecond: 30,
      FastOpen: false,
      ConnectTimeout: 10,
      ReconnectSecond: 0,
      Workers: 0,
      QueueSize: 0,
      ZeroCopy: true,
      IdleTimeout: 60,
    },
    TLS: {
      ServerName: '',
      CipherSuites: '',
      MinVersion: '',
      MaxVersion: '',
      UTLS: '',
      ALPN: [],
      AllowInsecure: false,
      PinnedPeerCertSha256: '',
      VerifyPeerCertByName: '',
      DtlsMtu: 0,
      DtlsReplayWindow: 0,
    },
  };
}
