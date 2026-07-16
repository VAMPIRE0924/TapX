import { generateWireguardKeypair } from '../../../shared/wireguard';

type JsonObject = Record<string, unknown>;

const defaultSniffing = {
  enabled: false,
  destOverride: ['http', 'tls', 'quic', 'fakedns'],
  metadataOnly: false,
  routeOnly: false,
  ipsExcluded: [],
  domainsExcluded: [],
};

export function outboundSettingsToWire(protocol: string, input: JsonObject): JsonObject {
  switch (protocol) {
    case 'vmess':
      return {
        vnext: [{
          address: stringValue(input.address),
          port: numberValue(input.port, 443),
          users: [{ id: stringValue(input.id), security: stringValue(input.security, 'auto') }],
        }],
      };
    case 'vless':
      return vlessToWire(input);
    case 'trojan':
      return { servers: [{ address: stringValue(input.address), port: numberValue(input.port, 443), password: stringValue(input.password) }] };
    case 'shadowsocks':
      return {
        servers: [{
          address: stringValue(input.address),
          port: numberValue(input.port, 443),
          password: stringValue(input.password),
          method: stringValue(input.method, '2022-blake3-aes-128-gcm'),
          uot: input.uot === true,
          UoTVersion: numberValue(input.UoTVersion, 1),
        }],
      };
    case 'socks':
      return simpleProxyToWire(input, 1080);
    case 'http': {
      const result = simpleProxyToWire(input, 8080);
      const headers = objectValue(input.headers);
      if (Object.keys(headers).length > 0) result.headers = headers;
      return result;
    }
    case 'wireguard':
      return wireguardToWire(input);
    case 'freedom':
      return freedomToWire(input);
    case 'blackhole': {
      const type = stringValue(input.type);
      return type ? { response: { type } } : {};
    }
    case 'dns':
      return dnsToWire(input);
    case 'loopback': {
      const result: JsonObject = {};
      const inboundTag = stringValue(input.inboundTag);
      const sniffing = objectValue(input.sniffing);
      if (inboundTag) result.inboundTag = inboundTag;
      if (sniffing.enabled === true) result.sniffing = sniffingToWire(sniffing);
      return result;
    }
    default:
      return cloneObject(input);
  }
}

export function outboundSettingsFromWire(protocol: string, input: JsonObject): JsonObject {
  switch (protocol) {
    case 'vmess': {
      const server = objectValue(arrayValue(input.vnext)[0]);
      const user = objectValue(arrayValue(server.users)[0]);
      return {
        address: stringValue(server.address),
        port: numberValue(server.port, 443),
        id: stringValue(user.id),
        security: stringValue(user.security, 'auto'),
      };
    }
    case 'vless':
      return vlessFromWire(input);
    case 'trojan': {
      const server = objectValue(arrayValue(input.servers)[0]);
      return { address: stringValue(server.address), port: numberValue(server.port, 443), password: stringValue(server.password) };
    }
    case 'shadowsocks': {
      const server = objectValue(arrayValue(input.servers)[0]);
      return {
        address: stringValue(server.address),
        port: numberValue(server.port, 443),
        password: stringValue(server.password),
        method: stringValue(server.method, '2022-blake3-aes-128-gcm'),
        uot: server.uot === true,
        UoTVersion: numberValue(server.UoTVersion, 1),
      };
    }
    case 'socks':
      return simpleProxyFromWire(input, 1080);
    case 'http':
      return { ...simpleProxyFromWire(input, 8080), headers: objectValue(input.headers) };
    case 'wireguard':
      return wireguardFromWire(input);
    case 'freedom':
      return freedomFromWire(input);
    case 'blackhole': {
      const type = stringValue(objectValue(input.response).type);
      return { type: type === 'none' || type === 'http' ? type : '' };
    }
    case 'dns':
      return dnsFromWire(input);
    case 'loopback':
      return {
        inboundTag: stringValue(input.inboundTag),
        sniffing: sniffingFromWire(input.sniffing),
      };
    default:
      return cloneObject(input);
  }
}

export function outboundStreamToWire(input: JsonObject): JsonObject {
  const output = cloneObject(input);
  const xhttp = objectValue(output.xhttpSettings);
  if (Object.keys(xhttp).length > 0) {
    const xmuxEnabled = xhttp.enableXmux === true;
    delete xhttp.enableXmux;
    if (!xmuxEnabled) delete xhttp.xmux;
    dropEmptyStrings(xhttp);
    output.xhttpSettings = xhttp;
  }
  return output;
}

export function outboundStreamFromWire(input: JsonObject): JsonObject {
  const output = cloneObject(input);
  const xhttp = objectValue(output.xhttpSettings);
  if (Object.keys(xhttp).length > 0) {
    xhttp.enableXmux = Object.keys(objectValue(xhttp.xmux)).length > 0;
    output.xhttpSettings = xhttp;
  }
  return output;
}

function vlessToWire(input: JsonObject): JsonObject {
  const result: JsonObject = {
    address: stringValue(input.address),
    port: numberValue(input.port, 443),
    id: stringValue(input.id),
    flow: stringValue(input.flow),
    encryption: stringValue(input.encryption, 'none'),
  };
  const reverseTag = stringValue(input.reverseTag);
  if (reverseTag) {
    result.reverse = {
      tag: reverseTag,
      sniffing: sniffingToWire(objectValue(input.reverseSniffing)),
    };
  }
  if (result.flow === 'xtls-rprx-vision') {
    const testpre = numberValue(input.testpre, 0);
    const testseed = arrayValue(input.testseed).map(Number);
    if (testpre > 0) result.testpre = testpre;
    if (testseed.length === 4 && testseed.every((value) => Number.isInteger(value) && value > 0)) result.testseed = testseed;
  }
  return result;
}

function vlessFromWire(input: JsonObject): JsonObject {
  let address = stringValue(input.address);
  let port = numberValue(input.port, 443);
  let id = stringValue(input.id);
  let flow = stringValue(input.flow);
  let encryption = stringValue(input.encryption, 'none');
  const legacy = objectValue(arrayValue(input.vnext)[0]);
  if (Object.keys(legacy).length > 0) {
    const user = objectValue(arrayValue(legacy.users)[0]);
    address = stringValue(legacy.address);
    port = numberValue(legacy.port, 443);
    id = stringValue(user.id);
    flow = stringValue(user.flow);
    encryption = stringValue(user.encryption, 'none');
  }
  const reverse = objectValue(input.reverse);
  const testseed = arrayValue(input.testseed).map(Number);
  return {
    address,
    port,
    id,
    flow,
    encryption,
    reverseTag: stringValue(reverse.tag),
    reverseSniffing: sniffingFromWire(reverse.sniffing),
    testpre: numberValue(input.testpre, 0),
    testseed: testseed.length === 4 ? testseed : [900, 500, 900, 256],
  };
}

function simpleProxyToWire(input: JsonObject, defaultPort: number): JsonObject {
  const user = stringValue(input.user);
  return {
    servers: [{
      address: stringValue(input.address),
      port: numberValue(input.port, defaultPort),
      users: user ? [{ user, pass: stringValue(input.pass) }] : [],
    }],
  };
}

function simpleProxyFromWire(input: JsonObject, defaultPort: number): JsonObject {
  const server = objectValue(arrayValue(input.servers)[0]);
  const user = objectValue(arrayValue(server.users)[0]);
  return {
    address: stringValue(server.address),
    port: numberValue(server.port, defaultPort),
    user: stringValue(user.user),
    pass: stringValue(user.pass),
  };
}

function wireguardToWire(input: JsonObject): JsonObject {
  const result: JsonObject = {
    secretKey: stringValue(input.secretKey),
    address: splitComma(input.address),
    peers: arrayValue(input.peers).map((item) => {
      const peer = objectValue(item);
      return compactObject({
        publicKey: stringValue(peer.publicKey),
        preSharedKey: stringValue(peer.psk) || undefined,
        allowedIPs: arrayValue(peer.allowedIPs).map(String).filter(Boolean),
        endpoint: stringValue(peer.endpoint),
        keepAlive: numberValue(peer.keepAlive, 0) || undefined,
      });
    }),
    noKernelTun: input.noKernelTun === true,
  };
  const mtu = numberValue(input.mtu, 0);
  const strategy = stringValue(input.domainStrategy);
  const reserved = splitComma(input.reserved).map(Number).filter(Number.isFinite);
  if (mtu > 0) result.mtu = mtu;
  if (strategy) result.domainStrategy = strategy;
  if (reserved.length > 0) result.reserved = reserved;
  return result;
}

function wireguardFromWire(input: JsonObject): JsonObject {
  const secretKey = stringValue(input.secretKey);
  let publicKey = '';
  if (secretKey) {
    try {
      publicKey = generateWireguardKeypair(secretKey).publicKey;
    } catch {
      publicKey = '';
    }
  }
  return {
    mtu: numberValue(input.mtu, 1420),
    secretKey,
    pubKey: publicKey,
    address: arrayValue(input.address).map(String).join(','),
    domainStrategy: stringValue(input.domainStrategy),
    reserved: arrayValue(input.reserved).map(String).join(','),
    peers: arrayValue(input.peers).map((item) => {
      const peer = objectValue(item);
      const allowedIPs = arrayValue(peer.allowedIPs).map(String).filter(Boolean);
      return {
        publicKey: stringValue(peer.publicKey),
        psk: stringValue(peer.preSharedKey),
        allowedIPs: allowedIPs.length > 0 ? allowedIPs : ['0.0.0.0/0', '::/0'],
        endpoint: stringValue(peer.endpoint),
        keepAlive: numberValue(peer.keepAlive, 0),
      };
    }),
    noKernelTun: input.noKernelTun === true,
  };
}

function freedomToWire(input: JsonObject): JsonObject {
  const result: JsonObject = {};
  const fragment = objectValue(input.fragment);
  const fragmentEnabled = Boolean(fragment.length || fragment.interval || fragment.maxSplit);
  for (const key of ['domainStrategy', 'redirect'] as const) {
    const value = stringValue(input[key]);
    if (value) result[key] = value;
  }
  for (const key of ['userLevel', 'proxyProtocol'] as const) {
    const value = numberValue(input[key], 0);
    if (value) result[key] = value;
  }
  if (fragmentEnabled) result.fragment = compactObject(fragment);
  const noises = arrayValue(input.noises);
  if (noises.length > 0) result.noises = noises;
  const finalRules = arrayValue(input.finalRules).map((item) => {
    const rule = objectValue(item);
    const action = stringValue(rule.action, 'allow');
    return compactObject({
      action,
      network: stringValue(rule.network) || undefined,
      port: stringValue(rule.port) || undefined,
      ip: arrayValue(rule.ip).length > 0 ? arrayValue(rule.ip) : undefined,
      blockDelay: action === 'block' ? stringValue(rule.blockDelay) || undefined : undefined,
    });
  });
  if (finalRules.length > 0) result.finalRules = finalRules;
  return result;
}

function freedomFromWire(input: JsonObject): JsonObject {
  const fragment = objectValue(input.fragment);
  const finalRules = arrayValue(input.finalRules).map((item) => {
    const rule = objectValue(item);
    return {
      action: stringValue(rule.action) === 'allow' ? 'allow' : 'block',
      network: Array.isArray(rule.network) ? rule.network.map(String).join(',') : stringValue(rule.network),
      port: stringValue(rule.port),
      ip: arrayValue(rule.ip).map(String),
      blockDelay: stringValue(rule.blockDelay),
    };
  });
  if (finalRules.length === 0) {
    const legacyIPs = arrayValue(input.ipsBlocked).map(String).filter(Boolean);
    if (legacyIPs.length > 0) finalRules.push({ action: 'block', network: '', port: '', ip: legacyIPs, blockDelay: '' });
  }
  return {
    domainStrategy: stringValue(input.targetStrategy || input.domainStrategy),
    redirect: stringValue(input.redirect),
    userLevel: numberValue(input.userLevel, 0),
    proxyProtocol: numberValue(input.proxyProtocol, 0),
    fragment: Object.keys(fragment).length > 0
      ? {
        packets: stringValue(fragment.packets, '1-3'),
        length: stringValue(fragment.length),
        interval: stringValue(fragment.interval),
        maxSplit: stringValue(fragment.maxSplit),
      }
      : { packets: '', length: '', interval: '', maxSplit: '' },
    noises: arrayValue(input.noises),
    finalRules,
  };
}

function dnsToWire(input: JsonObject): JsonObject {
  const result: JsonObject = {};
  for (const key of ['rewriteNetwork', 'rewriteAddress'] as const) {
    const value = stringValue(input[key]);
    if (value) result[key] = value;
  }
  for (const key of ['rewritePort', 'userLevel'] as const) {
    const value = numberValue(input[key], 0);
    if (value) result[key] = value;
  }
  const rules = arrayValue(input.rules).map((item) => {
    const rule = objectValue(item);
    const resultRule: JsonObject = { action: ['direct', 'drop', 'return', 'hijack'].includes(stringValue(rule.action)) ? rule.action : 'direct' };
    const qType = stringValue(rule.qType).trim();
    const domains = stringValue(rule.domain).split(',').map((value) => value.trim()).filter(Boolean);
    const rCode = numberValue(rule.rCode, 0);
    if (qType) resultRule.qType = /^\d+$/.test(qType) ? Number(qType) : qType;
    if (domains.length > 0) resultRule.domain = domains;
    if (rCode > 0) resultRule.rCode = rCode;
    return resultRule;
  });
  if (rules.length > 0) result.rules = rules;
  return result;
}

function dnsFromWire(input: JsonObject): JsonObject {
  return {
    rewriteNetwork: stringValue(input.rewriteNetwork || input.network),
    rewriteAddress: stringValue(input.rewriteAddress || input.address),
    rewritePort: numberValue(input.rewritePort || input.port, 53),
    userLevel: numberValue(input.userLevel, 0),
    rules: arrayValue(input.rules).map((item) => {
      const rule = objectValue(item);
      return {
        action: ['direct', 'drop', 'return', 'hijack'].includes(stringValue(rule.action)) ? rule.action : 'direct',
        qType: Array.isArray(rule.qType) ? rule.qType.map(String).join(',') : stringValue(rule.qType),
        domain: Array.isArray(rule.domain) ? rule.domain.map(String).join(',') : stringValue(rule.domain),
        rCode: numberValue(rule.rCode, 0),
      };
    }),
  };
}

function sniffingToWire(input: JsonObject): JsonObject {
  return compactObject({
    enabled: input.enabled === true,
    destOverride: arrayValue(input.destOverride),
    metadataOnly: input.metadataOnly === true,
    routeOnly: input.routeOnly === true,
    ipsExcluded: arrayValue(input.ipsExcluded).length > 0 ? arrayValue(input.ipsExcluded) : undefined,
    domainsExcluded: arrayValue(input.domainsExcluded).length > 0 ? arrayValue(input.domainsExcluded) : undefined,
  });
}

function sniffingFromWire(input: unknown): JsonObject {
  const value = objectValue(input);
  const destinations = arrayValue(value.destOverride).map(String).filter(Boolean);
  return {
    enabled: value.enabled === true,
    destOverride: destinations.length > 0 ? destinations : [...defaultSniffing.destOverride],
    metadataOnly: value.metadataOnly === true,
    routeOnly: value.routeOnly === true,
    ipsExcluded: arrayValue(value.ipsExcluded).map(String),
    domainsExcluded: arrayValue(value.domainsExcluded).map(String),
  };
}

function objectValue(value: unknown): JsonObject {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as JsonObject : {};
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function stringValue(value: unknown, fallback = ''): string {
  return typeof value === 'string' ? value : value == null ? fallback : String(value);
}

function numberValue(value: unknown, fallback: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function splitComma(value: unknown): string[] {
  return stringValue(value).split(',').map((item) => item.trim()).filter(Boolean);
}

function compactObject(input: JsonObject): JsonObject {
  return Object.fromEntries(Object.entries(input).filter(([, value]) => value !== undefined && value !== ''));
}

function dropEmptyStrings(input: JsonObject): void {
  for (const [key, value] of Object.entries(input)) {
    if (value === '') delete input[key];
  }
}

function cloneObject(input: JsonObject): JsonObject {
  return JSON.parse(JSON.stringify(input)) as JsonObject;
}
