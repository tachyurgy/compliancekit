import { z } from 'zod';
import { apiFetch } from './client';

// REQ-042: Chase Mode dashboard. Three endpoints back this page:
//
//   GET  /api/chase/state    — outstanding chases + stuck items
//   GET  /api/chase/activity — recent nag log
//   POST /api/chase/trigger  — manual nag, dispatches through NagHandler

export const ChaseRowSchema = z.object({
  recipientKind: z.enum(['parent', 'staff']),
  recipientId: z.string(),
  recipientName: z.string(),
  recipientRole: z.string().default(''),
  recipientPhone: z.string().optional().default(''),
  recipientEmail: z.string().optional().default(''),
  itemKind: z.string(),
  itemLabel: z.string(),
  dueOn: z.string().optional().default(''),
  channel: z.enum(['email', 'sms', 'both']).default('email'),
  lastNagAt: z.string().optional().default(''),
  lastNagChannel: z.string().optional().default(''),
  lastNagStatus: z.string().optional().default(''),
  nagCount: z.number().int().default(0),
  status: z.enum(['missing', 'expired', 'expiring_soon']),
});
export type ChaseRow = z.infer<typeof ChaseRowSchema>;

export const ChaseStateSchema = z.object({
  outstanding: z.array(ChaseRowSchema),
  stuck: z.array(ChaseRowSchema),
});
export type ChaseState = z.infer<typeof ChaseStateSchema>;

export const ChaseActivityItemSchema = z.object({
  id: z.string(),
  recipientKind: z.enum(['parent', 'staff']),
  recipientId: z.string(),
  recipientName: z.string(),
  itemLabel: z.string(),
  channel: z.string(),
  emailStatus: z.string().optional().default(''),
  smsStatus: z.string().optional().default(''),
  createdAt: z.string(),
});
export type ChaseActivityItem = z.infer<typeof ChaseActivityItemSchema>;

export const ChaseActivitySchema = z.object({
  items: z.array(ChaseActivityItemSchema),
});

export interface ChaseTriggerInput {
  recipientKind: 'parent' | 'staff';
  recipientId: string;
  docKind?: string;
  note?: string;
}

export const chaseApi = {
  async state(): Promise<ChaseState> {
    const data = await apiFetch<unknown>('/api/chase/state');
    return ChaseStateSchema.parse(data);
  },
  async activity(params?: { from?: string; to?: string }): Promise<ChaseActivityItem[]> {
    const data = await apiFetch<unknown>('/api/chase/activity', {
      query: params,
    });
    return ChaseActivitySchema.parse(data).items;
  },
  async trigger(input: ChaseTriggerInput): Promise<unknown> {
    return apiFetch('/api/chase/trigger', {
      method: 'POST',
      json: input,
    });
  },
};
