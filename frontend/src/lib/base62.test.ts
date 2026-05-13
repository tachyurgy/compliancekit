import { describe, it, expect } from 'vitest';
import { encodeBase62, decodeBase62 } from './base62';

describe('base62 round-trip', () => {
  it('encodes empty input as empty output', () => {
    expect(encodeBase62(new Uint8Array())).toBe('');
    expect(decodeBase62('')).toEqual(new Uint8Array());
  });

  it('round-trips small fixed payloads', () => {
    const cases: Uint8Array[] = [
      new Uint8Array([0]),
      new Uint8Array([1]),
      new Uint8Array([255]),
      new Uint8Array([1, 2, 3, 4]),
      new Uint8Array([0, 0, 0, 1]),
      new Uint8Array([255, 255, 255, 255, 255]),
    ];
    for (const c of cases) {
      const enc = encodeBase62(c);
      const dec = decodeBase62(enc);
      expect(Array.from(dec)).toEqual(Array.from(c));
    }
  });

  it('round-trips a 16-byte magic-link token', () => {
    // Magic-link tokens are 16 bytes per ADR-008. The alphabet is 62, so
    // 16 bytes encodes to ~22 chars. Verify the shape so any future
    // alphabet change is caught here, not in prod.
    const bytes = new Uint8Array(16);
    for (let i = 0; i < bytes.length; i++) bytes[i] = (i * 37 + 11) & 0xff;
    const enc = encodeBase62(bytes);
    expect(enc.length).toBeGreaterThan(0);
    expect(enc.length).toBeLessThanOrEqual(22);
    const dec = decodeBase62(enc);
    expect(Array.from(dec)).toEqual(Array.from(bytes));
  });

  it('rejects characters outside the alphabet', () => {
    expect(() => decodeBase62('AAA!BBB')).toThrow();
  });
});
