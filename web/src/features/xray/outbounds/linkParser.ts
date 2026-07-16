import { newStreamSlice } from '../XrayFormFields';
import { outboundSettingsFromWire } from './profileAdapter';

type JsonObject = Record<string, unknown>;

export interface ParsedOutboundLink {
  protocol: string;
  name: string;
  address: string;
  port: number;
  settings: JsonObject;
  streamSettings?: JsonObject;
}

export class OutboundLinkParseError extends Error {
  constructor(readonly code: 'invalid-shadowsocks') {
    super(code);
    this.name = 'OutboundLinkParseError';
  }
}

const xhttpStringKeys = [
  'xPaddingBytes', 'xPaddingKey', 'xPaddingHeader', 'xPaddingPlacement', 'xPaddingMethod',
  'sessionIDPlacement', 'sessionIDKey', 'sessionIDTable', 'sessionIDLength', 'seqPlacement',
  'seqKey', 'uplinkDataPlacement', 'uplinkDataKey', 'scMaxEachPostBytes',
  'scMinPostsIntervalMs', 'scStreamUpServerSecs', 'uplinkHTTPMethod',
] as const;
const xhttpNumberKeys = ['scMaxBufferedPosts', 'serverMaxHeaderBytes', 'uplinkChunkSize'] as const;
const xhttpBooleanKeys = ['xPaddingObfsMode', 'noSSEHeader', 'noGRPCHeader'] as const;

export function parseOutboundLink(link: string): ParsedOutboundLink | undefined {
  const value = link.trim();
  if (!value) return undefined;
  if (value.startsWith('vmess://')) return parseVmess(value);
  if (value.startsWith('ss://')) return parseShadowsocks(value);

  const url = new URL(value);
  const scheme = url.protocol.replace(':', '').toLowerCase();
  if (scheme === 'vless') return parseVless(url);
  if (scheme === 'trojan') return parseTrojan(url);
  if (scheme === 'hysteria2' || scheme === 'hy2') return parseHysteria(url);
  if (scheme === 'wireguard' || scheme === 'wg') return parseWireguard(url);
  return undefined;
}

function parseVmess(link: string): ParsedOutboundLink {
  const payload = JSON.parse(decodeBase64(link.slice('vmess://'.length))) as JsonObject;
  const address = stringValue(payload.add);
  const port = numberValue(payload.port, 443);
  const network = stringValue(payload.net, 'tcp');
  const security = payload.tls === 'tls' ? 'tls' : 'none';
  const stream = buildStream(network, security);
  applyVmessTransport(stream, payload);
  if (security === 'tls') {
    const tls = objectValue(stream.tlsSettings);
    tls.serverName = stringValue(payload.sni);
    tls.fingerprint = stringValue(payload.fp);
    const alpn = stringValue(payload.alpn);
    if (alpn) tls.alpn = splitComma(alpn);
  }
  return {
    protocol: 'vmess',
    name: stringValue(payload.ps),
    address,
    port,
    settings: {
      address,
      port,
      id: stringValue(payload.id),
      security: stringValue(payload.scy, 'auto'),
    },
    streamSettings: stream,
  };
}

function parseVless(url: URL): ParsedOutboundLink {
  const address = url.hostname;
  const port = Number(url.port) || 443;
  const params = url.searchParams;
  const stream = buildStream(params.get('type') || 'tcp', params.get('security') || 'none');
  applyTransportParams(stream, params);
  applySecurityParams(stream, params);
  applyFinalMask(stream, params);
  return {
    protocol: 'vless',
    name: decodeRemark(url),
    address,
    port,
    settings: {
      address,
      port,
      id: decodeURIComponent(url.username),
      flow: params.get('flow') || '',
      encryption: params.get('encryption') || 'none',
    },
    streamSettings: stream,
  };
}

function parseTrojan(url: URL): ParsedOutboundLink {
  const address = url.hostname;
  const port = Number(url.port) || 443;
  const params = url.searchParams;
  const stream = buildStream(params.get('type') || 'tcp', params.get('security') || 'tls');
  applyTransportParams(stream, params);
  applySecurityParams(stream, params);
  applyFinalMask(stream, params);
  return {
    protocol: 'trojan',
    name: decodeRemark(url),
    address,
    port,
    settings: { address, port, password: decodeURIComponent(url.username) },
    streamSettings: stream,
  };
}

function parseShadowsocks(link: string): ParsedOutboundLink {
  const hashIndex = link.indexOf('#');
  const name = hashIndex >= 0 ? decodeURIComponent(link.slice(hashIndex + 1)) : '';
  const withoutHash = hashIndex >= 0 ? link.slice(0, hashIndex) : link;
  const withoutQuery = withoutHash.split('?')[0];
  const core = withoutQuery.slice('ss://'.length);
  let credentials = '';
  let endpoint = '';
  const at = core.indexOf('@');
  if (at >= 0) {
    const rawCredentials = core.slice(0, at);
    credentials = rawCredentials.includes(':') ? decodeURIComponent(rawCredentials) : decodeBase64(rawCredentials);
    endpoint = core.slice(at + 1);
  } else {
    const decoded = decodeBase64(core);
    const decodedAt = decoded.lastIndexOf('@');
    if (decodedAt < 0) throw new OutboundLinkParseError('invalid-shadowsocks');
    credentials = decoded.slice(0, decodedAt);
    endpoint = decoded.slice(decodedAt + 1);
  }
  const credentialSeparator = credentials.indexOf(':');
  const method = credentialSeparator >= 0 ? credentials.slice(0, credentialSeparator) : '2022-blake3-aes-128-gcm';
  const password = credentialSeparator >= 0 ? credentials.slice(credentialSeparator + 1) : credentials;
  const target = splitEndpoint(endpoint, 443);
  return {
    protocol: 'shadowsocks',
    name,
    address: target.host,
    port: target.port,
    settings: {
      address: target.host,
      port: target.port,
      password,
      method,
      uot: false,
      UoTVersion: 1,
    },
    streamSettings: buildStream('tcp', 'none'),
  };
}

function parseHysteria(url: URL): ParsedOutboundLink {
  const address = url.hostname;
  const port = Number(url.port) || 443;
  const params = url.searchParams;
  const stream = buildStream('hysteria', 'tls');
  stream.hysteriaSettings = {
    version: 2,
    auth: decodeURIComponent(url.username),
    udpIdleTimeout: 60,
  };
  const tls = objectValue(stream.tlsSettings);
  tls.serverName = params.get('sni') || '';
  tls.alpn = splitComma(params.get('alpn') || 'h3');
  tls.fingerprint = params.get('fp') || '';
  tls.echConfigList = params.get('ech') || '';
  tls.verifyPeerCertByName = params.get('vcn') || '';
  tls.pinnedPeerCertSha256 = params.get('pinSHA256') || '';
  applyFinalMask(stream, params);
  return {
    protocol: 'hysteria',
    name: decodeRemark(url),
    address,
    port,
    settings: { address, port, version: 2 },
    streamSettings: stream,
  };
}

function parseWireguard(url: URL): ParsedOutboundLink {
  const params = url.searchParams;
  const address = url.hostname;
  const port = Number(url.port) || 0;
  const peer: JsonObject = {
    publicKey: firstParam(params, 'publickey', 'publicKey', 'public_key', 'peerPublicKey') || '',
    endpoint: port ? `${address}:${port}` : address,
    allowedIPs: splitComma(firstParam(params, 'allowedips', 'allowed_ips') || '0.0.0.0/0,::/0'),
    preSharedKey: firstParam(params, 'presharedkey', 'preshared_key', 'pre-shared-key', 'psk') || '',
    keepAlive: numberValue(firstParam(params, 'keepalive', 'persistentkeepalive', 'persistent_keepalive'), 0),
  };
  const wire: JsonObject = {
    secretKey: decodeURIComponent(url.username),
    address: splitComma(firstParam(params, 'address', 'ip') || ''),
    mtu: numberValue(params.get('mtu'), 1420),
    reserved: splitComma(params.get('reserved') || '').map(Number).filter(Number.isFinite),
    peers: [peer],
    noKernelTun: false,
  };
  return {
    protocol: 'wireguard',
    name: decodeRemark(url),
    address,
    port,
    settings: outboundSettingsFromWire('wireguard', wire),
    streamSettings: { security: 'none' },
  };
}

function buildStream(network: string, security: string): JsonObject {
  const stream = newStreamSlice(network);
  stream.security = security;
  if (security === 'tls') {
    stream.tlsSettings = {
      serverName: '', alpn: [], fingerprint: '', echConfigList: '',
      verifyPeerCertByName: '', pinnedPeerCertSha256: '',
    };
  } else if (security === 'reality') {
    stream.realitySettings = {
      serverName: '', fingerprint: 'chrome', publicKey: '', shortId: '', spiderX: '', mldsa65Verify: '',
    };
  }
  return stream;
}

function applyVmessTransport(stream: JsonObject, payload: JsonObject): void {
  const network = stringValue(stream.network, 'tcp');
  const host = stringValue(payload.host);
  const path = stringValue(payload.path, '/');
  if (network === 'tcp' && payload.type === 'http') {
    stream.tcpSettings = { header: { type: 'http', request: { version: '1.1', method: 'GET', path: splitComma(path), headers: host ? { Host: splitComma(host) } : {} } } };
  } else if (network === 'ws') {
    Object.assign(objectValue(stream.wsSettings), { host, path });
  } else if (network === 'grpc') {
    Object.assign(objectValue(stream.grpcSettings), { serviceName: path, authority: stringValue(payload.authority), multiMode: payload.type === 'multi' });
  } else if (network === 'httpupgrade') {
    Object.assign(objectValue(stream.httpupgradeSettings), { host, path });
  } else if (network === 'xhttp') {
    const xhttp = objectValue(stream.xhttpSettings);
    Object.assign(xhttp, { host, path });
    if (payload.mode) xhttp.mode = payload.mode;
    applyXhttpObject(xhttp, payload);
  }
}

function applyTransportParams(stream: JsonObject, params: URLSearchParams): void {
  const network = stringValue(stream.network, 'tcp');
  const host = params.get('host') || '';
  const path = params.get('path') || '/';
  if (network === 'ws') Object.assign(objectValue(stream.wsSettings), { host, path });
  if (network === 'grpc') Object.assign(objectValue(stream.grpcSettings), { serviceName: params.get('serviceName') || path, authority: params.get('authority') || '', multiMode: params.get('mode') === 'multi' });
  if (network === 'httpupgrade') Object.assign(objectValue(stream.httpupgradeSettings), { host, path });
  if (network === 'xhttp') {
    const xhttp = objectValue(stream.xhttpSettings);
    Object.assign(xhttp, { host, path });
    if (params.get('mode')) xhttp.mode = params.get('mode');
    applyXhttpParams(xhttp, params);
  }
  if (network === 'tcp' && (params.get('headerType') === 'http' || params.get('type') === 'http')) {
    stream.tcpSettings = { header: { type: 'http', request: { version: '1.1', method: 'GET', path: splitComma(path), headers: host ? { Host: splitComma(host) } : {} } } };
  }
}

function applySecurityParams(stream: JsonObject, params: URLSearchParams): void {
  if (stream.security === 'tls') {
    const tls = objectValue(stream.tlsSettings);
    tls.serverName = params.get('sni') || '';
    tls.fingerprint = params.get('fp') || '';
    tls.alpn = splitComma(params.get('alpn') || '');
    tls.echConfigList = params.get('ech') || '';
    tls.verifyPeerCertByName = params.get('vcn') || '';
    tls.pinnedPeerCertSha256 = params.get('pcs') || '';
  }
  if (stream.security === 'reality') {
    const reality = objectValue(stream.realitySettings);
    reality.serverName = params.get('sni') || '';
    reality.fingerprint = params.get('fp') || 'chrome';
    reality.publicKey = params.get('pbk') || '';
    reality.shortId = params.get('sid') || '';
    reality.spiderX = params.get('spx') || '';
    reality.mldsa65Verify = params.get('pqv') || '';
  }
}

function applyFinalMask(stream: JsonObject, params: URLSearchParams): void {
  const value = params.get('fm');
  if (!value) return;
  try {
    const parsed = JSON.parse(value) as unknown;
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) stream.finalmask = parsed;
  } catch {
    // Keep the rest of a valid link when an optional mask is malformed.
  }
}

function applyXhttpParams(xhttp: JsonObject, params: URLSearchParams): void {
  const extra = params.get('extra');
  if (extra) {
    try {
      applyXhttpObject(xhttp, JSON.parse(extra) as JsonObject);
    } catch {
      // Explicit query parameters still remain usable.
    }
  }
  const paddingAlias = params.get('x_padding_bytes');
  if (paddingAlias) xhttp.xPaddingBytes = paddingAlias;
  for (const key of xhttpStringKeys) if (params.get(key)) xhttp[key] = params.get(key);
  for (const key of xhttpNumberKeys) if (params.get(key)) xhttp[key] = Number(params.get(key)) || 0;
  for (const key of xhttpBooleanKeys) if (params.get(key)) xhttp[key] = ['true', '1'].includes(params.get(key) || '');
  if (!xhttp.sessionIDPlacement && params.get('sessionPlacement')) xhttp.sessionIDPlacement = params.get('sessionPlacement');
  if (!xhttp.sessionIDKey && params.get('sessionKey')) xhttp.sessionIDKey = params.get('sessionKey');
}

function applyXhttpObject(xhttp: JsonObject, source: JsonObject): void {
  for (const key of xhttpStringKeys) if (typeof source[key] === 'string') xhttp[key] = source[key];
  for (const key of xhttpNumberKeys) if (typeof source[key] === 'number') xhttp[key] = source[key];
  for (const key of xhttpBooleanKeys) if (typeof source[key] === 'boolean') xhttp[key] = source[key];
  for (const key of ['xmux', 'downloadSettings'] as const) if (Object.keys(objectValue(source[key])).length > 0) xhttp[key] = source[key];
  if (Object.keys(objectValue(source.headers)).length > 0) xhttp.headers = source.headers;
  if (!xhttp.sessionIDPlacement && typeof source.sessionPlacement === 'string') xhttp.sessionIDPlacement = source.sessionPlacement;
  if (!xhttp.sessionIDKey && typeof source.sessionKey === 'string') xhttp.sessionIDKey = source.sessionKey;
}

function splitEndpoint(value: string, fallbackPort: number): { host: string; port: number } {
  if (value.startsWith('[')) {
    const end = value.indexOf(']');
    if (end > 0) return { host: value.slice(1, end), port: Number(value.slice(end + 2)) || fallbackPort };
  }
  const separator = value.lastIndexOf(':');
  if (separator < 0) return { host: value, port: fallbackPort };
  return { host: value.slice(0, separator), port: Number(value.slice(separator + 1)) || fallbackPort };
}

function firstParam(params: URLSearchParams, ...keys: string[]): string | undefined {
  for (const key of keys) {
    const value = params.get(key);
    if (value) return value;
  }
  return undefined;
}

function decodeRemark(url: URL): string {
  try {
    return decodeURIComponent(url.hash.replace(/^#/, ''));
  } catch {
    return url.hash.replace(/^#/, '');
  }
}

function decodeBase64(value: string): string {
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(value.length / 4) * 4, '=');
  const binary = atob(normalized);
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}

function objectValue(value: unknown): JsonObject {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as JsonObject : {};
}

function stringValue(value: unknown, fallback = ''): string {
  return typeof value === 'string' ? value : value == null ? fallback : String(value);
}

function numberValue(value: unknown, fallback: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function splitComma(value: string): string[] {
  return value.split(',').map((item) => item.trim()).filter(Boolean);
}
