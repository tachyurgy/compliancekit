import { z } from 'zod';

import { API_BASE_URL, apiFetch } from './client';

/**
 * Inspection Readiness Simulator API client.
 *
 * Backend responses use snake_case JSON (Go struct tags). We parse them with
 * Zod and keep field names identical — no translation layer — because every
 * consumer of this module is local to the inspections feature.
 */

export const SeveritySchema = z.enum(['critical', 'major', 'minor']);
export type Severity = z.infer<typeof SeveritySchema>;

export const EvidenceKindSchema = z.enum([
  'none',
  'document',
  'photo',
  'attestation',
]);
export type EvidenceKind = z.infer<typeof EvidenceKindSchema>;

export const AnswerSchema = z.enum(['pass', 'fail', 'na']);
export type Answer = z.infer<typeof AnswerSchema>;

export const ItemSchema = z.object({
  id: z.string(),
  domain: z.string(),
  question: z.string(),
  reference: z.string(),
  form_ref: z.string(),
  evidence_kind: EvidenceKindSchema,
  severity: SeveritySchema,
});
export type Item = z.infer<typeof ItemSchema>;

export const DomainSchema = z.object({
  name: z.string(),
  item_count: z.number().int(),
  start_index: z.number().int(),
});
export type Domain = z.infer<typeof DomainSchema>;

export const ChecklistSchema = z.object({
  state: z.string(),
  form_ref: z.string(),
  items: z.array(ItemSchema),
  domains: z.array(DomainSchema),
});
export type Checklist = z.infer<typeof ChecklistSchema>;

export const InspectionRunSchema = z.object({
  id: z.string(),
  provider_id: z.string(),
  state: z.string(),
  form_ref: z.string().optional().default(''),
  started_at: z.string(),
  completed_at: z.string().nullable().optional(),
  score: z.number().int().nullable().optional(),
  total_items: z.number().int(),
  items_passed: z.number().int(),
  items_failed: z.number().int(),
  items_na: z.number().int(),
  items_answered: z.number().int(),
});
export type InspectionRun = z.infer<typeof InspectionRunSchema>;

export const ResponseSchema = z.object({
  item_id: z.string(),
  answer: AnswerSchema,
  note: z.string().optional().default(''),
  evidence_document_id: z.string().optional().default(''),
  answered_at: z.string(),
});
export type InspectionResponse = z.infer<typeof ResponseSchema>;

export const DomainBreakdownSchema = z.object({
  name: z.string(),
  total: z.number().int(),
  passed: z.number().int(),
  failed: z.number().int(),
  na: z.number().int(),
  unanswered: z.number().int(),
});
export type DomainBreakdown = z.infer<typeof DomainBreakdownSchema>;

export const CitationRiskSchema = z.object({
  item_id: z.string(),
  domain: z.string(),
  question: z.string(),
  reference: z.string(),
  form_ref: z.string(),
  severity: SeveritySchema,
  note: z.string().optional().default(''),
});
export type CitationRisk = z.infer<typeof CitationRiskSchema>;

export const RunDetailSchema = z.object({
  run: InspectionRunSchema,
  checklist: ChecklistSchema,
  responses: z.array(ResponseSchema).default([]),
  domain_breakdown: z.array(DomainBreakdownSchema).optional().default([]),
  predicted_citations: z.array(CitationRiskSchema).optional().default([]),
});
export type RunDetail = z.infer<typeof RunDetailSchema>;

export const inspectionsApi = {
  async list(): Promise<InspectionRun[]> {
    const data = await apiFetch<unknown>('/api/inspections');
    return z.array(InspectionRunSchema).parse(data);
  },
  async start(): Promise<RunDetail> {
    const data = await apiFetch<unknown>('/api/inspections', { method: 'POST', json: {} });
    return RunDetailSchema.parse(data);
  },
  async get(id: string): Promise<RunDetail> {
    const data = await apiFetch<unknown>(`/api/inspections/${encodeURIComponent(id)}`);
    return RunDetailSchema.parse(data);
  },
  async answer(
    runID: string,
    itemID: string,
    input: { answer: Answer; note?: string; evidence_document_id?: string },
  ): Promise<{ run: InspectionRun; response: InspectionResponse }> {
    const data = await apiFetch<unknown>(
      `/api/inspections/${encodeURIComponent(runID)}/items/${encodeURIComponent(itemID)}`,
      { method: 'PATCH', json: input },
    );
    return z
      .object({ run: InspectionRunSchema, response: ResponseSchema })
      .parse(data);
  },
  async finalize(id: string): Promise<RunDetail> {
    const data = await apiFetch<unknown>(
      `/api/inspections/${encodeURIComponent(id)}/finalize`,
      { method: 'POST', json: {} },
    );
    return RunDetailSchema.parse(data);
  },
  reportUrl(id: string): string {
    // Direct URL — the browser downloads through it with credentials: include
    // via a standard <a href download>. Building the full URL here avoids
    // importing the base constant at every call site.
    const base = API_BASE_URL || '';
    return `${base}/api/inspections/${encodeURIComponent(id)}/report.pdf`;
  },
};
