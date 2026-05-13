import { describe, it, expect, beforeAll, afterAll, vi } from 'vitest';
import {
  formatDate,
  formatDateShort,
  formatRelativeDays,
  formatPhone,
  formatCentsAsUSD,
  formatBytes,
  fullName,
} from './format';

describe('formatDate', () => {
  it('renders ISO timestamps in the en-US long format', () => {
    expect(formatDate('2026-05-29T00:00:00Z')).toMatch(/May 2[89], 2026/);
  });
  it('returns empty string for nullish input', () => {
    expect(formatDate(null)).toBe('');
    expect(formatDate(undefined)).toBe('');
    expect(formatDate('')).toBe('');
  });
  it('returns empty string for unparseable input rather than "Invalid Date"', () => {
    // The dashboard cannot render the literal text "Invalid Date" to a user;
    // empty string lets the CSS empty-state styling kick in.
    expect(formatDate('not a real date')).toBe('');
  });
});

describe('formatDateShort', () => {
  it('renders m/d/yy', () => {
    expect(formatDateShort('2026-05-29T00:00:00Z')).toMatch(/^5\/2[89]\/26$/);
  });
});

describe('formatRelativeDays', () => {
  // Pin "now" so the relative math is deterministic regardless of the
  // machine clock the test runs on. We feed Date objects (not ISO strings)
  // so the input doesn't get reinterpreted across timezones — the helper
  // intentionally works in local time per `setHours(0,0,0,0)`.
  const today = new Date(2026, 4, 13, 12, 0, 0); // May 13, 2026, noon local
  beforeAll(() => {
    vi.useFakeTimers();
    vi.setSystemTime(today);
  });
  afterAll(() => {
    vi.useRealTimers();
  });

  const day = (offset: number) => {
    const d = new Date(today);
    d.setDate(d.getDate() + offset);
    d.setHours(12, 0, 0, 0);
    return d;
  };

  it.each<[Date, string]>([
    [day(0), 'today'],
    [day(1), 'tomorrow'],
    [day(-1), 'yesterday'],
    [day(7), 'in 7 days'],
    [day(-7), '7 days ago'],
  ])('formats %s as %s', (input, want) => {
    expect(formatRelativeDays(input)).toBe(want);
  });
});

describe('formatPhone', () => {
  it.each([
    ['5551234567', '(555) 123-4567'],
    ['(555) 123-4567', '(555) 123-4567'],
    ['555-123-4567', '(555) 123-4567'],
    ['+15551234567', '(555) 123-4567'],
    ['15551234567', '(555) 123-4567'],
  ])('normalizes %s to %s', (input, want) => {
    expect(formatPhone(input)).toBe(want);
  });

  it('returns input unchanged when not a 10/11-digit US number', () => {
    expect(formatPhone('123')).toBe('123');
    expect(formatPhone('international')).toBe('international');
  });

  it('returns empty string for nullish', () => {
    expect(formatPhone(null)).toBe('');
    expect(formatPhone(undefined)).toBe('');
  });
});

describe('formatCentsAsUSD', () => {
  it('renders $99 for the canonical price', () => {
    // 9900¢ = $99 — the price-page floor. If this changes, marketing,
    // Stripe, and onboarding copy all need a coordinated update.
    expect(formatCentsAsUSD(9900)).toBe('$99');
  });
  it('drops fractional cents (consistent with the pricing UI)', () => {
    expect(formatCentsAsUSD(9950)).toBe('$100');
  });
});

describe('formatBytes', () => {
  it('renders bare bytes', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(1023)).toBe('1023 B');
  });
  it('rounds to KB/MB/GB', () => {
    expect(formatBytes(1024)).toBe('1.0 KB');
    expect(formatBytes(1024 * 1024 * 2.5)).toBe('2.5 MB');
  });
  it('returns empty for nullish', () => {
    expect(formatBytes(null)).toBe('');
    expect(formatBytes(undefined)).toBe('');
  });
});

describe('fullName', () => {
  it('joins first and last with a space', () => {
    expect(fullName('Maria', 'Rodriguez')).toBe('Maria Rodriguez');
  });
  it('drops missing parts cleanly', () => {
    expect(fullName('Maria', null)).toBe('Maria');
    expect(fullName(null, 'Rodriguez')).toBe('Rodriguez');
    expect(fullName(null, null)).toBe('');
  });
});
