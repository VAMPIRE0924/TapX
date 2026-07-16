const lowerAndNumberAlphabet = 'abcdefghijklmnopqrstuvwxyz0123456789';
const hexAlphabet = '0123456789abcdef';

export function randomBytes(length: number): Uint8Array {
  if (!Number.isSafeInteger(length) || length < 0) throw new RangeError('length must be a non-negative integer');
  const bytes = new Uint8Array(length);
  globalThis.crypto.getRandomValues(bytes);
  return bytes;
}

export function randomText(length: number, alphabet: string): string {
  if (!Number.isSafeInteger(length) || length < 0) throw new RangeError('length must be a non-negative integer');
  if (alphabet.length < 2 || alphabet.length > 256) throw new RangeError('alphabet must contain 2 to 256 characters');

  const acceptedLimit = 256 - (256 % alphabet.length);
  let result = '';
  while (result.length < length) {
    const bytes = randomBytes(Math.max(16, length - result.length));
    for (const byte of bytes) {
      if (byte >= acceptedLimit) continue;
      result += alphabet[byte % alphabet.length];
      if (result.length === length) break;
    }
  }
  return result;
}

export function randomLowerAndNumber(length: number): string {
  return randomText(length, lowerAndNumberAlphabet);
}

export function randomHex(length: number): string {
  return randomText(length, hexAlphabet);
}

export function randomShortIds(): string[] {
  const lengths = [2, 4, 6, 8, 10, 12, 14, 16];
  for (let index = lengths.length - 1; index > 0; index -= 1) {
    const other = randomInteger(0, index);
    [lengths[index], lengths[other]] = [lengths[other], lengths[index]];
  }
  return lengths.map(randomHex);
}

export function randomBase64(byteLength = 16): string {
  let binary = '';
  for (const byte of randomBytes(byteLength)) binary += String.fromCharCode(byte);
  return btoa(binary);
}

export function randomUUID(): string {
  if (globalThis.crypto.randomUUID) return globalThis.crypto.randomUUID();
  const bytes = randomBytes(16);
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

export function randomInteger(minInclusive: number, maxInclusive: number): number {
  if (!Number.isSafeInteger(minInclusive) || !Number.isSafeInteger(maxInclusive) || maxInclusive < minInclusive) {
    throw new RangeError('invalid integer range');
  }
  const range = maxInclusive - minInclusive + 1;
  if (range > 0x1_0000_0000) throw new RangeError('integer range is too large');
  const acceptedLimit = Math.floor(0x1_0000_0000 / range) * range;
  const value = new Uint32Array(1);
  do globalThis.crypto.getRandomValues(value); while (value[0] >= acceptedLimit);
  return minInclusive + (value[0] % range);
}
