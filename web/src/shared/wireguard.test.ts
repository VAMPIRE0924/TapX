import { describe, expect, it } from 'vitest';
import { generateWireguardKeypair, wireguardPublicKeyFromPrivate } from './wireguard';

describe('WireGuard keys', () => {
  it('derives the same public key from a generated private key', () => {
    const pair = generateWireguardKeypair();
    expect(wireguardPublicKeyFromPrivate(pair.privateKey)).toBe(pair.publicKey);
  });

  it('rejects malformed private keys', () => {
    expect(wireguardPublicKeyFromPrivate('invalid')).toBe('');
  });
});
