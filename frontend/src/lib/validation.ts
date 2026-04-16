import { z } from 'zod';

/** Accepts US phone numbers in common formats; normalizes to 10 digits. */
export const phoneSchema = z
  .string()
  .trim()
  .refine(
    (v) => {
      const digits = v.replace(/\D/g, '');
      return digits.length === 10 || (digits.length === 11 && digits.startsWith('1'));
    },
    { message: 'Enter a valid US phone number' },
  )
  .transform((v) => {
    const digits = v.replace(/\D/g, '');
    return digits.length === 11 ? digits.slice(1) : digits;
  });

export const emailSchema = z.string().trim().toLowerCase().email('Enter a valid email address');

export const stateCodeSchema = z.enum(['CA', 'TX', 'FL'], {
  errorMap: () => ({ message: 'Choose a supported state' }),
});

export const usStateRegionSchema = z
  .string()
  .trim()
  .length(2, 'Use 2-letter state code')
  .regex(/^[A-Z]{2}$/, 'Use uppercase 2-letter state code');

export const postalCodeSchema = z
  .string()
  .trim()
  .regex(/^\d{5}(-\d{4})?$/, 'Enter a valid US ZIP code');

export const dobSchema = z
  .string()
  .regex(/^\d{4}-\d{2}-\d{2}$/, 'Use YYYY-MM-DD')
  .refine((v) => !Number.isNaN(new Date(v).getTime()), 'Invalid date');

export const requiredString = (label = 'Required') =>
  z.string().trim().min(1, label);
