import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Document, Page, pdfjs } from "react-pdf";
import { PDFDocument } from "pdf-lib";
import { FieldOverlay } from "./FieldOverlay";
import { SignaturePad, type SignaturePadHandle } from "./SignaturePad";
import {
  appendAuditPage,
  dataUrlToBytes,
  sha256,
  stampCheckbox,
  stampSignature,
  stampText,
} from "./pdfStamp";
import type {
  AuditRecord,
  ConsentRecord,
  Field,
  FieldValue,
  SignSession,
} from "./types";

// Wire pdf.js worker. The caller's vite.config must copy
// pdfjs-dist/build/pdf.worker.min.mjs to /pdf.worker.min.mjs (see package-deps.md).
pdfjs.GlobalWorkerOptions.workerSrc = "/pdf.worker.min.mjs";

const ESIGN_DISCLOSURE_VERSION_FALLBACK = "v1.0.0";

export interface PdfSignerProps {
  session: SignSession;
  onComplete: (args: {
    signedBlob: Blob;
    auditRecord: AuditRecord;
    sha256Before: string;
    sha256After: string;
  }) => void | Promise<void>;
  onError?: (err: Error) => void;
}

export function PdfSigner({ session, onComplete, onError }: PdfSignerProps) {
  const [pdfBytes, setPdfBytes] = useState<Uint8Array | null>(null);
  const [pageCount, setPageCount] = useState<number>(0);
  const [pageSizes, setPageSizes] = useState<Array<{ w: number; h: number }>>([]);
  const [renderedWidths, setRenderedWidths] = useState<Record<number, number>>({});
  const [values, setValues] = useState<Record<string, FieldValue>>({});
  const [activeField, setActiveField] = useState<Field | null>(null);
  const [consentAccepted, setConsentAccepted] = useState(false);
  const [typedName, setTypedName] = useState(session.signerName);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const padRef = useRef<SignaturePadHandle | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);

  // Fetch the original PDF once so we can stamp it client-side at finalize.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const resp = await fetch(session.documentUrl, {
          credentials: "include",
        });
        if (!resp.ok) throw new Error(`fetch document: ${resp.status}`);
        const buf = new Uint8Array(await resp.arrayBuffer());
        if (!cancelled) setPdfBytes(buf);
      } catch (err) {
        if (!cancelled) setError((err as Error).message);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [session.documentUrl]);

  const fileForReactPdf = useMemo(() => {
    if (!pdfBytes) return null;
    // Clone so react-pdf/pdfjs cannot detach the buffer out from under us.
    return { data: new Uint8Array(pdfBytes) };
  }, [pdfBytes]);

  const filledFieldIds = useMemo(() => {
    const s = new Set<string>();
    for (const f of session.fields) {
      const v = values[f.id];
      if (!v) continue;
      if (v.type === "signature" && v.signaturePngBase64) s.add(f.id);
      if (v.type === "initial" && v.signaturePngBase64) s.add(f.id);
      if ((v.type === "date" || v.type === "text") && v.textValue) s.add(f.id);
      if (v.type === "checkbox" && v.checkboxChecked !== undefined) s.add(f.id);
    }
    return s;
  }, [values, session.fields]);

  const allRequiredFilled = useMemo(() => {
    return session.fields.every((f) => !f.required || filledFieldIds.has(f.id));
  }, [session.fields, filledFieldIds]);

  const onDocumentLoad = useCallback(async ({ numPages }: { numPages: number }) => {
    setPageCount(numPages);
    if (!pdfBytes) return;
    const doc = await PDFDocument.load(pdfBytes);
    const sizes = doc.getPages().map((p) => {
      const { width, height } = p.getSize();
      return { w: width, h: height };
    });
    setPageSizes(sizes);
  }, [pdfBytes]);

  const onPageRender = useCallback((pageIndex: number, w: number) => {
    setRenderedWidths((prev) => ({ ...prev, [pageIndex]: w }));
  }, []);

  const openField = useCallback((f: Field) => {
    setActiveField(f);
  }, []);

  const commitSignatureField = useCallback(async () => {
    if (!activeField) return;
    const pad = padRef.current;
    if (!pad) return;
    const dataUrl = pad.getDataUrl();
    if (!dataUrl) {
      setError("Please draw a signature first.");
      return;
    }
    const base64 = dataUrl.slice(dataUrl.indexOf(",") + 1);
    setValues((prev) => ({
      ...prev,
      [activeField.id]: {
        fieldId: activeField.id,
        type: activeField.type,
        signaturePngBase64: base64,
      },
    }));
    setActiveField(null);
    pad.clear();
    setError(null);
  }, [activeField]);

  const commitTextField = useCallback((field: Field, text: string) => {
    setValues((prev) => ({
      ...prev,
      [field.id]: { fieldId: field.id, type: field.type, textValue: text },
    }));
  }, []);

  const toggleCheckbox = useCallback((field: Field) => {
    setValues((prev) => {
      const current = prev[field.id];
      return {
        ...prev,
        [field.id]: {
          fieldId: field.id,
          type: "checkbox",
          checkboxChecked: !(current?.checkboxChecked ?? false),
        },
      };
    });
  }, []);

  const handleFinalize = useCallback(async () => {
    if (!pdfBytes) return;
    if (!consentAccepted) {
      setError("You must accept the electronic signature disclosure.");
      return;
    }
    if (!allRequiredFilled) {
      setError("Please complete all required fields.");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const sha256Before = await sha256(pdfBytes);
      let working = pdfBytes;

      for (const f of session.fields) {
        const v = values[f.id];
        if (!v) continue;
        if ((v.type === "signature" || v.type === "initial") && v.signaturePngBase64) {
          const png = dataUrlToBytes(`data:image/png;base64,${v.signaturePngBase64}`);
          working = await stampSignature(working, {
            pageIndex: f.pageIndex,
            x: f.x,
            y: f.y,
            width: f.width,
            height: f.height,
            pngBytes: png,
          });
        } else if ((v.type === "date" || v.type === "text") && v.textValue) {
          working = await stampText(working, {
            pageIndex: f.pageIndex,
            x: f.x,
            y: f.y + 3,
            fontSize: Math.min(14, f.height - 4),
            text: v.textValue,
          });
        } else if (v.type === "checkbox") {
          working = await stampCheckbox(working, {
            pageIndex: f.pageIndex,
            x: f.x,
            y: f.y,
            size: Math.min(f.width, f.height),
            checked: !!v.checkboxChecked,
          });
        }
      }

      const sha256AfterStamp = await sha256(working);

      const consent: ConsentRecord = {
        esignDisclosureVersion:
          session.esignDisclosureVersion || ESIGN_DISCLOSURE_VERSION_FALLBACK,
        acceptedAt: new Date().toISOString(),
        signerTypedName: typedName.trim(),
      };

      const clientTimeZone =
        Intl.DateTimeFormat().resolvedOptions().timeZone ?? "unknown";

      const audit: AuditRecord = {
        schemaVersion: "1.0",
        signatureId: genClientSignatureId(),
        sessionToken: session.token,
        documentId: session.documentId,
        providerId: session.providerId,
        signerName: session.signerName,
        signerEmail: session.signerEmail,
        signedAt: new Date().toISOString(),
        userAgent: navigator.userAgent,
        sha256Before,
        sha256After: sha256AfterStamp,
        consent,
        fieldValues: session.fields.map((f) => ({
          fieldId: f.id,
          type: f.type,
          pageIndex: f.pageIndex,
          filled: filledFieldIds.has(f.id),
        })),
        clientTimeZone,
      };

      const withAudit = await appendAuditPage(working, audit);
      const sha256Final = await sha256(withAudit);
      audit.sha256After = sha256Final;

      // Blob's BlobPart requires ArrayBuffer-backed Uint8Array in TS 5.6+.
      // Copy into a fresh Uint8Array<ArrayBuffer> to satisfy the typings.
      const blob = new Blob([new Uint8Array(withAudit)], { type: "application/pdf" });
      await onComplete({
        signedBlob: blob,
        auditRecord: audit,
        sha256Before,
        sha256After: sha256Final,
      });
    } catch (err) {
      const e = err as Error;
      setError(e.message);
      onError?.(e);
    } finally {
      setSubmitting(false);
    }
  }, [
    pdfBytes,
    consentAccepted,
    allRequiredFilled,
    session,
    values,
    typedName,
    filledFieldIds,
    onComplete,
    onError,
  ]);

  if (error && !pdfBytes) {
    return <div role="alert">Failed to load document: {error}</div>;
  }
  if (!fileForReactPdf) {
    return <div>Loading document\u2026</div>;
  }

  return (
    <div
      ref={containerRef}
      className="pdfsign-root"
      style={{ display: "flex", flexDirection: "column", gap: 24 }}
    >
      <Document
        file={fileForReactPdf}
        onLoadSuccess={onDocumentLoad}
        onLoadError={(e) => setError(e.message)}
        loading={<div>Rendering\u2026</div>}
      >
        {Array.from({ length: pageCount }, (_, idx) => {
          const ps = pageSizes[idx];
          const pageFields = session.fields.filter((f) => f.pageIndex === idx);
          const rw = renderedWidths[idx] ?? 816; // ~8.5in @ 96dpi
          const rh = ps ? (rw * ps.h) / ps.w : 1056;
          return (
            <div
              key={idx}
              style={{ position: "relative", margin: "0 auto 16px" }}
            >
              <Page
                pageNumber={idx + 1}
                width={rw}
                onRenderSuccess={() => {
                  if (ps) onPageRender(idx, rw);
                }}
                renderAnnotationLayer={false}
                renderTextLayer={false}
              />
              {ps && (
                <FieldOverlay
                  fields={pageFields}
                  renderedWidth={rw}
                  renderedHeight={rh}
                  pageWidth={ps.w}
                  pageHeight={ps.h}
                  filledFieldIds={filledFieldIds}
                  onFieldClick={(f) => {
                    if (f.type === "checkbox") toggleCheckbox(f);
                    else if (f.type === "date") {
                      commitTextField(f, new Date().toLocaleDateString("en-US"));
                    } else if (f.type === "text") {
                      const v = window.prompt(f.label ?? "Enter text", values[f.id]?.textValue ?? "");
                      if (v !== null) commitTextField(f, v);
                    } else {
                      openField(f);
                    }
                  }}
                />
              )}
            </div>
          );
        })}
      </Document>

      {activeField && (
        <div
          role="dialog"
          aria-label="Capture signature"
          style={{
            position: "fixed",
            inset: 0,
            background: "rgba(0,0,0,0.35)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            zIndex: 50,
          }}
          onClick={(e) => {
            if (e.target === e.currentTarget) setActiveField(null);
          }}
        >
          <div
            style={{
              background: "#fff",
              padding: 20,
              borderRadius: 8,
              boxShadow: "0 12px 32px rgba(0,0,0,0.25)",
              minWidth: 600,
            }}
          >
            <h3 style={{ marginTop: 0 }}>
              {activeField.type === "initial" ? "Initial" : "Sign"}
            </h3>
            <SignaturePad ref={padRef} width={560} height={180} />
            <div style={{ display: "flex", gap: 8, marginTop: 12 }}>
              <button type="button" onClick={() => padRef.current?.clear()}>
                Clear
              </button>
              <button type="button" onClick={() => padRef.current?.undo()}>
                Undo
              </button>
              <span style={{ flex: 1 }} />
              <button type="button" onClick={() => setActiveField(null)}>
                Cancel
              </button>
              <button type="button" onClick={commitSignatureField}>
                Apply
              </button>
            </div>
          </div>
        </div>
      )}

      <section
        style={{
          border: "1px solid #d0d7de",
          borderRadius: 6,
          padding: 16,
          background: "#f6f8fa",
        }}
      >
        <h3 style={{ marginTop: 0 }}>Confirm &amp; Sign</h3>
        <label style={{ display: "flex", gap: 8, alignItems: "flex-start" }}>
          <input
            type="checkbox"
            checked={consentAccepted}
            onChange={(e) => setConsentAccepted(e.target.checked)}
          />
          <span>
            I have read the{" "}
            <a href="/legal/esignature-disclosure" target="_blank" rel="noreferrer">
              Electronic Signature Disclosure
            </a>{" "}
            (version {session.esignDisclosureVersion || ESIGN_DISCLOSURE_VERSION_FALLBACK})
            and consent to sign this document electronically under the ESIGN Act and
            applicable state UETA provisions.
          </span>
        </label>
        <div style={{ marginTop: 12 }}>
          <label style={{ display: "block", fontSize: 13, marginBottom: 4 }}>
            Typed full legal name (affirmation of intent)
          </label>
          <input
            type="text"
            value={typedName}
            onChange={(e) => setTypedName(e.target.value)}
            style={{
              width: 360,
              padding: "6px 8px",
              border: "1px solid #d0d7de",
              borderRadius: 4,
            }}
          />
        </div>
        {error && (
          <div role="alert" style={{ marginTop: 12, color: "#cf222e" }}>
            {error}
          </div>
        )}
        <div style={{ marginTop: 16 }}>
          <button
            type="button"
            onClick={handleFinalize}
            disabled={submitting || !consentAccepted || !allRequiredFilled || !typedName.trim()}
            style={{
              background: "#1f883d",
              color: "#fff",
              border: "none",
              padding: "10px 20px",
              borderRadius: 6,
              fontSize: 15,
              cursor: submitting ? "wait" : "pointer",
              opacity: submitting ? 0.6 : 1,
            }}
          >
            {submitting ? "Finalizing\u2026" : "Finalize signature"}
          </button>
        </div>
      </section>
    </div>
  );
}

function genClientSignatureId(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return toBase62(bytes);
}

const B62 = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ";

function toBase62(bytes: Uint8Array): string {
  // BigInt conversion then base62. Fine for 32-byte inputs.
  let n = 0n;
  for (const b of bytes) n = (n << 8n) | BigInt(b);
  if (n === 0n) return "0";
  let s = "";
  const base = 62n;
  while (n > 0n) {
    const r = Number(n % base);
    s = B62[r] + s;
    n = n / base;
  }
  return s;
}
