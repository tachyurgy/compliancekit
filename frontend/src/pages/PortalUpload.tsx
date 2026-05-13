/**
 * /u/:token — Levelbrook unified upload portal (REQ-005 + REQ-006 + REQ-008).
 *
 * The single mobile-first portal for both staff and parent magic-link uploads.
 * Branch on the backend's `kind` field in the welcome payload — copy and
 * heading change, but the upload + confirmation flow is identical.
 *
 * Three-screen flow:
 *   1. Welcome      — name + facility + status pills + "Add a document"
 *   2. Upload       — three explicit buttons (camera / gallery / PDF)
 *   3. Confirm      — OCR confirmation card (REQ-006)
 *
 * Design follows brand-voice.md: warm yellow, 7th-grade copy, no jargon.
 * No login required. The token IS the auth.
 */
import { useEffect, useMemo, useRef, useState } from 'react';
import { useParams } from 'react-router-dom';
import { Camera, ImageIcon, FileText, CheckCircle2, AlertCircle, Sparkles, ArrowLeft } from 'lucide-react';

import { isLikelyBase62Token } from '@/lib/base62';

const API_BASE: string = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

// ---- Types ---------------------------------------------------------------

type Kind = 'parent_upload' | 'staff_upload';

interface PortalRequiredDoc {
  id: string;
  doc_kind: string;
  label: string;
  required: boolean;
  status: 'on_file' | 'expiring_soon' | 'expired' | 'missing' | 'awaiting_upload';
  document_id?: string;
  expires_at?: string | null;
}

interface PortalSession {
  kind: Kind;
  child?: { id: string; first_name: string; last_name: string; classroom: string };
  staff?: { id: string; first_name: string; last_name: string; role: string };
  parent_name?: string;
  provider: { id: string; name: string; state: string };
  required_documents: PortalRequiredDoc[];
  status_counts: Record<string, number>;
  token_expires_at: string;
}

interface UploadResponse {
  document_id: string;
  upload_url?: string;
  storage_key?: string;
  parts?: Array<{ upload_url: string; storage_key: string }>;
  multi_page?: number;
  final_key?: string;
}

interface FinalizeResponse {
  document_id: string;
  expires_at?: string | null;
  extraction_confidence?: number;
  ocr_confidence?: number;
  document_type_extracted_value?: string;
  expiration_extracted_value?: string;
}

// ---- Top-level page ------------------------------------------------------

type Screen = 'welcome' | 'upload' | 'reading' | 'confirm' | 'done';

export default function PortalUpload(): JSX.Element {
  const { token } = useParams<{ token: string }>();
  const [screen, setScreen] = useState<Screen>('welcome');
  const [session, setSession] = useState<PortalSession | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [docKind, setDocKind] = useState('');
  const [pendingDoc, setPendingDoc] = useState<{ id: string; extracted: FinalizeResponse } | null>(null);

  const validToken = useMemo(() => !!token && isLikelyBase62Token(token), [token]);

  useEffect(() => {
    if (!validToken) return;
    let cancelled = false;
    void (async () => {
      try {
        const path = '/portal/parent';
        // We try parent first; if 403 we fall back to /portal/staff.
        let r = await fetch(`${API_BASE}${path}?t=${encodeURIComponent(token!)}`, {
          credentials: 'omit',
        });
        if (r.status === 403) {
          r = await fetch(`${API_BASE}/portal/staff?t=${encodeURIComponent(token!)}`, {
            credentials: 'omit',
          });
        }
        if (!r.ok) {
          if (cancelled) return;
          setError(
            r.status === 401 || r.status === 410
              ? "This upload link doesn't work — ask your director for a new one."
              : 'Something went wrong on our end.',
          );
          return;
        }
        const j = (await r.json()) as PortalSession;
        if (!cancelled) setSession(j);
      } catch {
        if (!cancelled) setError("We couldn't reach Levelbrook just now. Check your internet and try again.");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [validToken, token]);

  if (!validToken || error) {
    return (
      <PortalShell>
        <div className="rounded-2xl bg-white p-6 text-center shadow-md ring-1 ring-amber-100">
          <div className="mx-auto mb-3 grid h-10 w-10 place-items-center rounded-full bg-amber-50">
            <AlertCircle className="h-5 w-5 text-amber-600" />
          </div>
          <h1 className="text-lg font-semibold text-ink-900">This upload link doesn&apos;t work</h1>
          <p className="mt-1 text-sm text-ink-700">
            {error ?? 'Ask your director for a new one — they can send it from your page.'}
          </p>
        </div>
      </PortalShell>
    );
  }

  if (!session) {
    return (
      <PortalShell>
        <div className="rounded-2xl bg-white p-6 text-center shadow-md">
          <div className="mx-auto h-6 w-24 animate-pulse rounded-full bg-amber-100" />
          <p className="mt-3 text-sm text-ink-500">Loading your page…</p>
        </div>
      </PortalShell>
    );
  }

  const firstName = session.kind === 'parent_upload' ? session.child?.first_name : session.staff?.first_name;

  return (
    <PortalShell>
      {screen === 'welcome' && (
        <Welcome
          session={session}
          onAddDoc={() => setScreen('upload')}
        />
      )}
      {screen === 'upload' && (
        <UploadScreen
          token={token!}
          session={session}
          docKind={docKind}
          onChangeDocKind={setDocKind}
          onBack={() => setScreen('welcome')}
          onUploaded={async (resp) => {
            setScreen('reading');
            // call finalize
            try {
              const r = await fetch(
                `${API_BASE}/portal/upload/${encodeURIComponent(resp.document_id)}/finalize?t=${encodeURIComponent(
                  token!,
                )}`,
                {
                  method: 'POST',
                  credentials: 'omit',
                  headers: { 'Content-Type': 'application/json' },
                  body: JSON.stringify(resp.parts ? { parts: resp.parts.map((p) => p.storage_key) } : {}),
                },
              );
              if (!r.ok) throw new Error('finalize failed');
              const j = (await r.json()) as FinalizeResponse;
              setPendingDoc({ id: resp.document_id, extracted: j });
              setScreen('confirm');
            } catch {
              setError('Something went wrong on our end. Please try again.');
              setScreen('welcome');
            }
          }}
        />
      )}
      {screen === 'reading' && <ReadingScreen />}
      {screen === 'confirm' && pendingDoc && (
        <ConfirmScreen
          token={token!}
          doc={pendingDoc}
          onDone={() => {
            setPendingDoc(null);
            setScreen('done');
          }}
        />
      )}
      {screen === 'done' && (
        <DoneScreen
          firstName={firstName ?? ''}
          onAnother={() => {
            setDocKind('');
            setScreen('upload');
          }}
        />
      )}
    </PortalShell>
  );
}

// ---- Shell + headers ------------------------------------------------------

function PortalShell({ children }: { children: React.ReactNode }): JSX.Element {
  return (
    <div
      className="min-h-screen"
      style={{ background: 'linear-gradient(180deg, #fff8e1 0%, #fef9ec 100%)' }}
    >
      <header className="bg-white/80 backdrop-blur border-b border-amber-100">
        <div className="mx-auto max-w-md px-4 h-14 flex items-center gap-2">
          <div className="grid h-7 w-7 place-items-center rounded-full bg-amber-300 text-amber-900 font-black">
            L
          </div>
          <span className="font-semibold text-ink-900">Levelbrook</span>
        </div>
      </header>
      <main className="mx-auto max-w-md px-4 py-6">{children}</main>
      <footer className="mx-auto max-w-md px-4 py-6 text-center text-xs text-ink-500">
        Need help? Email{' '}
        <a className="text-amber-700 underline" href="mailto:levelbrookteam@gmail.com">
          levelbrookteam@gmail.com
        </a>
      </footer>
    </div>
  );
}

// ---- Welcome -------------------------------------------------------------

function Welcome({
  session,
  onAddDoc,
}: {
  session: PortalSession;
  onAddDoc: () => void;
}): JSX.Element {
  const isParent = session.kind === 'parent_upload';
  const childFirst = session.child?.first_name ?? '';
  const staffFirst = session.staff?.first_name ?? '';

  const heading = isParent
    ? "Hi! 👋"
    : `Hi ${staffFirst}! 👋`;
  const sub = isParent
    ? `You're ${childFirst}'s parent at ${session.provider.name}. We need a few forms to keep their file complete. This takes about 2 minutes.`
    : `${session.provider.name} asked for a few documents to keep your file complete. This takes about 2 minutes.`;

  const counts = session.status_counts || {};
  const onFile = counts.on_file ?? 0;
  const expiringSoon = counts.expiring_soon ?? 0;
  const missing = (counts.missing ?? 0) + (counts.expired ?? 0);

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-2xl font-bold text-ink-900">{heading}</h1>
        <p className="mt-2 text-base text-ink-700 leading-relaxed">{sub}</p>
      </div>

      <div className="rounded-2xl bg-white p-4 shadow-md ring-1 ring-amber-100">
        <div className="text-xs font-medium uppercase tracking-wider text-amber-700">
          {isParent ? 'For ' + childFirst : 'Your file'}
        </div>
        <div className="mt-1 text-base font-semibold text-ink-900">{session.provider.name}</div>

        <div className="mt-4 grid grid-cols-3 gap-2">
          <Pill tone="green" label="On file" value={onFile} />
          <Pill tone="amber" label="Expiring soon" value={expiringSoon} />
          <Pill tone="coral" label="Missing" value={missing} />
        </div>

        {session.required_documents.length > 0 && (
          <ul className="mt-4 divide-y divide-amber-50">
            {session.required_documents.map((d) => (
              <li key={d.id} className="flex items-center justify-between py-2.5">
                <div className="pr-3 text-sm text-ink-800">
                  {d.label}
                  {d.required && <span className="ml-1 text-coral-600" aria-label="required">*</span>}
                </div>
                <StatusPill status={d.status} />
              </li>
            ))}
          </ul>
        )}
      </div>

      <button
        type="button"
        onClick={onAddDoc}
        className="w-full rounded-2xl bg-amber-300 hover:bg-amber-400 active:bg-amber-500 px-4 py-4 text-base font-semibold text-amber-950 shadow-md transition focus-visible:ring-2 focus-visible:ring-amber-600 focus-visible:ring-offset-2"
      >
        Add a document
      </button>
    </div>
  );
}

function Pill({ tone, label, value }: { tone: 'green' | 'amber' | 'coral'; label: string; value: number }): JSX.Element {
  const map = {
    green: 'bg-brand-50 text-brand-800 ring-brand-100',
    amber: 'bg-amber-50 text-amber-800 ring-amber-200',
    coral: 'bg-critical-50 text-critical-700 ring-critical-100',
  } as const;
  return (
    <div className={`rounded-xl px-2 py-2 text-center ring-1 ${map[tone]}`}>
      <div className="text-lg font-bold">{value}</div>
      <div className="text-[10px] uppercase tracking-wider font-medium">{label}</div>
    </div>
  );
}

function StatusPill({ status }: { status: PortalRequiredDoc['status'] }): JSX.Element {
  const map: Record<PortalRequiredDoc['status'], { label: string; className: string }> = {
    on_file: { label: 'On file', className: 'bg-brand-50 text-brand-700 ring-brand-100' },
    expiring_soon: { label: 'Expiring soon', className: 'bg-amber-50 text-amber-700 ring-amber-200' },
    expired: { label: 'Expired', className: 'bg-critical-50 text-critical-700 ring-critical-100' },
    missing: { label: 'Missing', className: 'bg-surface-sunken text-ink-700 ring-line' },
    awaiting_upload: { label: 'Awaiting upload', className: 'bg-sky-50 text-sky-700 ring-sky-100' },
  };
  const { label, className } = map[status];
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-medium ring-1 ${className}`}>
      {label}
    </span>
  );
}

// ---- Upload screen --------------------------------------------------------

const COMMON_PARENT_DOC_KINDS: Array<{ value: string; label: string }> = [
  { value: '', label: "I'm not sure — Levelbrook will figure it out" },
  { value: 'immunization_record', label: 'Immunization (DH 680)' },
  { value: 'physical_exam', label: 'Health exam (DH 3040)' },
  { value: 'enrollment_form', label: 'Enrollment form' },
  { value: 'emergency_card', label: 'Emergency contacts' },
  { value: 'allergy_medical', label: 'Allergy / medical' },
  { value: 'authorized_pickup', label: 'Authorized pickup' },
];
const COMMON_STAFF_DOC_KINDS: Array<{ value: string; label: string }> = [
  { value: '', label: "I'm not sure — Levelbrook will figure it out" },
  { value: 'cpr_cert', label: 'CPR / First Aid card' },
  { value: 'background_check', label: 'Background screening' },
  { value: 'tb_test', label: 'TB test' },
  { value: 'food_handler_cert', label: 'Food handler certificate' },
];

function UploadScreen({
  token,
  session,
  docKind,
  onChangeDocKind,
  onBack,
  onUploaded,
}: {
  token: string;
  session: PortalSession;
  docKind: string;
  onChangeDocKind: (k: string) => void;
  onBack: () => void;
  onUploaded: (r: UploadResponse) => void;
}): JSX.Element {
  const [busy, setBusy] = useState(false);
  const cameraRef = useRef<HTMLInputElement>(null);
  const galleryRef = useRef<HTMLInputElement>(null);
  const pdfRef = useRef<HTMLInputElement>(null);

  const docKinds = session.kind === 'parent_upload' ? COMMON_PARENT_DOC_KINDS : COMMON_STAFF_DOC_KINDS;

  const handleFiles = async (files: FileList | null, multi: boolean) => {
    if (!files || files.length === 0) return;
    setBusy(true);
    try {
      const arr = Array.from(files);
      const pdfMode = arr[0].type === 'application/pdf';
      const wantMulti = multi && arr.length > 1 && !pdfMode;

      const presignResp = await fetch(`${API_BASE}/portal/upload?t=${encodeURIComponent(token)}`, {
        method: 'POST',
        credentials: 'omit',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          kind: docKind,
          mime_type: pdfMode ? 'application/pdf' : 'image/jpeg',
          multi_page: wantMulti ? arr.length : 0,
        }),
      });
      if (!presignResp.ok) throw new Error('presign failed');
      const presigned = (await presignResp.json()) as UploadResponse;

      if (wantMulti && presigned.parts) {
        // PUT each photo to its presigned URL.
        await Promise.all(
          presigned.parts.map((part, i) =>
            fetch(part.upload_url, {
              method: 'PUT',
              headers: { 'Content-Type': 'image/jpeg' },
              body: arr[i],
            }),
          ),
        );
      } else if (presigned.upload_url) {
        await fetch(presigned.upload_url, {
          method: 'PUT',
          headers: { 'Content-Type': pdfMode ? 'application/pdf' : 'image/jpeg' },
          body: arr[0],
        });
      }

      onUploaded(presigned);
    } catch {
      alert("We couldn't upload that file. Please try again.");
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      <button
        type="button"
        onClick={onBack}
        className="inline-flex items-center gap-1 text-sm text-ink-700 hover:text-ink-900"
      >
        <ArrowLeft className="h-4 w-4" /> Back
      </button>
      <div>
        <h1 className="text-2xl font-bold text-ink-900">Add your document</h1>
        <p className="mt-1 text-base text-ink-700">
          Snap a photo, or grab one from your phone — whichever&apos;s easier.
        </p>
      </div>

      <div className="rounded-2xl bg-white p-4 shadow-md ring-1 ring-amber-100">
        <label className="block text-sm font-medium text-ink-900">What is this?</label>
        <select
          className="mt-2 block w-full rounded-xl border border-amber-200 bg-amber-50/40 px-3 py-3 text-base focus:border-amber-500 focus:outline-none focus:ring-2 focus:ring-amber-300"
          value={docKind}
          onChange={(e) => onChangeDocKind(e.target.value)}
        >
          {docKinds.map((k) => (
            <option key={k.value} value={k.value}>
              {k.label}
            </option>
          ))}
        </select>
      </div>

      <div className="space-y-3">
        <UploadButton
          icon={<Camera className="h-6 w-6" />}
          label="📷 Take a photo"
          subLabel="Front and back if it has them. We'll stitch them together."
          disabled={busy}
          onClick={() => cameraRef.current?.click()}
        />
        <input
          ref={cameraRef}
          type="file"
          accept="image/*"
          capture="environment"
          multiple
          className="sr-only"
          onChange={(e) => {
            void handleFiles(e.target.files, true);
            e.target.value = '';
          }}
        />

        <UploadButton
          icon={<ImageIcon className="h-6 w-6" />}
          label="🖼 Choose from your photos"
          subLabel="Pick a photo you already took."
          disabled={busy}
          onClick={() => galleryRef.current?.click()}
        />
        <input
          ref={galleryRef}
          type="file"
          accept="image/*"
          className="sr-only"
          onChange={(e) => {
            void handleFiles(e.target.files, false);
            e.target.value = '';
          }}
        />

        <UploadButton
          icon={<FileText className="h-6 w-6" />}
          label="📄 Upload a PDF"
          subLabel="If your provider sent you a PDF."
          disabled={busy}
          onClick={() => pdfRef.current?.click()}
        />
        <input
          ref={pdfRef}
          type="file"
          accept="application/pdf"
          className="sr-only"
          onChange={(e) => {
            void handleFiles(e.target.files, false);
            e.target.value = '';
          }}
        />
      </div>

      {busy && (
        <p className="text-center text-sm text-ink-500">Uploading…</p>
      )}
    </div>
  );
}

function UploadButton({
  icon,
  label,
  subLabel,
  onClick,
  disabled,
}: {
  icon: React.ReactNode;
  label: string;
  subLabel: string;
  onClick: () => void;
  disabled?: boolean;
}): JSX.Element {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className="flex w-full items-center gap-4 rounded-2xl bg-white px-4 py-4 text-left shadow-md ring-1 ring-amber-100 transition hover:bg-amber-50 active:bg-amber-100 disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-amber-500"
      style={{ minHeight: 64 }}
    >
      <span className="grid h-12 w-12 place-items-center rounded-xl bg-amber-100 text-amber-800">
        {icon}
      </span>
      <span className="flex-1">
        <span className="block text-base font-semibold text-ink-900">{label}</span>
        <span className="block text-xs text-ink-500">{subLabel}</span>
      </span>
    </button>
  );
}

// ---- Reading state --------------------------------------------------------

function ReadingScreen(): JSX.Element {
  return (
    <div className="rounded-2xl bg-white p-6 text-center shadow-md ring-1 ring-amber-100">
      <div className="mx-auto mb-3 grid h-12 w-12 place-items-center rounded-full bg-amber-100">
        <Sparkles className="h-6 w-6 text-amber-700 animate-pulse" />
      </div>
      <h2 className="text-lg font-semibold text-ink-900">Got it — give us a second to read this.</h2>
      <p className="mt-1 text-sm text-ink-700">
        Levelbrook AI is checking the document type and expiration date.
      </p>
    </div>
  );
}

// ---- Confirmation card ----------------------------------------------------

function ConfirmScreen({
  token,
  doc,
  onDone,
}: {
  token: string;
  doc: { id: string; extracted: FinalizeResponse };
  onDone: () => void;
}): JSX.Element {
  return (
    <OcrConfirmationCard
      docKindGuess={doc.extracted.document_type_extracted_value ?? ''}
      expirationGuess={doc.extracted.expiration_extracted_value ?? ''}
      confidence={doc.extracted.extraction_confidence ?? 0}
      submitLabel="Looks right"
      onConfirm={async (payload) => {
        const r = await fetch(
          `${API_BASE}/portal/document/${encodeURIComponent(doc.id)}/confirm?t=${encodeURIComponent(token)}`,
          {
            method: 'POST',
            credentials: 'omit',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
          },
        );
        if (!r.ok) throw new Error('confirm failed');
        onDone();
      }}
    />
  );
}

function DoneScreen({ firstName, onAnother }: { firstName: string; onAnother: () => void }): JSX.Element {
  return (
    <div className="rounded-2xl bg-white p-6 text-center shadow-md ring-1 ring-brand-100">
      <div className="mx-auto mb-3 grid h-12 w-12 place-items-center rounded-full bg-brand-100">
        <CheckCircle2 className="h-7 w-7 text-brand-700" />
      </div>
      <h2 className="text-xl font-bold text-ink-900">All done — thanks{firstName ? ', ' + firstName : ''}!</h2>
      <p className="mt-1 text-sm text-ink-700">We&apos;ll let your director know.</p>
      <button
        type="button"
        onClick={onAnother}
        className="mt-4 w-full rounded-2xl bg-amber-300 hover:bg-amber-400 px-4 py-3 text-sm font-semibold text-amber-950"
      >
        Add another document
      </button>
    </div>
  );
}

// ---- The OCR confirmation card (also used in manager dashboard) -----------

export interface OcrConfirmPayload {
  document_type: string;
  expiration_date?: string;
  does_not_expire?: boolean;
  accepted_extraction: boolean;
}

export function OcrConfirmationCard({
  docKindGuess,
  expirationGuess,
  confidence,
  submitLabel = 'Looks right',
  onConfirm,
  documentURL,
}: {
  docKindGuess: string;
  expirationGuess: string;
  confidence: number;
  submitLabel?: string;
  onConfirm: (p: OcrConfirmPayload) => Promise<void> | void;
  documentURL?: string;
}): JSX.Element {
  const [docKind, setDocKind] = useState(docKindGuess);
  const [expDate, setExpDate] = useState(expirationGuess);
  const [doesNotExpire, setDoesNotExpire] = useState(false);
  const [touchedKind, setTouchedKind] = useState(false);
  const [touchedDate, setTouchedDate] = useState(false);
  const [busy, setBusy] = useState(false);
  const [showDoc, setShowDoc] = useState(false);

  // Confidence buckets: high (≥0.9), medium (0.6–0.9), low (<0.6).
  const lowConfidence = confidence > 0 && confidence < 0.6;
  const mediumConfidence = confidence >= 0.6 && confidence < 0.9;

  const canConfirm = !lowConfidence || (touchedKind && touchedDate);

  const submit = async (acceptedExtraction: boolean) => {
    if (!docKind && !doesNotExpire) {
      alert('Please pick a document type.');
      return;
    }
    if (!doesNotExpire && !expDate) {
      alert('Please pick an expiration date, or check "This document doesn\'t expire."');
      return;
    }
    setBusy(true);
    try {
      await onConfirm({
        document_type: docKind,
        expiration_date: doesNotExpire ? undefined : expDate,
        does_not_expire: doesNotExpire,
        accepted_extraction: acceptedExtraction,
      });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      {/* Banner — never dismissible. Verbatim from REQ-006. */}
      <div className="rounded-2xl bg-amber-50 p-4 ring-1 ring-amber-200">
        <div className="flex items-start gap-3">
          <Sparkles className="mt-0.5 h-5 w-5 flex-none text-amber-700" />
          <div className="text-sm text-amber-900 leading-relaxed">
            Levelbrook AI read this document for you. Please check the type and expiration date below — AI can make
            mistakes, so a quick double-check keeps your file accurate.
            {lowConfidence && (
              <p className="mt-2 font-medium">
                We weren&apos;t sure about this one. Please correct anything that looks off, then tap {submitLabel}.
              </p>
            )}
          </div>
        </div>
      </div>

      {/* Field 1: Document type */}
      <div className="rounded-2xl bg-white p-4 shadow-md ring-1 ring-line-soft">
        <label className="block text-sm font-semibold text-ink-900">Document type</label>
        <div className="mt-2 flex items-center gap-2">
          <input
            type="text"
            value={docKind}
            onChange={(e) => {
              setDocKind(e.target.value);
              setTouchedKind(true);
            }}
            className="block w-full rounded-xl border border-line px-3 py-3 text-base focus:border-amber-500 focus:outline-none focus:ring-2 focus:ring-amber-300"
          />
          {mediumConfidence && (
            <span className="whitespace-nowrap rounded-full bg-amber-100 px-2 py-1 text-xs font-medium text-amber-800">
              Please review
            </span>
          )}
        </div>
      </div>

      {/* Field 2: Expiration */}
      <div className="rounded-2xl bg-white p-4 shadow-md ring-1 ring-line-soft">
        <label className="block text-sm font-semibold text-ink-900">This document expires on</label>
        {!doesNotExpire && (
          <input
            type="date"
            value={expDate}
            onChange={(e) => {
              setExpDate(e.target.value);
              setTouchedDate(true);
            }}
            className="mt-2 block w-full rounded-xl border border-line px-3 py-3 text-base focus:border-amber-500 focus:outline-none focus:ring-2 focus:ring-amber-300"
          />
        )}
        <label className="mt-3 flex items-center gap-2 text-sm text-ink-700">
          <input
            type="checkbox"
            className="h-5 w-5 rounded border-line text-amber-600 focus:ring-amber-500"
            checked={doesNotExpire}
            onChange={(e) => {
              setDoesNotExpire(e.target.checked);
              setTouchedDate(true);
            }}
          />
          This document doesn&apos;t expire
        </label>
      </div>

      {/* See the document accordion */}
      {documentURL && (
        <details className="rounded-2xl bg-white p-4 shadow-md ring-1 ring-line-soft">
          <summary
            className="cursor-pointer text-sm font-medium text-ink-700"
            onClick={() => setShowDoc((s) => !s)}
          >
            {showDoc ? 'Hide the document' : 'See the document'}
          </summary>
          <div className="mt-3">
            <iframe
              title="document preview"
              src={documentURL}
              className="h-96 w-full rounded-xl border border-line"
            />
          </div>
        </details>
      )}

      {/* Buttons — stacked on mobile (Looks right on top), side-by-side on >= sm. */}
      <div className="flex flex-col gap-3 sm:flex-row-reverse">
        <button
          type="button"
          disabled={busy || !canConfirm}
          onClick={() => void submit(true)}
          className="flex-1 rounded-2xl bg-amber-300 hover:bg-amber-400 active:bg-amber-500 px-4 py-4 text-base font-semibold text-amber-950 shadow-md disabled:cursor-not-allowed disabled:opacity-50"
        >
          {submitLabel}
        </button>
        <button
          type="button"
          disabled={busy}
          onClick={() => void submit(false)}
          className="flex-1 rounded-2xl bg-white px-4 py-4 text-base font-semibold text-ink-700 ring-1 ring-line hover:bg-surface-sunken"
        >
          Fix this
        </button>
      </div>
    </div>
  );
}
