import type { AuditRecord, PreparedSession, SignatureRecord } from "./types";

const API_BASE = (import.meta as unknown as { env?: { VITE_API_BASE?: string } })
  .env?.VITE_API_BASE ?? "";

async function handle<T>(resp: Response): Promise<T> {
  if (!resp.ok) {
    let body: string;
    try {
      body = await resp.text();
    } catch {
      body = resp.statusText;
    }
    throw new Error(`${resp.status} ${resp.statusText}: ${body}`);
  }
  return (await resp.json()) as T;
}

export async function prepareSignSession(token: string): Promise<PreparedSession> {
  const resp = await fetch(`${API_BASE}/api/pdfsign/sessions/${encodeURIComponent(token)}`, {
    method: "GET",
    credentials: "include",
  });
  return handle<PreparedSession>(resp);
}

export async function finalizeSignature(
  token: string,
  signedBlob: Blob,
  audit: AuditRecord,
): Promise<SignatureRecord> {
  const form = new FormData();
  form.append("audit", new Blob([JSON.stringify(audit)], { type: "application/json" }));
  form.append("pdf", signedBlob, `${audit.signatureId}.pdf`);
  const resp = await fetch(
    `${API_BASE}/api/pdfsign/sessions/${encodeURIComponent(token)}/finalize`,
    {
      method: "POST",
      body: form,
      credentials: "include",
    },
  );
  return handle<SignatureRecord>(resp);
}

export interface CreateSessionInput {
  documentTemplateId: string;
  signerName: string;
  signerEmail: string;
  fields: Array<{
    type: string;
    pageIndex: number;
    x: number;
    y: number;
    width: number;
    height: number;
    required: boolean;
    label?: string;
  }>;
  expiresInHours?: number;
}

export async function createSignSession(
  input: CreateSessionInput,
): Promise<{ token: string; signingUrl: string }> {
  const resp = await fetch(`${API_BASE}/api/pdfsign/sessions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    body: JSON.stringify(input),
  });
  return handle<{ token: string; signingUrl: string }>(resp);
}

export interface TemplateSummary {
  id: string;
  name: string;
  pageCount: number;
  fieldCount: number;
  updatedAt: string;
}

export async function listTemplates(): Promise<TemplateSummary[]> {
  const resp = await fetch(`${API_BASE}/api/pdfsign/templates`, {
    credentials: "include",
  });
  return handle<TemplateSummary[]>(resp);
}

export async function saveTemplateFields(
  templateId: string,
  fields: CreateSessionInput["fields"],
): Promise<void> {
  const resp = await fetch(
    `${API_BASE}/api/pdfsign/templates/${encodeURIComponent(templateId)}/fields`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ fields }),
    },
  );
  if (!resp.ok) {
    throw new Error(`saveTemplateFields: ${resp.status}`);
  }
}
