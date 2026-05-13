import { describe, it, expect } from 'vitest';
import {
  phoneSchema,
  emailSchema,
  stateCodeSchema,
  postalCodeSchema,
  dobSchema,
} from './validation';

describe('phoneSchema', () => {
  it('normalizes a variety of US shapes to 10 digits', () => {
    expect(phoneSchema.parse('(555) 123-4567')).toBe('5551234567');
    expect(phoneSchema.parse('555-123-4567')).toBe('5551234567');
    expect(phoneSchema.parse('+1 555 123 4567')).toBe('5551234567');
    expect(phoneSchema.parse('1-555-123-4567')).toBe('5551234567');
  });
  it('rejects too-short and too-long inputs', () => {
    expect(() => phoneSchema.parse('123')).toThrow();
    expect(() => phoneSchema.parse('555-123-456')).toThrow();
    expect(() => phoneSchema.parse('+44 20 7946 0958')).toThrow();
  });
});

describe('emailSchema', () => {
  it('lowercases and trims valid emails', () => {
    expect(emailSchema.parse('  Maria@Example.COM ')).toBe('maria@example.com');
  });
  it('rejects malformed emails', () => {
    expect(() => emailSchema.parse('not-an-email')).toThrow();
    expect(() => emailSchema.parse('@example.com')).toThrow();
  });
});

describe('stateCodeSchema', () => {
  it('accepts the three launch states', () => {
    expect(stateCodeSchema.parse('FL')).toBe('FL');
    expect(stateCodeSchema.parse('TX')).toBe('TX');
    expect(stateCodeSchema.parse('CA')).toBe('CA');
  });
  it('rejects unsupported states — the rule packs only cover FL/TX/CA today', () => {
    expect(() => stateCodeSchema.parse('NY')).toThrow();
    expect(() => stateCodeSchema.parse('fl')).toThrow();
  });
});

describe('postalCodeSchema', () => {
  it.each(['33101', '33101-1234'])('accepts %s', (z) => {
    expect(postalCodeSchema.parse(z)).toBe(z);
  });
  it.each(['3310', '331011', 'abcde'])('rejects %s', (z) => {
    expect(() => postalCodeSchema.parse(z)).toThrow();
  });
});

describe('dobSchema', () => {
  it('requires YYYY-MM-DD shape', () => {
    expect(dobSchema.parse('2022-01-15')).toBe('2022-01-15');
    expect(() => dobSchema.parse('1/15/2022')).toThrow();
    expect(() => dobSchema.parse('2022-1-15')).toThrow();
  });
  it('rejects calendar-impossible dates', () => {
    expect(() => dobSchema.parse('2022-13-01')).toThrow();
  });
});
