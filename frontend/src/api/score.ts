/**
 * REQ-013 client: GET /api/compliance/score.
 *
 * Returns the synthesised dashboard score (0-100), its calm band, the four
 * highest-impact "Gap" cards, and the per-category breakdown. The dashboard
 * hero card binds directly to this shape.
 */
import { z } from 'zod';
import { apiFetch } from './client';

export const ScoreBandSchema = z.enum(['excellent', 'solid', 'attention', 'urgent']);
export type ScoreBand = z.infer<typeof ScoreBandSchema>;

export const GapActionSchema = z.object({
  kind: z.string(),
  title: z.string(),
  subtitle: z.string().default(''),
  cta_label: z.string(),
  cta_route: z.string(),
  staff_id: z.string().optional().default(''),
  child_id: z.string().optional().default(''),
  due_at: z.string().nullable().optional(),
});
export type GapAction = z.infer<typeof GapActionSchema>;

export const BreakdownRowSchema = z.object({
  category: z.string(),
  deduction: z.number().int(),
  count: z.number().int(),
});
export type BreakdownRow = z.infer<typeof BreakdownRowSchema>;

export const ScoreSchema = z.object({
  score: z.number().int().min(0).max(100),
  band: ScoreBandSchema,
  last_calculated_at: z.string(),
  top_gaps: z.array(GapActionSchema).default([]),
  breakdown: z.array(BreakdownRowSchema).default([]),
});
export type ScoreData = z.infer<typeof ScoreSchema>;

export const scoreApi = {
  async get(): Promise<ScoreData> {
    const data = await apiFetch<unknown>('/api/compliance/score');
    return ScoreSchema.parse(data);
  },
  async history(days = 30): Promise<{ days: number; history: Array<{ score: number; band: ScoreBand; calculated_at: string }> }> {
    return await apiFetch(`/api/compliance/score/history?days=${days}`);
  },
};
