# PdfSigner — In-house PDF e-Signature Component

ComplianceKit's self-hosted alternative to DocuSign / PSPDFKit / Adobe Sign.
Zero per-signature fees, full control over the audit trail, no third-party
data-processor dependency. Legally reasonable under the U.S. ESIGN Act and
state UETA enactments, subject to the caveats at the bottom of this document.

## Files

| File | Purpose |
|------|---------|
| `types.ts` | Shared TypeScript types (`Field`, `SignSession`, `AuditRecord`, etc.) |
| `pdfStamp.ts` | Pure functions that stamp signatures/text/checkboxes and append audit pages. Also SHA-256 via `crypto.subtle`. |
| `SignaturePad.tsx` | Wrapper around [`signature_pad`](https://github.com/szimek/signature_pad) with touch / mouse / stylus support, `getPng()`, clear, undo. |
| `FieldOverlay.tsx` | Translates PDF-point field rectangles into absolute-positioned DOM overlays. Dual-mode: signer (read-only) and authoring (draggable/resizable). |
| `PdfSigner.tsx` | Main orchestrator. Loads the PDF, renders pages with `react-pdf`, collects field values, produces the signed PDF and audit record, calls `onComplete`. |
| `FieldDesigner.tsx` | Provider-side authoring UI. Drag signature / date / text / checkbox fields onto any page. Persists to `document_template_fields`. |
| `api.ts` | HTTP client for session load / finalize / template save. |

## Sequence Diagram

```
Provider UI                          Browser (Signer)                   Go Backend                   S3 / Postgres
    |                                         |                               |                              |
    | POST /api/pdfsign/sessions              |                               |                              |
    | (templateId, signer, fields)            |                               |                              |
    |---------------------------------------->|                               |                              |
    |                                         |            CreateSession()---->|   INSERT sign_sessions       |
    |<-- { token, signingUrl } ---------------|                               |----------------------------->|
    |                                         |                               |                              |
    | (email signingUrl to signer)            |                               |                              |
    |                                         |                               |                              |
    |                                         | GET /sign/:token              |                              |
    |                                         |------------------------------>|   SELECT sign_sessions       |
    |                                         |<-- SignSession + documentUrl -|                              |
    |                                         |                               |                              |
    |                                         | GET documentUrl (S3 pre-signed)                              |
    |                                         |------------------------------------------------------------->|
    |                                         |<-------------------------------- PDF bytes ------------------|
    |                                         |                                                              |
    |                                         | render pages (react-pdf)                                      |
    |                                         | capture signature (signature_pad)                             |
    |                                         | stamp PDF (pdf-lib)                                           |
    |                                         | sha256Before, sha256After                                     |
    |                                         | append audit certificate page                                 |
    |                                         |                                                              |
    |                                         | POST /sessions/:token/finalize                                |
    |                                         | multipart: pdf=<blob>, audit=<json>                           |
    |                                         |------------------------------>|                              |
    |                                         |                               | recompute sha256             |
    |                                         |                               | verify %PDF header           |
    |                                         |                               | PUT s3://ck-signed-pdfs/...  |
    |                                         |                               | PUT s3://ck-audit-trail/...  |
    |                                         |                               | INSERT signatures            |
    |                                         |                               |----------------------------->|
    |                                         |<-- SignatureRecord -----------|                              |
```

## How the audit trail works

Two SHA-256 hashes are captured for every signature:

- `sha256Before` — the unmodified PDF bytes the signer received from S3.
- `sha256After` — the final bytes after field stamping + audit-certificate page.

Both are recorded in (a) the Postgres `signatures` row, (b) the JSON stored in
the `ck-audit-trail` bucket, and (c) the rendered certificate page on the last
page of the signed PDF itself. Any future byte-level modification of the signed
PDF that is not reflected in all three locations is detectable.

## Legal caveats (read me)

This component alone does **not** make a document "a legally binding signed
document". It implements the technical requirements of ESIGN / UETA, but legal
validity additionally requires:

1. **Intent to sign** — satisfied by the affirmative "Finalize signature" click
   and the typed-name affirmation field.
2. **Consent to do business electronically** — satisfied by the mandatory
   checkbox linking the versioned ESIGN Disclosure (`esignature-disclosure.md`).
3. **Association of signature with the record** — satisfied by stamping the PNG
   signature directly onto the PDF via pdf-lib, plus the cryptographic hash.
4. **Record retention** — the provider must retain the signed PDF + audit JSON
   for the period required by their state (CA: 3 years minimum for most child
   care records; TX: 5 years for employee/child files; FL: varies).
5. **Attribution** — we record IP, User-Agent, session token (tied to the
   original invitation link sent to the signer's verified email). For
   higher-assurance signatures (notarizations, etc.) layer SMS OTP or WebAuthn
   on top — not in scope for v1.

This is **not** a qualified electronic signature (QES) under eIDAS and should
not be used where a QES is legally required. See
`docs/pdf-signature-spec.md` \u00a7 "Upgrade Path" for PAdES-B-B integration later.

## Development notes

- The pdf.js worker must be served at `/pdf.worker.min.mjs`. See
  `package-deps.md` for the vite config snippet.
- `crypto.subtle.digest` requires a secure context (HTTPS or localhost).
- The audit JSON intentionally **does not** include the raw signature PNG
  (those live inside the signed PDF). The audit trail references filled fields
  by id and page index only, so the JSON stays small and PII-lean.
- SHA-256 of the final PDF is recomputed server-side and rejected if it does
  not match what the client claimed — prevents a malicious client from sending
  one hash but a different file.
