import { z } from 'zod';
import { apiFetch } from './client';

// Roles the FL launch supports. Backend keeps these as free-text TEXT
// columns so future states can extend without a migration; the frontend
// constrains the dropdown choices.
export const StaffRoleSchema = z.enum([
  'owner',
  'director',
  'lead_teacher',
  'assistant_teacher',
  'aide',
  'cook',
  'driver',
  'volunteer',
]);
export type StaffRole = z.infer<typeof StaffRoleSchema>;

// brand-voice.md status pills. Mirrors backend DocStatus.
export const StatusPillSchema = z.enum([
  'on_file',
  'expiring_soon',
  'expired',
  'missing',
  'awaiting_upload',
]);
export type StatusPill = z.infer<typeof StatusPillSchema>;

export const RollupSchema = z.object({
  onFile: z.number().int(),
  expiringSoon: z.number().int(),
  expired: z.number().int(),
  missing: z.number().int(),
  awaitingUpload: z.number().int(),
  total: z.number().int(),
  status: StatusPillSchema,
});
export type Rollup = z.infer<typeof RollupSchema>;

export const StaffListRowSchema = z.object({
  id: z.string(),
  firstName: z.string(),
  lastName: z.string(),
  role: z.string(),
  email: z.string().default(''),
  phone: z.string().default(''),
  preferredChannel: z.enum(['email', 'sms', 'both']).default('email'),
  hireDate: z.string().optional().default(''),
  status: z.string().default('active'),
  rollup: RollupSchema,
});
export type StaffListRow = z.infer<typeof StaffListRowSchema>;

export const RequirementStatusSchema = z.object({
  docKind: z.string(),
  label: z.string(),
  authority: z.string().optional().default(''),
  frequency: z.string().optional().default(''),
  status: StatusPillSchema,
  issuedOn: z.string().nullable().optional(),
  expiresOn: z.string().nullable().optional(),
  documentId: z.string().optional().default(''),
  source: z.string().optional().default(''),
  sortOrder: z.number().int(),
});
export type RequirementStatus = z.infer<typeof RequirementStatusSchema>;

export const OtherDocumentSchema = z.object({
  id: z.string(),
  kind: z.string(),
  title: z.string(),
  issuedOn: z.string().nullable().optional(),
  expiresOn: z.string().nullable().optional(),
  uploadedAt: z.string(),
  source: z.string().optional().default(''),
});
export type OtherDocument = z.infer<typeof OtherDocumentSchema>;

export const ActivityEventSchema = z.object({
  id: z.string(),
  kind: z.string(),
  summary: z.string(),
  detail: z.string().optional().default(''),
  actor: z.string().optional().default(''),
  createdAt: z.string(),
});
export type ActivityEvent = z.infer<typeof ActivityEventSchema>;

export const StaffDetailSchema = z.object({
  id: z.string(),
  firstName: z.string(),
  lastName: z.string(),
  role: z.string(),
  email: z.string().default(''),
  phone: z.string().default(''),
  preferredChannel: z.enum(['email', 'sms', 'both']).default('email'),
  hireDate: z.string().optional().default(''),
  status: z.string().default('active'),
  archivedAt: z.string().nullable().optional(),
  rollup: RollupSchema,
  requiredDocuments: z.array(RequirementStatusSchema),
  otherDocuments: z.array(OtherDocumentSchema).default([]),
  activity: z.array(ActivityEventSchema).default([]),
});
export type StaffDetail = z.infer<typeof StaffDetailSchema>;

export interface CreateStaffInput {
  first_name: string;
  last_name: string;
  email: string;
  phone?: string;
  role: string;
  hire_date?: string;
  preferred_contact_channel?: 'email' | 'sms' | 'both';
}

export const staffApi = {
  async list(filter?: { status?: 'passing' | 'missing' | 'expired' }): Promise<StaffListRow[]> {
    const data = await apiFetch<unknown>('/api/staff', {
      query: filter?.status ? { status: filter.status } : undefined,
    });
    return z.array(StaffListRowSchema).parse(data);
  },
  async get(id: string): Promise<StaffDetail> {
    const data = await apiFetch<unknown>(`/api/staff/${encodeURIComponent(id)}`);
    return StaffDetailSchema.parse(data);
  },
  async create(input: CreateStaffInput): Promise<{ id: string }> {
    const data = await apiFetch<{ id: string }>('/api/staff', {
      method: 'POST',
      json: input,
    });
    return data;
  },
  async update(id: string, input: Partial<CreateStaffInput>): Promise<unknown> {
    return apiFetch(`/api/staff/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      json: input,
    });
  },
  async archive(id: string): Promise<void> {
    await apiFetch(`/api/staff/${encodeURIComponent(id)}/archive`, { method: 'POST' });
  },
  async sendReminder(
    id: string,
    body: { reason?: 'missing' | 'manual'; doc_kind?: string; note?: string },
  ): Promise<unknown> {
    return apiFetch(`/api/staff/${encodeURIComponent(id)}/nag`, {
      method: 'POST',
      json: body,
    });
  },
  async getUploadLink(id: string): Promise<{ url: string; expiresAt: string }> {
    return apiFetch<{ url: string; expiresAt: string }>(
      `/api/staff/${encodeURIComponent(id)}/upload-link`,
    );
  },
};

// Brand-voice copy for status pills. Use these strings everywhere — see
// active/product/brand-voice.md.
export const STATUS_PILL_LABEL: Record<StatusPill, string> = {
  on_file: 'On file',
  expiring_soon: 'Expiring soon',
  expired: 'Expired',
  missing: 'Missing',
  awaiting_upload: 'Awaiting upload',
};

// Operator-facing role labels (display only).
export const ROLE_LABEL: Record<string, string> = {
  owner: 'Owner',
  director: 'Director',
  lead_teacher: 'Lead teacher',
  assistant_teacher: 'Assistant teacher',
  aide: 'Aide',
  cook: 'Cook',
  driver: 'Driver',
  volunteer: 'Volunteer',
};
