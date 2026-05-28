import { describe, it, expect } from 'vitest';
import { ChildSchema } from './children';

const baseRow = {
  id: 'kid_001',
  providerId: 'prv_001',
  firstName: 'Liam',
  lastName: 'Davis',
  dateOfBirth: '2022-03-04',
  enrollmentDate: '2024-08-15',
  parentEmail: 'karina@example.com',
  parentName: 'Karina Davis',
  parentPhone: '+13055550100',
  immunizationStatus: 'up_to_date',
  requiredDocsCount: 5,
  completedDocsCount: 4,
  complianceStatus: 'warning',
  updatedAt: '2026-05-24T12:00:00Z',
};

describe('ChildSchema', () => {
  it('parses a canonical row', () => {
    const r = ChildSchema.parse(baseRow);
    expect(r.firstName).toBe('Liam');
    expect(r.parentEmail).toBe('karina@example.com');
  });

  // Regression for the user-pasted error: Children.tsx showed a 4 KB Zod
  // dump because the backend returned `""` for parentEmail/parentName/
  // parentPhone (COALESCE default) and the schema demanded a valid email
  // or null. The schema must accept empty strings and missing keys without
  // throwing; the UI renders dashes for blanks.
  it('accepts empty-string parent fields', () => {
    const r = ChildSchema.parse({
      ...baseRow,
      parentEmail: '',
      parentName: '',
      parentPhone: '',
    });
    // Empty strings flow through as-is (we don't normalize to null because
    // the rest of the app uses ?? '—' to render). What matters is no throw.
    expect(r.parentEmail).toBe('');
  });

  it('accepts null parent fields', () => {
    const r = ChildSchema.parse({
      ...baseRow,
      parentEmail: null,
      parentName: null,
      parentPhone: null,
      enrollmentDate: null,
    });
    expect(r.parentEmail).toBeNull();
    expect(r.enrollmentDate).toBeNull();
  });

  it('treats missing optional parent keys as null', () => {
    const { parentEmail: _e, parentName: _n, parentPhone: _p, ...rest } =
      baseRow;
    const r = ChildSchema.parse(rest);
    expect(r.parentEmail).toBeNull();
    expect(r.parentName).toBeNull();
    expect(r.parentPhone).toBeNull();
  });

  it('falls back to safe defaults for unknown immunizationStatus/complianceStatus', () => {
    const r = ChildSchema.parse({
      ...baseRow,
      immunizationStatus: 'fake_status_from_old_db',
      complianceStatus: 'not_a_bucket',
    });
    expect(r.immunizationStatus).toBe('unknown');
    expect(r.complianceStatus).toBe('warning');
  });

  it('still rejects rows missing the truly required fields', () => {
    // providerId/firstName/lastName/dateOfBirth — the four fields the user
    // actually saw "undefined" for. If the API genuinely omits these, we
    // want Zod to throw (and humanizeError to convert the throw into the
    // calm fallback message). Test confirms the schema hasn't become so
    // lax that broken rows pass through silently.
    expect(() => ChildSchema.parse({})).toThrow();
  });
});
