/**
 * REQ-002 #6 + #7: ToS + DPA e-signature capture.
 *
 * The /onboarding/legal page renders the Markdown the frontend bundles, but
 * the audit trail records what the BACKEND served. This module talks to the
 * backend's /api/legal/* surface; the frontend Markdown is the bundled copy
 * for fast first-paint, while the canonical hash is what the backend hashes
 * at sign time.
 */
import { apiFetch } from './client';

export interface LegalDoc {
  version: string;
  sha256: string;
  text: string;
}

export interface LegalDocs {
  tos: LegalDoc;
  dpa: LegalDoc;
}

export const legalApi = {
  async getDocuments(): Promise<LegalDocs> {
    const data = await apiFetch<LegalDocs>('/api/legal/documents');
    return data;
  },
  async sign(input: {
    typedName: string;
    documentKinds?: Array<'tos' | 'dpa'>;
  }): Promise<{
    status: string;
    signed: Array<{ kind: string; version: string; sha256: string }>;
    typed_name: string;
  }> {
    return apiFetch('/api/legal/sign', {
      method: 'POST',
      json: {
        typed_name: input.typedName,
        document_kinds: input.documentKinds ?? ['tos', 'dpa'],
      },
    });
  },
};
