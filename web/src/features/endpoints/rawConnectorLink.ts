export type RawConnectorProtocol = 'raw-tcp' | 'raw-udp';
export type RawConnectorSecurity = 'none' | 'tls' | 'dtls';
import type { TranslationKey, TranslationValues } from '../../i18n/dictionaries';
import type { TcpLengthMode } from './tcpLengthMode';

type Translate = (key: TranslationKey, values?: TranslationValues) => string;

export interface RawConnectorLink {
  protocol: RawConnectorProtocol;
  name: string;
  address: string;
  port: number;
  security: RawConnectorSecurity;
  vkey: string;
  serverName: string;
  lengthMode?: TcpLengthMode;
}

export function parseRawConnectorLink(link: string, t?: Translate): RawConnectorLink | undefined {
  const value = link.trim();
  if (!value) return undefined;

  const url = new URL(value);
  if (url.protocol.toLowerCase() !== 'raw:') return undefined;

  const network = (url.searchParams.get('network') || 'udp').toLowerCase();
  if (network !== 'tcp' && network !== 'udp') {
    throw new Error(t ? t('connector.rawNetworkInvalid') : 'Raw link network must be tcp or udp');
  }

  const security = (url.searchParams.get('security') || 'none').toLowerCase();
  const allowedSecurity = network === 'tcp' ? ['none', 'tls'] : ['none', 'dtls'];
  if (!allowedSecurity.includes(security)) {
    throw new Error(network === 'tcp'
      ? (t ? t('connector.rawTcpSecurityInvalid') : 'Raw TCP supports only no security or TLS')
      : (t ? t('connector.rawUdpSecurityInvalid') : 'Raw UDP supports only no security or DTLS'));
  }

  const address = stripIPv6Brackets(url.hostname);
  const port = Number(url.port);
  if (!address) throw new Error(t ? t('connector.rawAddressRequired') : 'Raw link requires a remote address');
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error(t ? t('connector.rawPortRange') : 'Raw link port must be between 1 and 65535');
  }
  const requestedLength = (url.searchParams.get('length') || 'uint16').toLowerCase();
  if (network === 'tcp' && requestedLength !== 'uint16' && requestedLength !== 'uint32') {
    throw new Error(t ? t('connector.rawTcpLengthInvalid') : 'Raw TCP length mode must be uint16 or uint32');
  }

  return {
    protocol: network === 'tcp' ? 'raw-tcp' : 'raw-udp',
    name: decodeURIComponent(url.hash.replace(/^#/, '')),
    address,
    port,
    security: security as RawConnectorSecurity,
    vkey: url.searchParams.get('vkey') || decodeURIComponent(url.username || ''),
    serverName: url.searchParams.get('sni') || '',
    lengthMode: network === 'tcp' ? requestedLength as TcpLengthMode : undefined,
  };
}

export function buildRawConnectorLink(input: RawConnectorLink, t?: Translate): string {
  validateRawConnectorLink(input, t);
  const params = new URLSearchParams({
    network: input.protocol === 'raw-tcp' ? 'tcp' : 'udp',
    security: input.security,
  });
  if (input.serverName) params.set('sni', input.serverName);
  if (input.vkey) params.set('vkey', input.vkey);
  if (input.protocol === 'raw-tcp') params.set('length', input.lengthMode || 'uint16');

  const host = input.address.includes(':') && !input.address.startsWith('[')
    ? `[${input.address}]`
    : input.address;
  const name = input.name ? `#${encodeURIComponent(input.name)}` : '';
  return `raw://${host}:${input.port}?${params.toString()}${name}`;
}

function validateRawConnectorLink(input: RawConnectorLink, t?: Translate): void {
  const network = input.protocol === 'raw-tcp' ? 'tcp' : input.protocol === 'raw-udp' ? 'udp' : '';
  if (!network) throw new Error(t ? t('connector.rawProtocolInvalid') : 'Invalid Raw connector protocol');
  if (!input.address.trim()) throw new Error(t ? t('connector.rawAddressRequired') : 'Raw connector requires a remote address');
  if (!Number.isInteger(input.port) || input.port < 1 || input.port > 65535) {
    throw new Error(t ? t('connector.rawPortRange') : 'Raw connector port must be between 1 and 65535');
  }
  if (network === 'tcp' && input.security !== 'none' && input.security !== 'tls') {
    throw new Error(t ? t('connector.rawTcpSecurityInvalid') : 'Raw TCP supports only no security or TLS');
  }
  if (network === 'udp' && input.security !== 'none' && input.security !== 'dtls') {
    throw new Error(t ? t('connector.rawUdpSecurityInvalid') : 'Raw UDP supports only no security or DTLS');
  }
  if (network === 'tcp' && input.lengthMode && input.lengthMode !== 'uint16' && input.lengthMode !== 'uint32') {
    throw new Error(t ? t('connector.rawTcpLengthInvalid') : 'Raw TCP length mode must be uint16 or uint32');
  }
}

function stripIPv6Brackets(host: string): string {
  return host.startsWith('[') && host.endsWith(']') ? host.slice(1, -1) : host;
}
