import { z } from 'zod';
import { apiFetch } from './client';

export const StaffRoleSchema = z.enum(['director', 'lead_teacher', 'assistant', 'aide', 'cook', 'other']);

export const StaffSchema = z.object({
  id: z.string(),
  providerId: z.string(),
  firstName: z.string(),
  lastName: z.string(),
  email: z.string().email().nullable(),
  phone: z.string().nullable(),
  role: StaffRoleSchema,
  hireDate: z.string().nullable(),
  backgroundCheckStatus: z.enum(['pending', 'cleared', 'failed', 'expired']),
  backgroundCheckDate: z.string().nullable(),
  trainingHoursYTD: z.number(),
  trainingHoursRequired: z.number(),
  complianceStatus: z.enum(['compliant', 'warning', 'critical']),
  updatedAt: z.string(),
});
export type Staff = z.infer<typeof StaffSchema>;

export const CertificationSchema = z.object({
  id: z.string(),
  staffId: z.string(),
  name: z.string(),
  issuer: z.string().nullable(),
  issuedDate: z.string().nullable(),
  expirationDate: z.string().nullable(),
  status: z.enum(['valid', 'expiring_soon', 'expired', 'missing']),
  documentId: z.string().nullable(),
});
export type Certification = z.infer<typeof CertificationSchema>;

export const StaffDetailSchema = StaffSchema.extend({
  certifications: z.array(CertificationSchema),
  requiredCertifications: z.array(
    z.object({
      slug: z.string(),
      label: z.string(),
      present: z.boolean(),
    }),
  ),
});
export type StaffDetail = z.infer<typeof StaffDetailSchema>;

export const staffApi = {
  async list(): Promise<Staff[]> {
    const data = await apiFetch<unknown>('/api/staff');
    return z.array(StaffSchema).parse(data);
  },
  async get(id: string): Promise<StaffDetail> {
    const data = await apiFetch<unknown>(`/api/staff/${encodeURIComponent(id)}`);
    return StaffDetailSchema.parse(data);
  },
  async create(input: Partial<Staff>): Promise<Staff> {
    const data = await apiFetch<unknown>('/api/staff', { method: 'POST', json: input });
    return StaffSchema.parse(data);
  },
  async update(id: string, input: Partial<Staff>): Promise<Staff> {
    const data = await apiFetch<unknown>(`/api/staff/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      json: input,
    });
    return StaffSchema.parse(data);
  },
  async remove(id: string): Promise<void> {
    await apiFetch(`/api/staff/${encodeURIComponent(id)}`, { method: 'DELETE' });
  },
  async sendStaffPortalLink(
    id: string,
    opts: { send?: 'email' } = {},
  ): Promise<{
    url: string;
    expires_at: string;
    subject_id: string;
    subject_kind: 'staff';
    emailed: boolean;
  }> {
    return apiFetch(`/api/staff/${encodeURIComponent(id)}/portal-link`, {
      method: 'POST',
      query: opts.send ? { send: opts.send } : undefined,
    });
  },
};
