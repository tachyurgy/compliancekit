import { z } from 'zod';
import { apiFetch } from './client';

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
  async upload(
    file: File,
    meta: { documentType: string; subjectType: 'provider' | 'child' | 'staff'; subjectId?: string },
  ): Promise<DaycareDocument> {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('documentType', meta.documentType);
    formData.append('subjectType', meta.subjectType);
    if (meta.subjectId) formData.append('subjectId', meta.subjectId);
    const data = await apiFetch<unknown>('/api/documents', {
      method: 'POST',
      body: formData,
    });
    return DocumentSchema.parse(data);
  },
  async remove(id: string): Promise<void> {
    await apiFetch(`/api/documents/${encodeURIComponent(id)}`, { method: 'DELETE' });
  },
};
