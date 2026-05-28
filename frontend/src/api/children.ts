import { z } from 'zod';
import { apiFetch } from './client';

export const ImmunizationStatusSchema = z.enum([
  'up_to_date',
  'due_soon',
  'overdue',
  'exempt',
  'unknown',
]);

/**
 * Tolerant of legacy/empty fields. Earlier this schema rejected the entire
 * roster when ANY child row carried an empty parentEmail (`""` fails .email()
 * yet isn't null), or had a missing optional column. Operators saw "We
 * couldn't load your roster" with a 4 KB JSON Zod dump in red. Accept what
 * the backend returns; let the UI render dashes for blanks.
 */
const optionalString = z
  .union([z.string(), z.null(), z.undefined()])
  .transform((v) => v ?? null);

export const ChildSchema = z.object({
  id: z.string(),
  providerId: z.string(),
  firstName: z.string(),
  lastName: z.string(),
  dateOfBirth: z.string(),
  enrollmentDate: optionalString,
  parentEmail: optionalString,
  parentName: optionalString,
  parentPhone: optionalString,
  immunizationStatus: ImmunizationStatusSchema.catch('unknown'),
  requiredDocsCount: z.number().int().catch(0),
  completedDocsCount: z.number().int().catch(0),
  complianceStatus: z.enum(['compliant', 'warning', 'critical']).catch('warning'),
  updatedAt: z.string(),
});
export type Child = z.infer<typeof ChildSchema>;

export const ChildDetailSchema = ChildSchema.extend({
  notes: z.string().nullable(),
  requiredDocs: z.array(
    z.object({
      id: z.string(),
      documentType: z.string(),
      label: z.string(),
      status: z.enum(['missing', 'pending', 'complete', 'expired']),
      dueDate: z.string().nullable(),
      documentId: z.string().nullable(),
    }),
  ),
  immunizations: z.array(
    z.object({
      vaccine: z.string(),
      status: z.enum(['up_to_date', 'due_soon', 'overdue', 'exempt']),
      dueDate: z.string().nullable(),
      lastDose: z.string().nullable(),
    }),
  ),
});
export type ChildDetail = z.infer<typeof ChildDetailSchema>;

export const childrenApi = {
  async list(): Promise<Child[]> {
    const data = await apiFetch<unknown>('/api/children');
    return z.array(ChildSchema).parse(data);
  },
  async get(id: string): Promise<ChildDetail> {
    const data = await apiFetch<unknown>(`/api/children/${encodeURIComponent(id)}`);
    return ChildDetailSchema.parse(data);
  },
  async create(input: Partial<Child>): Promise<Child> {
    const data = await apiFetch<unknown>('/api/children', { method: 'POST', json: input });
    return ChildSchema.parse(data);
  },
  async update(id: string, input: Partial<Child>): Promise<Child> {
    const data = await apiFetch<unknown>(`/api/children/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      json: input,
    });
    return ChildSchema.parse(data);
  },
  async remove(id: string): Promise<void> {
    await apiFetch(`/api/children/${encodeURIComponent(id)}`, { method: 'DELETE' });
  },
  async sendParentPortalLink(
    id: string,
    opts: { send?: 'email' } = {},
  ): Promise<{
    url: string;
    expires_at: string;
    subject_id: string;
    subject_kind: 'child';
    emailed: boolean;
  }> {
    return apiFetch(`/api/children/${encodeURIComponent(id)}/portal-link`, {
      method: 'POST',
      query: opts.send ? { send: opts.send } : undefined,
    });
  },
};
