import { z } from 'zod';
import { apiFetch } from './client';

export const ImmunizationStatusSchema = z.enum([
  'up_to_date',
  'due_soon',
  'overdue',
  'exempt',
  'unknown',
]);

export const ChildSchema = z.object({
  id: z.string(),
  providerId: z.string(),
  firstName: z.string(),
  lastName: z.string(),
  dateOfBirth: z.string(),
  enrollmentDate: z.string().nullable(),
  parentEmail: z.string().email().nullable(),
  parentName: z.string().nullable(),
  parentPhone: z.string().nullable(),
  immunizationStatus: ImmunizationStatusSchema,
  requiredDocsCount: z.number().int(),
  completedDocsCount: z.number().int(),
  complianceStatus: z.enum(['compliant', 'warning', 'critical']),
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
