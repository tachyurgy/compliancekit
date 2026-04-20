/**
 * API client for the Facility & Operations feature:
 *   - Drill logs CRUD
 *   - Wall postings checklist
 *   - Staff:child ratio calculator
 *
 * Mirrors the Go backend handlers in internal/handlers/{drills,postings,ratio}.go.
 * All JSON shapes are wall-validated with Zod.
 */
import { z } from 'zod';
import { apiFetch } from './client';

// ---------------------------------------------------------------------------
// Drill logs
// ---------------------------------------------------------------------------

export const DrillKindSchema = z.enum([
  'fire',
  'tornado',
  'lockdown',
  'earthquake',
  'evacuation',
  'other',
]);
export type DrillKind = z.infer<typeof DrillKindSchema>;

export const DrillLogSchema = z.object({
  id: z.string(),
  provider_id: z.string(),
  drill_kind: DrillKindSchema,
  drill_date: z.string(),
  duration_seconds: z.number().int().optional().default(0),
  notes: z.string().optional().default(''),
  attachment_document_id: z.string().optional().default(''),
  logged_by_user_id: z.string().optional().default(''),
  created_at: z.string(),
  updated_at: z.string(),
});
export type DrillLog = z.infer<typeof DrillLogSchema>;

export interface DrillCreateInput {
  drill_kind: DrillKind;
  drill_date: string; // ISO datetime
  duration_seconds?: number;
  notes?: string;
  attachment_document_id?: string;
}

export interface DrillListFilters {
  kind?: DrillKind;
  from?: string; // ISO date or datetime
  to?: string;
}

// ---------------------------------------------------------------------------
// Wall postings
// ---------------------------------------------------------------------------

export const PostingItemSchema = z.object({
  key: z.string(),
  label: z.string(),
  state_specific: z.boolean(),
  required: z.boolean(),
  posted_at: z.string().nullable().optional(),
  photo_document_id: z.string().optional().default(''),
});
export type PostingItem = z.infer<typeof PostingItemSchema>;

export const PostingsResponseSchema = z.object({
  items: z.array(PostingItemSchema),
  all_required_posted: z.boolean(),
});
export type PostingsResponse = z.infer<typeof PostingsResponseSchema>;

// ---------------------------------------------------------------------------
// Ratio check
// ---------------------------------------------------------------------------

export interface RoomInput {
  label: string;
  age_months_low: number;
  age_months_high: number;
  children_present: number;
  staff_present: number;
}

export const RoomResultSchema = z.object({
  label: z.string(),
  ratio_cap: z.number().int(),
  actual_ratio: z.number(),
  in_ratio: z.boolean(),
});
export type RoomResult = z.infer<typeof RoomResultSchema>;

export const RatioCheckResponseSchema = z.object({
  ok: z.boolean(),
  rooms: z.array(RoomResultSchema),
  violated_rooms: z.array(z.string()),
});
export type RatioCheckResponse = z.infer<typeof RatioCheckResponseSchema>;

// ---------------------------------------------------------------------------
// API surface
// ---------------------------------------------------------------------------

export const operationsApi = {
  async listDrills(filters: DrillListFilters = {}): Promise<DrillLog[]> {
    const query: Record<string, string> = {};
    if (filters.kind) query.kind = filters.kind;
    if (filters.from) query.from = filters.from;
    if (filters.to) query.to = filters.to;
    const data = await apiFetch<unknown>('/api/drills', { query });
    return z.array(DrillLogSchema).parse(data);
  },

  async createDrill(input: DrillCreateInput): Promise<DrillLog> {
    const data = await apiFetch<unknown>('/api/drills', {
      method: 'POST',
      json: input,
    });
    return DrillLogSchema.parse(data);
  },

  async deleteDrill(id: string): Promise<void> {
    await apiFetch(`/api/drills/${encodeURIComponent(id)}`, { method: 'DELETE' });
  },

  async getPostings(): Promise<PostingsResponse> {
    const data = await apiFetch<unknown>('/api/facility/postings');
    return PostingsResponseSchema.parse(data);
  },

  async upsertPosting(
    key: string,
    input: { posted_at?: string; photo_document_id?: string; unpost?: boolean },
  ): Promise<PostingsResponse> {
    const data = await apiFetch<unknown>(
      `/api/facility/postings/${encodeURIComponent(key)}`,
      { method: 'PATCH', json: input },
    );
    return PostingsResponseSchema.parse(data);
  },

  async checkRatio(rooms: RoomInput[]): Promise<RatioCheckResponse> {
    const data = await apiFetch<unknown>('/api/facility/ratio-check', {
      method: 'POST',
      json: { rooms },
    });
    return RatioCheckResponseSchema.parse(data);
  },
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Drill cadence expected by each state, in days.
 * - CA requires monthly (22 CCR §101174) — ~30d.
 * - TX requires monthly fire drills (26 TAC §746.5301) — ~30d.
 * - FL requires monthly fire drills (65C-22.002(7)) — ~30d.
 * Tornado/lockdown cadences vary; 90d is a defensive default.
 */
export const DRILL_CADENCE_DAYS: Record<DrillKind, number> = {
  fire: 30,
  evacuation: 30,
  tornado: 90,
  lockdown: 90,
  earthquake: 90,
  other: 90,
};

export function drillLabel(kind: DrillKind): string {
  return kind.charAt(0).toUpperCase() + kind.slice(1);
}
