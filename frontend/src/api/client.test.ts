import { describe, it, expect } from 'vitest';
import { z } from 'zod';
import { ApiError, humanizeError } from './client';

describe('humanizeError', () => {
  it('returns ApiError.message when the err is an ApiError', () => {
    const err = new ApiError(404, { code: 'not_found', message: 'Child not found' });
    expect(humanizeError(err)).toBe('Child not found');
  });

  it('returns ApiError fallback when ApiError carries an empty message', () => {
    const err = new ApiError(500, { code: 'server', message: '' });
    expect(humanizeError(err, 'oops')).toBe('oops');
  });

  // The bug Cosmo Kramer reproduced: when /children API returns a row with
  // an empty parentEmail, Zod throws and its `.message` is a 4 KB JSON
  // dump. Children.tsx rendered the whole dump in red. humanizeError must
  // catch any structured payload, regardless of its length.
  it('replaces ZodError JSON dumps with the fallback', () => {
    let zodErr: Error | null = null;
    try {
      z.object({ a: z.string() }).parse({ a: 1 });
    } catch (e) {
      zodErr = e as Error;
    }
    expect(zodErr).toBeTruthy();
    expect(humanizeError(zodErr)).toMatch(/something went wrong/i);
    expect(humanizeError(zodErr)).not.toContain('invalid_type');
  });

  it('replaces a raw JSON-looking string with the fallback', () => {
    const looksLikeJson = '[{"code":"invalid_type","path":[0,"providerId"]}]';
    expect(humanizeError(new Error(looksLikeJson), 'try again')).toBe(
      'try again',
    );
  });

  it('passes plain short messages through unchanged', () => {
    expect(humanizeError(new Error('Could not save the child.'))).toBe(
      'Could not save the child.',
    );
  });

  it('replaces very long messages with the fallback', () => {
    const long = 'x'.repeat(500);
    expect(humanizeError(new Error(long), 'fallback')).toBe('fallback');
  });

  it('returns fallback for nullish inputs', () => {
    expect(humanizeError(null, 'fb')).toBe('fb');
    expect(humanizeError(undefined, 'fb')).toBe('fb');
  });
});
