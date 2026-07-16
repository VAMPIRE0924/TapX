import type { TapxClient, TapxEndpoint, TapxXrayProfile } from '../../shared/api';
import type { TranslationKey, TranslationValues } from '../../i18n/dictionaries';
import { randomBase64 } from '../../shared/random';

type NodeOwned = { ManagedNodeID?: string };

function nodeIDOf(value: NodeOwned | undefined): string {
  return value?.ManagedNodeID || 'local';
}

type Translate = (key: TranslationKey, values?: TranslationValues) => string;

export type ShadowsocksRequirement = {
  methods: string[];
  keyBytes: number[];
};

export function shadowsocksRequirements(
  listenerIDs: string[],
  listeners: TapxEndpoint[],
  profiles: TapxXrayProfile[],
  ownerID = 'local',
): ShadowsocksRequirement {
  const listenerByID = new Map(listeners
    .filter((listener) => nodeIDOf(listener as NodeOwned) === ownerID)
    .map((listener) => [listener.ID, listener]));
  const profileByID = new Map(profiles
    .filter((profile) => nodeIDOf(profile as NodeOwned) === ownerID)
    .map((profile) => [profile.ID, profile]));
  const methods = listenerIDs.flatMap((id) => {
    const listener = listenerByID.get(id);
    const profile = listener?.XrayProfileID ? profileByID.get(listener.XrayProfileID) : undefined;
    if (profile?.InboundProtocol !== 'shadowsocks') return [];
    const settings = parseObject(profile.InboundSettingsJSON);
    const method = typeof settings.method === 'string' ? settings.method : '';
    return method.startsWith('2022-') ? [method] : [];
  });
  return {
    methods: [...new Set(methods)],
    keyBytes: [...new Set(methods.map(shadowsocks2022KeyBytes))],
  };
}

export function randomShadowsocksPassword(keyBytes = 32): string {
  return randomBase64(keyBytes);
}

export function isShadowsocks2022Password(value: string, keyBytes: number): boolean {
  try {
    return atob(value).length === keyBytes;
  } catch {
    return false;
  }
}

export function validateUserCredentialSet(
  user: TapxClient,
  protocols: string[],
  shadowsocks: ShadowsocksRequirement,
  t?: Translate,
): string | undefined {
  if (protocols.some((protocol) => protocol === 'vless' || protocol === 'vmess') && !user.UUID?.trim()) {
    return t ? t('user.credentialUuidRequired') : 'Associated VLESS/VMess listeners require a UUID';
  }
  if (protocols.some((protocol) => protocol === 'trojan' || protocol === 'shadowsocks') && !user.Password?.trim()) {
    return t ? t('user.credentialPasswordRequired') : 'Associated Trojan/Shadowsocks listeners require a password';
  }
  if (shadowsocks.keyBytes.length > 1) {
    return t ? t('user.credentialSsConflict', { methods: shadowsocks.methods.join(', ') }) : `Associated Shadowsocks 2022 listeners require different key lengths (${shadowsocks.methods.join(', ')})`;
  }
  if (shadowsocks.keyBytes.length === 1 && !isShadowsocks2022Password(user.Password || '', shadowsocks.keyBytes[0])) {
    return t ? t('user.credentialSsLength', { bytes: shadowsocks.keyBytes[0] }) : `Shadowsocks 2022 password must be a ${shadowsocks.keyBytes[0]}-byte Base64 key`;
  }
  if (protocols.includes('hysteria') && !user.Auth?.trim()) {
    return t ? t('user.credentialAuthRequired') : 'Associated Hysteria listeners require Auth';
  }
  return undefined;
}

function shadowsocks2022KeyBytes(method: string): number {
  return method === '2022-blake3-aes-128-gcm' ? 16 : 32;
}

function parseObject(value?: string): Record<string, unknown> {
  try {
    const parsed = JSON.parse(value || '{}') as unknown;
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed as Record<string, unknown> : {};
  } catch {
    return {};
  }
}
