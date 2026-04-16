/**
 * base62 codec — mirrors the Go implementation on the backend.
 *
 * Alphabet: 0-9A-Za-z, big-endian, no padding. Used for opaque tokens (magic links,
 * portal tokens, share links). The client needs this mainly for display truncation
 * and light-weight shape validation before round-tripping to the server.
 */

const ALPHABET = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz';
const BASE = BigInt(62);

export function encodeBase62(bytes: Uint8Array): string {
  if (bytes.length === 0) return '';
  // Convert bytes to bigint.
  let num = 0n;
  for (const b of bytes) {
    num = (num << 8n) | BigInt(b);
  }
  if (num === 0n) return '0'.repeat(bytes.length);
  let out = '';
  while (num > 0n) {
    const rem = num % BASE;
    num = num / BASE;
    out = ALPHABET[Number(rem)] + out;
  }
  // Preserve leading zero bytes.
  let leading = 0;
  for (const b of bytes) {
    if (b === 0) leading++;
    else break;
  }
  return '0'.repeat(leading) + out;
}

export function decodeBase62(str: string): Uint8Array {
  if (str.length === 0) return new Uint8Array();
  let num = 0n;
  for (const ch of str) {
    const idx = ALPHABET.indexOf(ch);
    if (idx < 0) throw new Error(`invalid base62 character: ${ch}`);
    num = num * BASE + BigInt(idx);
  }
  const bytes: number[] = [];
  while (num > 0n) {
    bytes.unshift(Number(num & 0xffn));
    num >>= 8n;
  }
  // Preserve leading '0' characters.
  let leading = 0;
  for (const ch of str) {
    if (ch === '0') leading++;
    else break;
  }
  while (leading-- > 0) bytes.unshift(0);
  return new Uint8Array(bytes);
}

/** Cheap shape validation for opaque tokens — rejects anything with non-base62 chars. */
export function isLikelyBase62Token(str: string, minLen = 8, maxLen = 256): boolean {
  if (str.length < minLen || str.length > maxLen) return false;
  for (const ch of str) {
    if (ALPHABET.indexOf(ch) < 0) return false;
  }
  return true;
}

/** Truncate a token for display (first 6 + ellipsis + last 4). */
export function truncateToken(token: string): string {
  if (token.length <= 12) return token;
  return `${token.slice(0, 6)}\u2026${token.slice(-4)}`;
}
