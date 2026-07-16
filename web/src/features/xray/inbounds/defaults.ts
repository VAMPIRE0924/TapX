import { generateWireguardKeypair } from '../../../shared/wireguard';
import { randomBase64, randomBytes, randomLowerAndNumber } from '../../../shared/random';

export function defaultInboundSettings(protocol: string): Record<string, unknown> {
  switch (protocol) {
    case 'vless':
      return { clients: [], decryption: 'none', encryption: 'none', fallbacks: [] };
    case 'vmess':
      return { clients: [] };
    case 'trojan':
      return { clients: [], fallbacks: [] };
    case 'shadowsocks':
      return {
        method: '2022-blake3-aes-256-gcm',
        password: randomBase64(32),
        network: 'tcp,udp',
        clients: [],
        ivCheck: false,
      };
    case 'hysteria':
      return { version: 2, clients: [] };
    case 'http':
      return {
        accounts: [{ user: randomLowerAndNumber(8), pass: randomLowerAndNumber(12) }],
        allowTransparent: false,
      };
    case 'mixed':
      return {
        auth: 'password',
        accounts: [{ user: randomLowerAndNumber(8), pass: randomLowerAndNumber(12) }],
        udp: false,
        ip: '127.0.0.1',
      };
    case 'mtproto': {
      const fakeTlsDomain = 'www.cloudflare.com';
      return { fakeTlsDomain, secret: mtprotoSecret(fakeTlsDomain) };
    }
    case 'tunnel':
      return { portMap: {}, allowedNetwork: 'tcp,udp', followRedirect: false };
    case 'tun':
      return {
        name: 'xray0',
        mtu: 1500,
        gateway: [],
        dns: [],
        userLevel: 0,
        autoSystemRoutingTable: [],
        autoOutboundsInterface: 'auto',
      };
    case 'wireguard': {
      const keypair = generateWireguardKeypair();
      return {
        mtu: 1420,
        secretKey: keypair.privateKey,
        pubKey: keypair.publicKey,
        peers: [],
        clients: [],
        noKernelTun: false,
      };
    }
    default:
      return {};
  }
}

export function defaultTapxListenerFields() {
  return {
    RawUDP: {
      KeepAliveSecond: 0,
      Workers: 0,
      QueueSize: 0,
      ZeroCopy: true,
      ConnectTimeout: 0,
      IdleTimeout: 0,
    },
    RawTCP: {
      LengthMode: 'uint16',
      NoDelay: true,
      KeepAliveSecond: 30,
      FastOpen: false,
      ConnectTimeout: 0,
      ReconnectSecond: 0,
      Workers: 0,
      QueueSize: 0,
      ZeroCopy: true,
      IdleTimeout: 0,
    },
    TLS: {
      ServerName: '',
      MinVersion: '',
      MaxVersion: '',
      CertFile: '',
      KeyFile: '',
      AllowInsecure: false,
      DtlsMtu: 0,
      DtlsReplayWindow: 0,
    },
  };
}

function mtprotoSecret(domain: string): string {
  const random = randomBytes(16);
  const randomHex = Array.from(random, (byte) => byte.toString(16).padStart(2, '0')).join('');
  const domainHex = Array.from(new TextEncoder().encode(domain), (byte) => byte.toString(16).padStart(2, '0')).join('');
  return `ee${randomHex}${domainHex}`;
}
