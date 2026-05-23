import { z } from 'zod';
import { apiFetch } from './client';

// REQ-038 evidence-photo client. Multipart uploads go through a plain
// fetch() call because apiFetch only handles JSON bodies; we keep the
// same `credentials: 'include'` posture so the session cookie travels.

export const EvidencePhotoSubjectKind = z.enum(['drill', 'posting', 'generic']);
export type EvidencePhotoSubjectKind = z.infer<typeof EvidencePhotoSubjectKind>;

export const EvidencePhotoSchema = z.object({
  id: z.string(),
  provider_id: z.string(),
  subject_kind: EvidencePhotoSubjectKind,
  subject_id: z.string(),
  storage_bucket: z.string(),
  storage_key: z.string(),
  mime_type: z.string(),
  caption: z.string().default(''),
  uploaded_by_user_id: z.string().optional().default(''),
  download_url: z.string().optional().default(''),
  created_at: z.string(),
});
export type EvidencePhoto = z.infer<typeof EvidencePhotoSchema>;

const API_BASE_URL: string =
  (import.meta.env.VITE_API_BASE_URL as string) ?? 'http://localhost:8080';

export const evidencePhotosApi = {
  async list(
    subjectKind: EvidencePhotoSubjectKind,
    subjectId: string,
  ): Promise<EvidencePhoto[]> {
    const data = await apiFetch<unknown>('/api/evidence-photos', {
      query: { subjectKind, subjectId },
    });
    return z.array(EvidencePhotoSchema).parse(data);
  },
  async upload(
    subjectKind: EvidencePhotoSubjectKind,
    subjectId: string,
    files: File[],
    caption?: string,
  ): Promise<EvidencePhoto[]> {
    const fd = new FormData();
    fd.append('subject_kind', subjectKind);
    fd.append('subject_id', subjectId);
    if (caption) fd.append('caption', caption);
    for (const f of files) fd.append('files', f, f.name);
    const res = await fetch(`${API_BASE_URL}/api/evidence-photos`, {
      method: 'POST',
      credentials: 'include',
      body: fd,
    });
    if (!res.ok) {
      const text = await res.text().catch(() => '');
      throw new Error(text || `upload failed (${res.status})`);
    }
    const data = (await res.json()) as unknown;
    return z.array(EvidencePhotoSchema).parse(data);
  },
  async update(id: string, caption: string): Promise<void> {
    await apiFetch(`/api/evidence-photos/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      json: { caption },
    });
  },
  async remove(id: string): Promise<void> {
    await apiFetch(`/api/evidence-photos/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    });
  },
};
