import { z } from 'zod';
import { apiFetch } from './client';

/**
 * Data export + retention API bindings.
 *
 * Two concerns live here:
 *  - `exports`: kick off a full provider data export, list past exports, and
 *    mint fresh presigned download URLs.
 *  - `deleteProvider`: request account deletion, which sets providers.deleted_at
 *    on the server and kicks off the 90-day retention clock.
 *
 * Every schema mirrors the Go handler's JSON shape 1:1 so Zod parse failures
 * tell us the backend changed without a matching frontend update.
 */

export const DataExportSchema = z.object({
  id: z.string(),
  provider_id: z.string(),
  requested_by_user_id: z.string().optional().default(''),
  status: z.enum(['requested', 'running', 'completed', 'failed']),
  s3_key: z.string().optional().default(''),
  error_text: z.string().optional().default(''),
  started_at: z.string(),
  finished_at: z.string().optional().default(''),
});
export type DataExport = z.infer<typeof DataExportSchema>;

export const DataExportListSchema = z.array(DataExportSchema);

export const CreateExportResponseSchema = z.object({
  export_id: z.string(),
  status: z.string(),
  message: z.string().optional().default(''),
});
export type CreateExportResponse = z.infer<typeof CreateExportResponseSchema>;

export const DownloadUrlSchema = z.object({ url: z.string().url() });

export const DeletionResponseSchema = z.object({
  status: z.string(),
  grace_period_days: z.number().int(),
  message: z.string(),
});
export type DeletionResponse = z.infer<typeof DeletionResponseSchema>;

export const dataExportApi = {
  /** POST /api/exports — enqueue a full export. */
  async create(): Promise<CreateExportResponse> {
    const data = await apiFetch<unknown>('/api/exports', { method: 'POST' });
    return CreateExportResponseSchema.parse(data);
  },

  /** GET /api/exports — historical list. */
  async list(): Promise<DataExport[]> {
    const data = await apiFetch<unknown>('/api/exports');
    return DataExportListSchema.parse(data);
  },

  /** GET /api/exports/{id}/download — mint a fresh presigned URL. */
  async getDownloadUrl(id: string): Promise<string> {
    const data = await apiFetch<unknown>(
      `/api/exports/${encodeURIComponent(id)}/download`,
    );
    return DownloadUrlSchema.parse(data).url;
  },

  /** DELETE /api/providers/me — request deletion (starts 90-day clock). */
  async deleteProvider(confirm: string): Promise<DeletionResponse> {
    const data = await apiFetch<unknown>('/api/providers/me', {
      method: 'DELETE',
      json: { confirm },
    });
    return DeletionResponseSchema.parse(data);
  },
};
