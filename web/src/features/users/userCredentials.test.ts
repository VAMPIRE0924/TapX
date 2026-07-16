import { describe, expect, it } from 'vitest';
import {
  isShadowsocks2022Password,
  randomShadowsocksPassword,
  shadowsocksRequirements,
  validateUserCredentialSet,
} from './userCredentials';

describe('user credential composition', () => {
  it('reads Shadowsocks 2022 key sizes from attached listener profiles', () => {
    const requirement = shadowsocksRequirements(
      ['ss-128', 'ss-256'],
      [
        { ID: 'ss-128', Transport: 'xray', XrayProfileID: 'p128' },
        { ID: 'ss-256', Transport: 'xray', XrayProfileID: 'p256' },
      ],
      [
        { ID: 'p128', InboundProtocol: 'shadowsocks', InboundSettingsJSON: '{"method":"2022-blake3-aes-128-gcm"}' },
        { ID: 'p256', InboundProtocol: 'shadowsocks', InboundSettingsJSON: '{"method":"2022-blake3-aes-256-gcm"}' },
      ],
    );
    expect(requirement.keyBytes).toEqual([16, 32]);
  });

  it('generates and validates method-sized Shadowsocks keys', () => {
    expect(isShadowsocks2022Password(randomShadowsocksPassword(16), 16)).toBe(true);
    expect(isShadowsocks2022Password(randomShadowsocksPassword(32), 32)).toBe(true);
    expect(isShadowsocks2022Password('not-base64', 32)).toBe(false);
  });

  it('requires only credentials used by the attached protocols', () => {
    expect(validateUserCredentialSet({ ID: 'raw' }, ['raw-tcp', 'raw-udp'], { methods: [], keyBytes: [] })).toBeUndefined();
    expect(validateUserCredentialSet({ ID: 'xray' }, ['vless'], { methods: [], keyBytes: [] })).toContain('UUID');
    expect(validateUserCredentialSet({ ID: 'hy', UUID: 'id' }, ['hysteria'], { methods: [], keyBytes: [] })).toContain('Auth');
    expect(validateUserCredentialSet({ ID: 'wg' }, ['wireguard'], { methods: [], keyBytes: [] })).toBeUndefined();
  });
});
