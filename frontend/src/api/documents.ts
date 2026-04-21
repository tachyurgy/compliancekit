import { z } from 'zod';
import { apiFetch, ApiError } from './client';

export const DocumentStatusSchema = z.enum([
  'uploaded',
  'pending_signature',
  'signed',
  'expired',
  'rejected',
]);

export const DocumentSchema = z.object({
  id: z.string(),
  providerId: z.string(),
  documentType: z.string(),
  label: z.string(),
  fileName: z.string().nullable(),
  mimeType: z.string().nullable(),
  sizeBytes: z.number().int().nullable(),
  status: DocumentStatusSchema,
  subjectType: z.enum(['provider', 'child', 'staff']),
  subjectId: z.string().nullable(),
  expirationDate: z.string().nullable(),
  uploadedAt: z.string(),
  updatedAt: z.string(),
});
export type DaycareDocument = z.infer<typeof DocumentSchema>;

export const DocumentDetailSchema = DocumentSchema.extend({
  downloadUrl: z.string().nullable(),
  previewUrl: z.string().nullable(),
  notes: z.string().nullable(),
  signatureRequests: z
    .array(
      z.object({
        id: z.string(),
        signerEmail: z.string().email(),
        signerName: z.string().nullable(),
        status: z.enum(['pending', 'signed', 'expired', 'cancelled']),
        sentAt: z.string(),
        signedAt: z.string().nullable(),
        signUrl: z.string().nullable(),
      }),
    )
    .default([]),
});
export type DocumentDetail = z.infer<typeof DocumentDetailSchema>;

export const documentsApi = {
  async list(params?: {
    subjectType?: 'provider' | 'child' | 'staff';
    subjectId?: string;
    status?: z.infer<typeof DocumentStatusSchema>;
  }): Promise<DaycareDocument[]> {
    const data = await apiFetch<unknown>('/api/documents', { query: params });
    return z.array(DocumentSchema).parse(data);
  },
  async get(id: string): Promise<DocumentDetail> {
    const data = await apiFetch<unknown>(`/api/documents/${encodeURIComponent(id)}`);
    return DocumentDetailSchema.parse(data);
  },
  /**
   * Upload a document via the S3 presigned-PUT flow:
   *   1. POST /api/documents/presign  → { document_id, upload_url, storage_key }
   *   2. PUT  upload_url (browser → S3 directly, no backend transit)
   *   3. POST /api/documents/{id}/finalize  → kicks off OCR + expiry extraction
   *
   * Returns a minimal DaycareDocument shape so the caller can optimistically
   * update caches; the authoritative record arrives on the next list()/get().
   * If the backend returns 503 'storage_not_configured', ApiError surfaces
   * the "document upload is not configured" message verbatim.
   */
  async upload(
    file: File,
    meta: { documentType: string; subjectType: 'provider' | 'child' | 'staff'; subjectId?: string },
  ): Promise<DaycareDocument> {
    const mimeType = file.type || 'application/octet-stream';
    const presign = await apiFetch<{
      document_id: string;
      upload_url: string;
      storage_key: string;
    }>('/api/documents/presign', {
      method: 'POST',
      json: {
        subject_kind: meta.subjectType,
        subject_id: meta.subjectId ?? '',
        kind: meta.documentType,
        mime_type: mimeType,
        title: file.name,
      },
    });

    const putResp = await fetch(presign.upload_url, {
      method: 'PUT',
      headers: { 'Content-Type': mimeType },
      body: file,
    });
    if (!putResp.ok) {
      throw new ApiError(putResp.status, {
        code: 's3_put_failed',
        message: `Upload to S3 failed (${putResp.status} ${putResp.statusText}).`,
      });
    }

    await apiFetch(`/api/documents/${encodeURIComponent(presign.document_id)}/finalize`, {
      method: 'POST',
    });

    return {
      id: presign.document_id,
      providerId: '',
      documentType: meta.documentType,
      label: file.name,
      fileName: file.name,
      mimeType,
      sizeBytes: file.size,
      status: 'uploaded',
      subjectType: meta.subjectType,
      subjectId: meta.subjectId ?? null,
      expirationDate: null,
      uploadedAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };
  },
  async remove(id: string): Promise<void> {
    await apiFetch(`/api/documents/${encodeURIComponent(id)}`, { method: 'DELETE' });
  },
};
