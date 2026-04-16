# ComplianceKit PDF E-Signature — Design Spec

Status: **Draft v1.0** \u00b7 Owner: founder \u00b7 Last updated: 2026-04-16

## 1. Goals & Non-Goals

### Goals

1. **Let daycare providers send PDFs to parents / staff for electronic
   signature**, receive back a tamper-evident signed PDF, and have a
   legally-reasonable audit trail.
2. **Build in-house, free of per-signature fees.** No DocuSign, no PSPDFKit,
   no HelloSign. The founder has previously paid $5,000 *just for field
   printing onto PDFs* with a homebrew solution; we replace that here.
3. **Let providers drag signature / date / text / checkbox fields onto any
   PDF they upload.** No pre-authored templates required.
4. **Compliance with the federal ESIGN Act (15 U.S.C. \u00a7 7001 et seq.) and
   state UETA enactments** (California, Texas, Florida all have UETA).
5. **Self-hosted end to end** — S3 for blobs, Postgres for rows, Go + chi
   for API, React + Vite for UI. Zero third-party SaaS on the critical path.

### Non-Goals (v1)

- **Qualified electronic signatures (QES)** under EU eIDAS. We are US-only;
  we are not building PKI-backed signatures in v1. See \u00a7 10 for the upgrade
  path to PAdES-B-B.
- **Multi-party / sequential signing** with different signers per field. v1
  supports exactly one signer per session. Multi-signer is a v2 feature and
  the `Field.AssignedSignerID` column is already reserved for it.
- **Certified timestamping** via an RFC 3161 TSA. Our timestamps are server
  wall-clock; good enough for ESIGN, not good enough for courts that demand
  a trusted timestamp authority.
- **In-PDF digital signature fields** (the pdf-lib way of filling a PDF's
  native `/Sig` dictionary). We stamp a PNG onto the page because that is
  what 99% of daycare inspection workflows expect visually.

## 2. Why we built our own

| Vendor | Cost at our scale | Why rejected |
|--------|-------------------|--------------|
| DocuSign API | ~$40/envelope/mo at plan, per-envelope pricing above | Founder has explicitly refused; per-envelope costs kill unit economics on a $99/mo SaaS. |
| HelloSign / Dropbox Sign | $0.50\u2013$3 / signature | Same unit-economics problem. |
| PSPDFKit | 5-figure annual contract, offline pricing | Overkill; we don't need full-featured PDF editing. |
| Adobe Acrobat Sign | Per-transaction pricing | Same. |

Beyond cost, we want:

- Full control over the audit trail JSON shape (our `legal/
  signature-audit-trail-schema.md` owns it).
- No third-party data processor in the signer's journey (parents uploading
  their child's immunization records would otherwise route through a third
  party — adds to the DPA / subprocessor list).
- Ability to insert the signed PDF directly into our Compliance Dashboard
  timeline without a callback webhook.

## 3. Threat Model

| # | Threat | Mitigation |
|---|--------|------------|
| T1 | **Payload tampering.** Attacker alters the PDF after signing. | SHA-256 chain: `sha256Before` (unsigned) and `sha256After` (stamped + audit page) stored in three places (DB row, audit JSON in S3, certificate page inside the PDF). `Service.VerifyIntegrity` flags any mismatch. |
| T2 | **Replay.** Attacker submits a previously captured Finalize payload. | Session tokens are one-shot; status flips to `completed` on success and further finalize calls return HTTP 410 Gone. |
| T3 | **Impersonation.** Attacker signs on behalf of someone else. | Tokens are 32 random bytes (\u2248 190 bits entropy) and only delivered to the signer's verified email. Our v1 trust anchor is email possession; v2 adds SMS OTP / WebAuthn. |
| T4 | **Token theft** (forwarded email, device compromise). | 14-day default TTL; provider can revoke. We record IP + UA in the audit record so investigators can spot anomalies. Signers see the typed-name affirmation before submission, giving email-forward attackers one more hurdle. |
| T5 | **Malicious PDF upload** (template side). | Server enforces `%PDF-` header check + size cap (25 MB). v2: server-side render via pdfcpu to catch malformed PDFs early. |
| T6 | **Content confusion** (upload HTML disguised as PDF). | Same header check. Content-Type is stamped server-side, never trusted from the client. |
| T7 | **Client hash lying.** Browser submits a hash that does not match the bytes. | Server re-hashes on receipt; mismatch rejects the upload. Client hash is a helper, not a trust anchor. |
| T8 | **Audit bucket corruption** (accidental delete / rewrite). | `ck-audit-trail` uses S3 versioning + object lock in compliance mode. DB row is the secondary record; either one alone is sufficient to identify the signature event. |
| T9 | **PII leak via audit JSON.** | Audit JSON contains email + IP + UA. It does NOT contain the raw signature PNG (that is inside the PDF). Bucket is encrypted at rest (SSE-KMS, per-tenant key in v2). |
| T10 | **Log4Shell-class deserialization on audit JSON.** | We `json.Unmarshal` into a typed `AuditRecord` struct — no interface{} soup — on every read. No template evaluation. |

## 4. Sign Session Lifecycle

```
                     POST /sessions (provider)
                               \u2502
                               \u25bc
                          [ pending ]
                               \u2502
                   GET /sessions/:token (signer)
                               \u2502
                               \u25bc
                       [ in_progress ]
                               \u2502
              \u250c\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u253c\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2510
              \u2502                \u2502                  \u2502
        POST finalize       (TTL elapses)          (provider
              \u2502                \u2502                  revokes)
              \u25bc                \u25bc                  \u25bc
        [ completed ]      [ expired ]           [ revoked ]
              \u2502
              \u25bc
     \u2022 signed PDF in S3
     \u2022 audit JSON in S3
     \u2022 signatures row in PG
```

Only `completed` is a success terminal. `expired` / `revoked` are final and
not resumable; the provider must create a fresh session to re-invite.

## 5. PDF Field Placement Protocol

### 5.1 Coordinate System

Every field is stored with:

- `pageIndex` (0-based, integer)
- `x`, `y` (number, PDF points, origin **bottom-left** of the page)
- `width`, `height` (number, PDF points)

This matches pdf-lib's `drawImage({ x, y, width, height })` call, so no
coordinate flipping is needed during stamping. The browser renders with a
top-left origin; the FieldOverlay component does the flip at the UI layer
only.

### 5.2 JSON shape

```json
{
  "id": "7fK2...a",
  "type": "signature",
  "pageIndex": 0,
  "x": 123.4,
  "y": 220.5,
  "width": 180,
  "height": 42,
  "required": true,
  "label": "Parent signature",
  "assignedSignerId": null
}
```

Persisted as a JSONB array on `document_template_fields.fields_json`, and
**frozen onto the `sign_sessions` row** at invitation time so that a later
template edit cannot retroactively change an outstanding invitation.

### 5.3 Supported field types

| Type | Rendered by | Notes |
|------|-------------|-------|
| `signature` | Modal SignaturePad \u2192 PNG stamped via pdf-lib | Always drawn as an image. |
| `initial` | Same as signature, with smaller default size | Distinct for UX; same pdf-lib call. |
| `date` | Auto-filled with today's date on click | US format by default; localizable. |
| `text` | `window.prompt()` (v1) / rich input (v2) | Stamped with Helvetica at min(14, height-4) pt. |
| `checkbox` | Tap to toggle | Stamped as a 1pt border rectangle + X diagonals when checked. |

## 6. Audit Trail Schema

Matches `legal/signature-audit-trail-schema.md` when that file is authored.
Canonical Go struct is `pdfsign.AuditRecord`. Canonical TS shape is
`AuditRecord` in `frontend/src/components/PdfSigner/types.ts`. They agree
field-for-field.

Notable properties:

- `schemaVersion: "1.0"` — additive v1.1+ changes only (new optional fields).
  Renaming or removing a field bumps to v2.
- `sha256Before` and `sha256After` are the tamper-evidence anchors.
- `consent.signerTypedName` is the legal affirmation of intent.
- `ipAddress` and `userAgent` are server-populated; the client copy is
  overwritten.
- `fieldValues` is a sparse echo — filled-or-not, no raw PNG data.

## 7. S3 Key Strategy

| Bucket | Purpose | Key pattern | Lifecycle |
|--------|---------|-------------|-----------|
| `ck-templates` | Blank PDFs the provider uploaded | `{provider_id}/templates/{document_id}.pdf` | Deletable by provider |
| `ck-signed-pdfs` | Final stamped PDFs | `{provider_id}/{document_id}/{signature_id}.pdf` | Never deleted; versioned |
| `ck-audit-trail` | Audit JSON | `{provider_id}/{signature_id}.json` | Object-lock enabled (compliance mode) |

### 7.1 Least-privilege IAM policy (signed bucket example)

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "BackendWriteSigned",
      "Effect": "Allow",
      "Principal": { "AWS": "arn:aws:iam::ACCT:role/ck-backend" },
      "Action": ["s3:PutObject"],
      "Resource": "arn:aws:s3:::ck-signed-pdfs/*",
      "Condition": {
        "StringEquals": {
          "s3:x-amz-server-side-encryption": "aws:kms"
        }
      }
    },
    {
      "Sid": "BackendReadSigned",
      "Effect": "Allow",
      "Principal": { "AWS": "arn:aws:iam::ACCT:role/ck-backend" },
      "Action": ["s3:GetObject"],
      "Resource": "arn:aws:s3:::ck-signed-pdfs/*"
    },
    {
      "Sid": "DenyNonSSE",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "s3:PutObject",
      "Resource": "arn:aws:s3:::ck-signed-pdfs/*",
      "Condition": {
        "StringNotEquals": {
          "s3:x-amz-server-side-encryption": "aws:kms"
        }
      }
    },
    {
      "Sid": "DenyUnencryptedTransport",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "s3:*",
      "Resource": ["arn:aws:s3:::ck-signed-pdfs", "arn:aws:s3:::ck-signed-pdfs/*"],
      "Condition": { "Bool": { "aws:SecureTransport": "false" } }
    }
  ]
}
```

The audit bucket adds `s3:Object Lock` + denies `s3:DeleteObject` for the
backend role, so an application bug cannot overwrite history.

### 7.2 Key-naming rationale

- Provider prefix first \u2192 enables per-tenant bucket policies and
  straightforward lifecycle rules.
- Signature ID last in `ck-signed-pdfs` so one provider can have many
  documents with parallel uploads without contention.
- Signature ID flat in `ck-audit-trail` because no further nesting is useful
  there; the JSON is always small (< 10 KB).

## 8. DB Schema Summary

Full DDL is at the top of `backend/internal/pdfsign/store.go`. Migration
numbering lives in `backend/migrations/`. Tables:

- `document_templates` — one row per uploaded blank PDF.
- `document_template_fields` — 1:1 with the template; holds the `fields_json`.
- `sign_sessions` — one row per invitation (the base62 token is PK).
- `signatures` — one row per completed signature; append-only.

A `policy_versions` table already exists (see `legal/README.md`) and holds
the versioned ESIGN Disclosure text. The `audit.consent.esignDisclosureVersion`
field joins back to `policy_versions.version_label`.

## 9. HTTP API Summary

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/pdfsign/sessions` | Provider session cookie | Create a signing invitation |
| GET | `/api/pdfsign/sessions/:token` | No auth (token IS the credential) | Load session; returns pre-signed PDF URL |
| POST | `/api/pdfsign/sessions/:token/finalize` | Token only | Upload stamped PDF + audit JSON |
| GET | `/api/pdfsign/templates` | Provider session cookie | List templates with field counts |
| PUT | `/api/pdfsign/templates/:id/fields` | Provider session cookie | Save template field layout |

All unprotected endpoints live under `/api/pdfsign` and are rate-limited at
the edge. See `backend/internal/middleware`.

## 10. Upgrade Path: PAdES-B-B (v2)

v1 signatures are visually stamped PNGs + SHA-256 anchors. This is sufficient
for ESIGN / UETA compliance in US child care contexts, but is **not**
cryptographically self-verifying: a regulator cannot, without the audit JSON
and the DB row, prove the signature is authentic using only the PDF file.

To upgrade to **PAdES-B-B** (the baseline profile of ETSI EN 319 142) without
rewriting the client:

1. Issue a long-lived X.509 signing certificate for the ComplianceKit platform
   (either self-signed for internal use or from a commercial CA like
   GlobalSign for external trust).
2. After the browser uploads the stamped PDF, run it through
   [`pdfcpu`](https://github.com/pdfcpu/pdfcpu) or the Go port of the iText
   algorithm to add a `/Sig` dictionary with:
   - `/SubFilter /ETSI.CAdES.detached`
   - CAdES-BES signed attributes (signing-cert-v2, signing-time).
   - PKCS#7 blob signed by the platform's private key (kept in AWS KMS).
3. Emit the augmented PDF as a new version in `ck-signed-pdfs` with suffix
   `.pades.pdf`. Keep the original as the primary; the PAdES version is a
   bonus artifact for signers who ask for it.
4. In the audit JSON, record the signing cert's SHA-256 and the signature
   value's SHA-256.

This is a pure backend change; no client UX moves. It adds ~0.5 s per
signature (the KMS sign call) and unlocks "notarization-grade" signatures
for states that eventually require them.

## 11. Open Questions

- **Retention clock.** Does the signature retention period start at
  `signed_at`, or at the date the child/staff member leaves the provider?
  Per-state legal review required.
- **Parent language.** Our parent consent forms are EN + ES. Should the
  ESIGN disclosure + signing UI also be ES? Likely yes for CA/TX/FL.
- **Email proof-of-delivery.** Do we need an SPF/DKIM-signed email log as
  part of the attribution chain? Probably not for v1, but revisit when we
  see our first signature dispute.

## 12. Appendix \u2014 File Map

| File | Responsibility |
|------|----------------|
| `frontend/src/components/PdfSigner/PdfSigner.tsx` | Signer orchestrator |
| `frontend/src/components/PdfSigner/SignaturePad.tsx` | Canvas-based signature capture |
| `frontend/src/components/PdfSigner/FieldOverlay.tsx` | Field rendering, signer + authoring modes |
| `frontend/src/components/PdfSigner/FieldDesigner.tsx` | Provider-side field authoring UI |
| `frontend/src/components/PdfSigner/pdfStamp.ts` | Pure pdf-lib stamping + SHA-256 |
| `frontend/src/components/PdfSigner/api.ts` | Fetch wrappers |
| `frontend/src/components/PdfSigner/types.ts` | Shared TS types |
| `frontend/src/pages/SignDocument.tsx` | `/sign/:token` route |
| `frontend/src/pages/DocumentTemplates.tsx` | Provider template list |
| `backend/internal/pdfsign/pdfsign.go` | Types + Service definition |
| `backend/internal/pdfsign/finalize.go` | Finalize pipeline |
| `backend/internal/pdfsign/store.go` | Postgres (pgx) Store |
| `backend/internal/pdfsign/http.go` | chi handlers |
| `backend/internal/pdfsign/token.go` | Base62 + SHA-256 helpers |
| `backend/internal/pdfsign/pdfsign_test.go` | Unit tests |
