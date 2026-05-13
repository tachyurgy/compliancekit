/**
 * /u/:token — Levelbrook unified upload portal (REQ-005 / REQ-006 / REQ-008
 * / REQ-012e).
 *
 * The single mobile-first portal for both staff and parent magic-link uploads.
 * Branch on the backend's `kind` field in the welcome payload — copy and
 * heading change, but the upload + confirmation flow is identical.
 *
 * Three-screen flow:
 *   1. Welcome      — name + facility + status pills + "Add a document"
 *   2. Upload       — three explicit buttons (camera / gallery / PDF)
 *   3. Reading      — calm loading state while OCR runs
 *   4. Confirm      — OCR confirmation card (REQ-006)
 *   5. Done         — calm thank-you + "Add another document"
 *
 * The token IS the auth. No login required.
 */
import { useEffect, useMemo, useRef, useState } from 'react';
import { useParams } from 'react-router-dom';
import {
  Camera,
  ImageIcon,
  FileText,
  CheckCircle2,
  AlertCircle,
  Sparkles,
  ArrowLeft,
} from 'lucide-react';

import Button from '@/components/common/Button';
import Select from '@/components/common/Select';
import BrandMark from '@/components/common/BrandMark';
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
        // We try parent first; if 403 we fall back to /portal/staff.
        let r = await fetch(`${API_BASE}/portal/parent?t=${encodeURIComponent(token!)}`, {
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
              : 'Something went wrong on our end. Please try again in a minute.',
          );
          return;
        }
        const j = (await r.json()) as PortalSession;
        if (!cancelled) setSession(j);
      } catch {
        if (!cancelled) {
          setError(
            "We couldn't reach Levelbrook just now. Check your internet and try again.",
          );
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [validToken, token]);

  if (!validToken || error) {
    return (
      <PortalShell>
        <div className="rounded-lb bg-white p-7 text-center shadow-card border border-line">
          <div
            className="mx-auto mb-4 grid h-14 w-14 place-items-center rounded-full bg-critical-50 ring-1 ring-critical-100"
            aria-hidden
          >
            <AlertCircle className="h-6 w-6 text-critical-600" />
          </div>
          <h1 className="font-display text-xl text-ink-900">
            This upload link doesn&apos;t work
          </h1>
          <p className="mt-2 text-sm text-ink-700 leading-relaxed">
            {error ??
              'Ask your director for a new one — they can send it from your page.'}
          </p>
        </div>
      </PortalShell>
    );
  }

  if (!session) {
    return (
      <PortalShell>
        <div className="rounded-lb bg-white p-7 text-center shadow-card border border-line">
          <div className="mx-auto h-6 w-28 animate-pulse rounded-full bg-line-soft" />
          <p className="mt-4 text-sm text-ink-500">Loading your page…</p>
        </div>
      </PortalShell>
    );
  }

  const firstName =
    session.kind === 'parent_upload'
      ? session.child?.first_name
      : session.staff?.first_name;

  return (
    <PortalShell>
      {screen === 'welcome' && (
        <Welcome session={session} onAddDoc={() => setScreen('upload')} />
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
            try {
              const r = await fetch(
                `${API_BASE}/portal/upload/${encodeURIComponent(resp.document_id)}/finalize?t=${encodeURIComponent(
                  token!,
                )}`,
                {
                  method: 'POST',
                  credentials: 'omit',
                  headers: { 'Content-Type': 'application/json' },
                  body: JSON.stringify(
                    resp.parts ? { parts: resp.parts.map((p) => p.storage_key) } : {},
                  ),
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

// ---- Shell ----------------------------------------------------------------

function PortalShell({ children }: { children: React.ReactNode }): JSX.Element {
  return (
    <div
      className="min-h-screen bg-surface-muted"
      style={{
        backgroundImage:
          'radial-gradient(ellipse 90% 50% at 50% 0%, rgba(13,111,94,0.06), transparent 60%),' +
          'radial-gradient(ellipse 60% 40% at 100% 100%, rgba(197,119,74,0.06), transparent 60%)',
      }}
    >
      <header className="bg-white/80 backdrop-blur border-b border-line-soft sticky top-0 z-10">
        <div className="mx-auto max-w-md px-4 h-14 flex items-center">
          <BrandMark size="sm" />
        </div>
      </header>
      <main className="mx-auto max-w-md px-4 py-7">{children}</main>
      <footer className="mx-auto max-w-md px-4 py-8 text-center text-xs text-ink-500 leading-relaxed">
        Need help? Email{' '}
        <a
          className="font-medium text-brand-700 hover:underline"
          href="mailto:levelbrookteam@gmail.com"
        >
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

  const heading = isParent ? 'Welcome' : `Hi ${staffFirst}`;
  const sub = isParent
    ? `You're ${childFirst}'s parent at ${session.provider.name}. We need a few forms to keep their file complete. This takes about 2 minutes.`
    : `${session.provider.name} asked for a few documents to keep your file complete. This takes about 2 minutes.`;

  const counts = session.status_counts || {};
  const onFile = counts.on_file ?? 0;
  const expiringSoon = counts.expiring_soon ?? 0;
  const missing = (counts.missing ?? 0) + (counts.expired ?? 0);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-display text-3xl text-ink-900 leading-tight tracking-tight">
          {heading}
        </h1>
        <p className="mt-3 text-base text-ink-700 leading-relaxed">{sub}</p>
      </div>

      <div className="rounded-lb bg-white p-5 shadow-card border border-line">
        <div className="eyebrow">
          {isParent ? `For ${childFirst}` : 'Your file'}
        </div>
        <div className="mt-3 text-base font-semibold text-ink-900">
          {session.provider.name}
        </div>

        <div className="mt-5 grid grid-cols-3 gap-2.5">
          <Pill tone="ok" label="On file" value={onFile} />
          <Pill tone="caution" label="Expiring" value={expiringSoon} />
          <Pill tone="missing" label="Missing" value={missing} />
        </div>

        {session.required_documents.length > 0 && (
          <ul className="mt-5 divide-y divide-line-soft">
            {session.required_documents.map((d) => (
              <li
                key={d.id}
                className="flex items-center justify-between py-3 gap-3"
              >
                <div className="text-sm text-ink-800 min-w-0">
                  {d.label}
                  {d.required && (
                    <span
                      className="ml-1 text-critical-600"
                      aria-label="required"
                    >
                      *
                    </span>
                  )}
                </div>
                <StatusPill status={d.status} />
              </li>
            ))}
          </ul>
        )}
      </div>

      <Button size="lg" fullWidth onClick={onAddDoc}>
        Add a document
      </Button>
    </div>
  );
}

function Pill({
  tone,
  label,
  value,
}: {
  tone: 'ok' | 'caution' | 'missing';
  label: string;
  value: number;
}): JSX.Element {
  const map = {
    ok: 'bg-brand-50 text-brand-800 ring-brand-100',
    caution: 'bg-caution-50 text-caution-700 ring-caution-100',
    missing: 'bg-critical-50 text-critical-700 ring-critical-100',
  } as const;
  return (
    <div className={`rounded-lb-sm px-2 py-2.5 text-center ring-1 ${map[tone]}`}>
      <div className="text-xl font-bold leading-none">{value}</div>
      <div className="mt-1 text-[10px] uppercase tracking-[0.1em] font-semibold">
        {label}
      </div>
    </div>
  );
}

function StatusPill({
  status,
}: {
  status: PortalRequiredDoc['status'];
}): JSX.Element {
  const map: Record<
    PortalRequiredDoc['status'],
    { label: string; className: string }
  > = {
    on_file: {
      label: 'On file',
      className: 'bg-brand-50 text-brand-700 ring-brand-100',
    },
    expiring_soon: {
      label: 'Expiring soon',
      className: 'bg-caution-50 text-caution-700 ring-caution-100',
    },
    expired: {
      label: 'Expired',
      className: 'bg-critical-50 text-critical-700 ring-critical-100',
    },
    missing: {
      label: 'Missing',
      className: 'bg-surface-sunken text-ink-700 ring-line',
    },
    awaiting_upload: {
      label: 'Awaiting upload',
      className: 'bg-[#eaf2f7] text-[#3a6b8a] ring-[#cdd9e2]',
    },
  };
  const { label, className } = map[status];
  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-semibold ring-1 ring-inset ${className} whitespace-nowrap`}
    >
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
  const [uploadError, setUploadError] = useState<string | null>(null);
  const cameraRef = useRef<HTMLInputElement>(null);
  const galleryRef = useRef<HTMLInputElement>(null);
  const pdfRef = useRef<HTMLInputElement>(null);

  const docKinds =
    session.kind === 'parent_upload'
      ? COMMON_PARENT_DOC_KINDS
      : COMMON_STAFF_DOC_KINDS;

  const handleFiles = async (files: FileList | null, multi: boolean) => {
    if (!files || files.length === 0) return;
    setBusy(true);
    setUploadError(null);
    try {
      const arr = Array.from(files);
      const pdfMode = arr[0].type === 'application/pdf';
      const wantMulti = multi && arr.length > 1 && !pdfMode;

      const presignResp = await fetch(
        `${API_BASE}/portal/upload?t=${encodeURIComponent(token)}`,
        {
          method: 'POST',
          credentials: 'omit',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            kind: docKind,
            mime_type: pdfMode ? 'application/pdf' : 'image/jpeg',
            multi_page: wantMulti ? arr.length : 0,
          }),
        },
      );
      if (!presignResp.ok) throw new Error('presign failed');
      const presigned = (await presignResp.json()) as UploadResponse;

      if (wantMulti && presigned.parts) {
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
          headers: {
            'Content-Type': pdfMode ? 'application/pdf' : 'image/jpeg',
          },
          body: arr[0],
        });
      }

      onUploaded(presigned);
    } catch {
      setUploadError(
        "We couldn't upload that file. Please check your internet and try again.",
      );
      setBusy(false);
    }
  };

  return (
    <div className="space-y-5">
      <button
        type="button"
        onClick={onBack}
        className="inline-flex items-center gap-1 text-sm font-medium text-ink-700 hover:text-ink-900"
      >
        <ArrowLeft className="h-4 w-4" /> Back
      </button>
      <div>
        <h1 className="font-display text-3xl text-ink-900 leading-tight tracking-tight">
          Add your document
        </h1>
        <p className="mt-2 text-base text-ink-700 leading-relaxed">
          Snap a photo, or grab one from your phone — whichever&apos;s easier.
        </p>
      </div>

      <div className="rounded-lb bg-white p-5 shadow-card border border-line">
        <Select
          label="What is this?"
          value={docKind}
          onChange={(e) => onChangeDocKind(e.target.value)}
          options={docKinds}
        />
      </div>

      <div className="space-y-3">
        <UploadButton
          icon={<Camera className="h-6 w-6" />}
          label="Take a photo"
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
          label="Choose from your photos"
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
          label="Upload a PDF"
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
        <p
          className="text-center text-sm text-ink-700 inline-flex items-center justify-center gap-2 w-full"
          role="status"
        >
          <span className="h-2 w-2 rounded-full bg-brand-500 animate-pulse" />
          Uploading…
        </p>
      )}

      {uploadError && (
        <div
          className="rounded-lb-sm bg-critical-50 px-4 py-3 text-sm text-critical-700 ring-1 ring-critical-200"
          role="alert"
        >
          {uploadError}
        </div>
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
      className="flex w-full items-center gap-4 rounded-lb bg-white px-4 py-4 text-left shadow-card border border-line transition-all hover:border-brand-200 hover:shadow-elevated active:scale-[0.99] disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-600"
      style={{ minHeight: 72 }}
    >
      <span
        className="grid h-12 w-12 place-items-center rounded-lb-sm bg-brand-50 text-brand-700 ring-1 ring-brand-100 shrink-0"
        aria-hidden
      >
        {icon}
      </span>
      <span className="flex-1 min-w-0">
        <span className="block text-base font-semibold text-ink-900">
          {label}
        </span>
        <span className="block text-xs text-ink-500 mt-0.5 leading-snug">
          {subLabel}
        </span>
      </span>
    </button>
  );
}

// ---- Reading state --------------------------------------------------------

function ReadingScreen(): JSX.Element {
  return (
    <div className="rounded-lb bg-white p-7 text-center shadow-card border border-line">
      <div
        className="mx-auto mb-4 grid h-14 w-14 place-items-center rounded-full bg-warm-50 ring-1 ring-warm-100"
        aria-hidden
      >
        <Sparkles className="h-6 w-6 text-warm-600 animate-pulse" />
      </div>
      <h2 className="font-display text-xl text-ink-900">
        Got it — give us a second to read this.
      </h2>
      <p className="mt-2 text-sm text-ink-700 leading-relaxed">
        Levelbrook AI is checking the document type and expiration date.
      </p>
    </div>
  );
}

// ---- Confirmation screen --------------------------------------------------

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

function DoneScreen({
  firstName,
  onAnother,
}: {
  firstName: string;
  onAnother: () => void;
}): JSX.Element {
  return (
    <div className="rounded-lb bg-white p-7 text-center shadow-card border border-line">
      <div
        className="mx-auto mb-4 grid h-14 w-14 place-items-center rounded-full bg-brand-50 ring-1 ring-brand-100"
        aria-hidden
      >
        <CheckCircle2 className="h-7 w-7 text-brand-700" />
      </div>
      <h2 className="font-display text-2xl text-ink-900 tracking-tight">
        All done{firstName ? `, ${firstName}` : ''}.
      </h2>
      <p className="mt-2 text-sm text-ink-700 leading-relaxed">
        Thanks for sending that over — we&apos;ll let your director know.
      </p>
      <div className="mt-6">
        <Button fullWidth variant="secondary" onClick={onAnother}>
          Add another document
        </Button>
      </div>
    </div>
  );
}

// ---- OCR confirmation card -----------------------------------------------

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
  const [submitError, setSubmitError] = useState<string | null>(null);

  // Confidence buckets: high (≥0.9), medium (0.6–0.9), low (<0.6).
  const lowConfidence = confidence > 0 && confidence < 0.6;
  const mediumConfidence = confidence >= 0.6 && confidence < 0.9;

  const canConfirm = !lowConfidence || (touchedKind && touchedDate);

  const submit = async (acceptedExtraction: boolean) => {
    setSubmitError(null);
    if (!docKind && !doesNotExpire) {
      setSubmitError('Please pick a document type before saving.');
      return;
    }
    if (!doesNotExpire && !expDate) {
      setSubmitError(
        "Please pick an expiration date, or check \"This document doesn't expire.\"",
      );
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
    } catch {
      setSubmitError(
        "Something went wrong on our end. Please try again in a minute.",
      );
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      {/* REQ-006 banner — never dismissible. Amber (caution) signals
          "please review", not "warning". */}
      <div
        className="rounded-lb bg-caution-50 p-4 ring-1 ring-caution-200"
        role="status"
      >
        <div className="flex items-start gap-3">
          <Sparkles className="mt-0.5 h-5 w-5 flex-none text-caution-700" aria-hidden />
          <div className="text-sm text-caution-700 leading-relaxed">
            <span className="text-ink-800">
              Levelbrook AI read this document for you. Please check the type
              and expiration date below — AI can make mistakes, so a quick
              double-check keeps your file accurate.
            </span>
            {lowConfidence && (
              <p className="mt-2 font-semibold text-caution-700">
                We weren&apos;t sure about this one. Please correct anything
                that looks off, then tap {submitLabel}.
              </p>
            )}
          </div>
        </div>
      </div>

      {/* Field 1: Document type */}
      <div className="rounded-lb bg-white p-5 shadow-card border border-line">
        <label
          htmlFor="ocr-doc-kind"
          className="block text-sm font-semibold text-ink-900"
        >
          Document type
        </label>
        <div className="mt-2 flex items-center gap-2">
          <input
            id="ocr-doc-kind"
            type="text"
            value={docKind}
            onChange={(e) => {
              setDocKind(e.target.value);
              setTouchedKind(true);
            }}
            className="block w-full rounded-lb-sm border border-line bg-white px-3.5 py-3 text-base text-ink-900 placeholder:text-ink-400 focus:border-brand-600 focus:outline-none focus:ring-2 focus:ring-brand-600/20"
          />
          {mediumConfidence && (
            <span className="whitespace-nowrap rounded-full bg-caution-100 px-2.5 py-1 text-xs font-semibold text-caution-700">
              Please review
            </span>
          )}
        </div>
      </div>

      {/* Field 2: Expiration */}
      <div className="rounded-lb bg-white p-5 shadow-card border border-line">
        <label
          htmlFor="ocr-exp-date"
          className="block text-sm font-semibold text-ink-900"
        >
          This document expires on
        </label>
        {!doesNotExpire && (
          <input
            id="ocr-exp-date"
            type="date"
            value={expDate}
            onChange={(e) => {
              setExpDate(e.target.value);
              setTouchedDate(true);
            }}
            className="mt-2 block w-full rounded-lb-sm border border-line bg-white px-3.5 py-3 text-base text-ink-900 focus:border-brand-600 focus:outline-none focus:ring-2 focus:ring-brand-600/20"
          />
        )}
        <label className="mt-4 flex items-center gap-2.5 text-sm text-ink-800">
          <input
            type="checkbox"
            className="h-5 w-5 rounded border-line text-brand-600 focus:ring-brand-500"
            checked={doesNotExpire}
            onChange={(e) => {
              setDoesNotExpire(e.target.checked);
              setTouchedDate(true);
            }}
          />
          This document doesn&apos;t expire
        </label>
      </div>

      {/* See the document accordion (manager mode) */}
      {documentURL && (
        <details className="rounded-lb bg-white p-5 shadow-card border border-line">
          <summary
            className="cursor-pointer text-sm font-medium text-ink-700 hover:text-ink-900"
            onClick={() => setShowDoc((s) => !s)}
          >
            {showDoc ? 'Hide the document' : 'See the document'}
          </summary>
          <div className="mt-3">
            <iframe
              title="document preview"
              src={documentURL}
              className="h-96 w-full rounded-lb-sm border border-line"
            />
          </div>
        </details>
      )}

      {submitError && (
        <div
          className="rounded-lb-sm bg-critical-50 px-4 py-3 text-sm text-critical-700 ring-1 ring-critical-200"
          role="alert"
        >
          {submitError}
        </div>
      )}

      {/* Buttons — "Looks right" on top on mobile, side-by-side on >= sm. */}
      <div className="flex flex-col gap-3 sm:flex-row-reverse">
        <Button
          size="lg"
          fullWidth
          disabled={!canConfirm}
          loading={busy}
          onClick={() => void submit(true)}
        >
          {submitLabel}
        </Button>
        <Button
          size="lg"
          fullWidth
          variant="secondary"
          disabled={busy}
          onClick={() => void submit(false)}
        >
          Fix this
        </Button>
      </div>
    </div>
  );
}
