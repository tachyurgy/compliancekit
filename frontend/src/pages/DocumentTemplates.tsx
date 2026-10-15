import { useCallback, useEffect, useRef, useState } from 'react';

import Button from '@/components/common/Button';
import Card from '@/components/common/Card';
import Input from '@/components/common/Input';
import PageHeader from '@/components/common/PageHeader';
import { FieldDesigner } from '../components/PdfSigner/FieldDesigner';
import type { Field } from '../components/PdfSigner/types';
import {
  createSignSession,
  getTemplate,
  listSignatures,
  listTemplates,
  uploadTemplate,
  type SignatureSummary,
  type TemplateDetail,
  type TemplateSummary,
} from '../components/PdfSigner/api';

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '';

/**
 * E-signature templates: upload a blank PDF, place fields on it, send it to a
 * signer, and see completed signatures (signed PDF + audit JSON).
 *
 * The signing link is shown on screen after it is created. In production it
 * is also emailed; in the public demo outbound email is switched off, so the
 * on-screen link is how you play the signer.
 */
export default function DocumentTemplates(): JSX.Element {
  const [templates, setTemplates] = useState<TemplateSummary[] | null>(null);
  const [signatures, setSignatures] = useState<SignatureSummary[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<TemplateDetail | null>(null);
  const [sending, setSending] = useState<TemplateSummary | null>(null);
  const [uploading, setUploading] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const refresh = useCallback(async () => {
    try {
      const [t, s] = await Promise.all([listTemplates(), listSignatures()]);
      setTemplates(t);
      setSignatures(s);
    } catch (err) {
      setError((err as Error).message);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const onUpload = async (file: File | undefined): Promise<void> => {
    if (!file) return;
    setUploading(true);
    setError(null);
    try {
      await uploadTemplate(file.name.replace(/\.pdf$/i, ''), file);
      await refresh();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = '';
    }
  };

  const openEditor = async (t: TemplateSummary): Promise<void> => {
    try {
      setEditing(await getTemplate(t.id));
    } catch (err) {
      setError((err as Error).message);
    }
  };

  if (editing) {
    return (
      <div className="space-y-4">
        <Button variant="secondary" size="sm" onClick={() => setEditing(null)}>
          ← Back to templates
        </Button>
        <FieldDesigner
          templateId={editing.id}
          templateName={editing.name}
          documentUrl={`${API_BASE}/api/pdfsign/templates/${editing.id}/pdf`}
          initialFields={editing.fields as Field[]}
          onSaved={() => {
            setEditing(null);
            void refresh();
          }}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="E-signature templates"
        description="Blank forms you send out for signature. Signing happens in the browser; the server verifies the hashes and keeps the signed PDF with its audit trail."
        actions={
          <>
            <input
              ref={fileRef}
              type="file"
              accept="application/pdf"
              className="hidden"
              onChange={(e) => void onUpload(e.target.files?.[0])}
            />
            <Button loading={uploading} onClick={() => fileRef.current?.click()}>
              Upload PDF
            </Button>
          </>
        }
      />

      {error ? (
        <p role="alert" className="rounded-lb-sm bg-critical-50 px-4 py-3 text-sm text-critical-700 ring-1 ring-critical-200">
          {error}
        </p>
      ) : null}

      <Card title="Templates" padded={false}>
        {!templates ? (
          <p className="px-6 py-5 text-sm text-ink-500">Loading…</p>
        ) : templates.length === 0 ? (
          <p className="px-6 py-5 text-sm text-ink-500">
            No templates yet. Upload a PDF, then place signature fields on it.
          </p>
        ) : (
          <ul className="divide-y divide-line-soft">
            {templates.map((t) => (
              <li key={t.id} className="px-6 py-4 flex flex-wrap items-center justify-between gap-3">
                <div>
                  <p className="font-medium text-ink-900">{t.name}</p>
                  <p className="text-sm text-ink-500">
                    {t.pageCount} page{t.pageCount === 1 ? '' : 's'} · {t.fieldCount} field
                    {t.fieldCount === 1 ? '' : 's'}
                  </p>
                </div>
                <div className="flex gap-2">
                  <Button variant="secondary" size="sm" onClick={() => void openEditor(t)}>
                    Edit fields
                  </Button>
                  <Button size="sm" disabled={t.fieldCount === 0} onClick={() => setSending(t)}>
                    Send for signature
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {sending ? (
        <SendPanel template={sending} onClose={() => setSending(null)} />
      ) : null}

      <Card
        title="Completed signatures"
        description="Each one stores the signed PDF and a JSON audit record (signer, IP, user agent, consent, SHA-256 before and after)."
        padded={false}
      >
        {signatures.length === 0 ? (
          <p className="px-6 py-5 text-sm text-ink-500">Nothing signed yet.</p>
        ) : (
          <ul className="divide-y divide-line-soft">
            {signatures.map((s) => (
              <li key={s.signatureId} className="px-6 py-4 flex flex-wrap items-center justify-between gap-3">
                <div className="min-w-0">
                  <p className="font-medium text-ink-900">
                    {s.templateName || 'Document'} · {s.signerName}
                  </p>
                  <p className="text-sm text-ink-500 break-all">
                    {new Date(s.signedAt).toLocaleString()} · sha256 {s.sha256After.slice(0, 16)}…
                  </p>
                </div>
                <div className="flex gap-3 text-sm font-medium">
                  {s.signedPdfUrl ? (
                    <a className="text-brand-700 hover:underline" href={s.signedPdfUrl} target="_blank" rel="noreferrer">
                      Signed PDF
                    </a>
                  ) : null}
                  {s.auditUrl ? (
                    <a className="text-brand-700 hover:underline" href={s.auditUrl} target="_blank" rel="noreferrer">
                      Audit JSON
                    </a>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}

function SendPanel({
  template,
  onClose,
}: {
  template: TemplateSummary;
  onClose: () => void;
}): JSX.Element {
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [busy, setBusy] = useState(false);
  const [link, setLink] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const submit = async (e: React.FormEvent): Promise<void> => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const detail = await getTemplate(template.id);
      const res = await createSignSession({
        documentTemplateId: template.id,
        signerName: name,
        signerEmail: email,
        fields: detail.fields,
        expiresInHours: 72,
      });
      setLink(res.signingUrl);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card
      title={`Send “${template.name}” for signature`}
      action={
        <Button variant="ghost" size="sm" onClick={onClose}>
          Close
        </Button>
      }
    >
      {link ? (
        <div className="space-y-3">
          <p className="text-sm text-ink-700">
            Signing link created (valid 72 hours). In production this goes to the signer by email; here, open it yourself to play the signer.
          </p>
          <p className="break-all rounded-lb-sm bg-surface-muted px-3 py-2 text-sm font-mono">{link}</p>
          <a href={link} target="_blank" rel="noreferrer">
            <Button>Open the signing page</Button>
          </a>
        </div>
      ) : (
        <form onSubmit={(e) => void submit(e)} className="grid gap-4 sm:grid-cols-2">
          <Input label="Signer name" value={name} onChange={(e) => setName(e.target.value)} required />
          <Input
            label="Signer email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            placeholder="parent@example.com"
          />
          {error ? (
            <p role="alert" className="sm:col-span-2 text-sm text-critical-700">
              {error}
            </p>
          ) : null}
          <div className="sm:col-span-2">
            <Button type="submit" loading={busy}>
              Create signing link
            </Button>
          </div>
        </form>
      )}
    </Card>
  );
}
