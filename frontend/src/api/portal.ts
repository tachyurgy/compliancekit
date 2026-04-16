import { z } from 'zod';
import { apiFetch } from './client';

export const PortalRequiredDocSchema = z.object({
  id: z.string(),
  documentType: z.string(),
  label: z.string(),
  description: z.string().nullable(),
  status: z.enum(['missing', 'pending', 'complete', 'expired']),
  required: z.boolean(),
});
export type PortalRequiredDoc = z.infer<typeof PortalRequiredDocSchema>;

export const ParentPortalSessionSchema = z.object({
  child: z.object({
    firstName: z.string(),
    lastName: z.string(),
  }),
  providerName: z.string(),
  requiredDocs: z.array(PortalRequiredDocSchema),
});
export type ParentPortalSession = z.infer<typeof ParentPortalSessionSchema>;

export const StaffPortalSessionSchema = z.object({
  staff: z.object({
    firstName: z.string(),
    lastName: z.string(),
  }),
  providerName: z.string(),
  requiredDocs: z.array(PortalRequiredDocSchema),
});
export type StaffPortalSession = z.infer<typeof StaffPortalSessionSchema>;

export const portalApi = {
  async parentSession(token: string): Promise<ParentPortalSession> {
    const data = await apiFetch<unknown>(`/api/portal/parent/${encodeURIComponent(token)}`, {
      skipAuthRedirect: true,
    });
    return ParentPortalSessionSchema.parse(data);
  },
  async parentUpload(
    token: string,
    requiredDocId: string,
    file: File,
  ): Promise<{ ok: true }> {
    const fd = new FormData();
    fd.append('file', file);
    fd.append('requiredDocId', requiredDocId);
    await apiFetch(`/api/portal/parent/${encodeURIComponent(token)}/upload`, {
      method: 'POST',
      body: fd,
      skipAuthRedirect: true,
    });
    return { ok: true };
  },
  async staffSession(token: string): Promise<StaffPortalSession> {
    const data = await apiFetch<unknown>(`/api/portal/staff/${encodeURIComponent(token)}`, {
      skipAuthRedirect: true,
    });
    return StaffPortalSessionSchema.parse(data);
  },
  async staffUpload(
    token: string,
    requiredDocId: string,
    file: File,
  ): Promise<{ ok: true }> {
    const fd = new FormData();
    fd.append('file', file);
    fd.append('requiredDocId', requiredDocId);
    await apiFetch(`/api/portal/staff/${encodeURIComponent(token)}/upload`, {
      method: 'POST',
      body: fd,
      skipAuthRedirect: true,
    });
    return { ok: true };
  },
};
