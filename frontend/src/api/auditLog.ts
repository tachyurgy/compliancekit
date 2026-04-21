/**
 * Client for the admin-only audit log viewer.
 *
 * The backend handler lives in `backend/internal/handlers/auditlog.go` and is
 * gated at the router with `middleware.RequireRole(RoleProviderAdmin)`.
 *
 * Every row returned from the server has metadata pre-decoded server-side so
 * the UI can render it directly as a JSON object without a second parse step.
 */
import { z } from 'zod';
import { apiFetch } from './client';

export const AuditLogActorKindSchema = z.enum([
  'system',
  'provider_admin',
  'staff',
  'parent',
  'webhook',
]);
export type AuditLogActorKind = z.infer<typeof AuditLogActorKindSchema>;

export const AuditLogItemSchema = z.object({
  id: z.string(),
  provider_id: z.string(),
  actor_kind: AuditLogActorKindSchema,
  actor_id: z.string().optional().default(''),
  actor_email: z.string().optional().default(''),
  action: z.string(),
  target_kind: z.string().optional().default(''),
  target_id: z.string().optional().default(''),
  metadata: z.record(z.string(), z.unknown()).default({}),
  ip: z.string().optional().default(''),
  user_agent: z.string().optional().default(''),
  created_at: z.string(),
});
export type AuditLogItem = z.infer<typeof AuditLogItemSchema>;

export const AuditLogResponseSchema = z.object({
  items: z.array(AuditLogItemSchema),
  next_cursor: z.string().optional().default(''),
});
export type AuditLogResponse = z.infer<typeof AuditLogResponseSchema>;

export interface AuditLogListFilters {
  /** Page size; server clamps to [1, 500]. */
  limit?: number;
  /** 0-based offset into the DESC-by-created_at result set. */
  offset?: number;
  /** Exact action string (e.g. "child.create"). */
  action?: string;
  /** Exact target_kind (e.g. "child", "staff", "document"). */
  target_kind?: string;
  /** ISO datetime; lower bound on created_at. */
  since?: string;
  /** ISO datetime; upper bound on created_at. */
  until?: string;
}

export const auditLogApi = {
  async list(filters: AuditLogListFilters = {}): Promise<AuditLogResponse> {
    const query: Record<string, string | number | undefined> = {};
    if (filters.limit !== undefined) query.limit = filters.limit;
    if (filters.offset !== undefined) query.offset = filters.offset;
    if (filters.action) query.action = filters.action;
    if (filters.target_kind) query.target_kind = filters.target_kind;
    if (filters.since) query.since = filters.since;
    if (filters.until) query.until = filters.until;
    const data = await apiFetch<unknown>('/api/audit-log', { query });
    return AuditLogResponseSchema.parse(data);
  },
};

/** Action strings emitted by the backend. Must mirror auditlog.Action* constants. */
export const AUDIT_LOG_ACTIONS = [
  'auth.login',
  'auth.signup',
  'provider.me.update',
  'child.create',
  'child.update',
  'child.delete',
  'staff.create',
  'staff.update',
  'staff.delete',
  'document.finalize',
  'document.delete',
  'drill.create',
  'drill.delete',
  'posting.update',
  'ratio.check',
  'inspection.start',
  'inspection.finalize',
] as const;

export type AuditLogAction = (typeof AUDIT_LOG_ACTIONS)[number];

/** Target kinds emitted by the backend. Must mirror auditlog.TargetKind* constants. */
export const AUDIT_LOG_TARGET_KINDS = [
  'provider',
  'user',
  'child',
  'staff',
  'document',
  'drill',
  'posting',
  'ratio',
  'inspection',
] as const;

export type AuditLogTargetKind = (typeof AUDIT_LOG_TARGET_KINDS)[number];
