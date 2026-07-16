const p = (1n << 255n) - 19n;
const a24 = 121665n;

function mod(value: bigint): bigint {
  const next = value % p;
  return next >= 0n ? next : next + p;
}

function powMod(base: bigint, exponent: bigint): bigint {
  let result = 1n;
  let nextBase = mod(base);
  let nextExponent = exponent;
  while (nextExponent > 0n) {
    if (nextExponent & 1n) result = mod(result * nextBase);
    nextBase = mod(nextBase * nextBase);
    nextExponent >>= 1n;
  }
  return result;
}

function bytesToBigIntLE(bytes: Uint8Array): bigint {
  let value = 0n;
  for (let index = bytes.length - 1; index >= 0; index -= 1) {
    value = (value << 8n) + BigInt(bytes[index]);
  }
  return value;
}

function bigIntToBytesLE(value: bigint): Uint8Array {
  const bytes = new Uint8Array(32);
  let next = mod(value);
  for (let index = 0; index < bytes.length; index += 1) {
    bytes[index] = Number(next & 0xffn);
    next >>= 8n;
  }
  return bytes;
}

function clampScalar(bytes: Uint8Array): Uint8Array {
  const scalar = new Uint8Array(bytes);
  scalar[0] &= 248;
  scalar[31] &= 127;
  scalar[31] |= 64;
  return scalar;
}

function conditionalSwap(swap: bigint, left: bigint, right: bigint): [bigint, bigint] {
  return swap === 1n ? [right, left] : [left, right];
}

function x25519(scalarBytes: Uint8Array, point = 9n): Uint8Array {
  const scalar = clampScalar(scalarBytes);
  const x1 = point;
  let x2 = 1n;
  let z2 = 0n;
  let x3 = x1;
  let z3 = 1n;
  let swap = 0n;

  const k = bytesToBigIntLE(scalar);
  for (let bit = 254n; bit >= 0n; bit -= 1n) {
    const kBit = (k >> bit) & 1n;
    swap ^= kBit;
    [x2, x3] = conditionalSwap(swap, x2, x3);
    [z2, z3] = conditionalSwap(swap, z2, z3);
    swap = kBit;

    const a = mod(x2 + z2);
    const aa = mod(a * a);
    const b = mod(x2 - z2);
    const bb = mod(b * b);
    const e = mod(aa - bb);
    const c = mod(x3 + z3);
    const d = mod(x3 - z3);
    const da = mod(d * a);
    const cb = mod(c * b);
    x3 = mod((da + cb) ** 2n);
    z3 = mod(x1 * mod((da - cb) ** 2n));
    x2 = mod(aa * bb);
    z2 = mod(e * mod(aa + a24 * e));
  }

  [x2, x3] = conditionalSwap(swap, x2, x3);
  [z2, z3] = conditionalSwap(swap, z2, z3);
  return bigIntToBytesLE(x2 * powMod(z2, p - 2n));
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function bytesToBase64Url(bytes: Uint8Array): string {
  return bytesToBase64(bytes).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

function base64ToBytes(value: string): Uint8Array {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index);
  return bytes;
}

export function generateWireguardKeypair(secretKey = ''): { privateKey: string; publicKey: string } {
  const privateBytes = new Uint8Array(new ArrayBuffer(32));
  if (secretKey) {
    privateBytes.set(clampScalar(base64ToBytes(secretKey)).subarray(0, 32));
  } else {
    globalThis.crypto.getRandomValues(privateBytes);
  }
  const clampedPrivate = clampScalar(privateBytes);
  const publicBytes = x25519(clampedPrivate);
  return {
    privateKey: secretKey || bytesToBase64(clampedPrivate),
    publicKey: bytesToBase64(publicBytes),
  };
}

export function wireguardPublicKeyFromPrivate(privateKey: string): string {
  try {
    if (base64ToBytes(privateKey).length !== 32) return '';
    return generateWireguardKeypair(privateKey).publicKey;
  } catch {
    return '';
  }
}

export function generateRealityX25519Keypair(): { privateKey: string; publicKey: string } {
  const privateBytes = new Uint8Array(new ArrayBuffer(32));
  globalThis.crypto.getRandomValues(privateBytes);
  const clampedPrivate = clampScalar(privateBytes);
  const publicBytes = x25519(clampedPrivate);
  return {
    privateKey: bytesToBase64Url(clampedPrivate),
    publicKey: bytesToBase64Url(publicBytes),
  };
}
