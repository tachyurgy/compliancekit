# pdfsign (Go package)

Server half of ComplianceKit's in-house PDF e-signature feature. Handles
session issuance, upload of the browser-stamped PDF, SHA-256 verification,
S3 persistence, and the audit trail.

See `docs/pdf-signature-spec.md` for the full design. See
`frontend/src/components/PdfSigner/README.md` for the browser half.

## Architecture

```
HTTP (chi router)
  └── Handlers        (http.go)          - transport-layer validation
        └── Service   (pdfsign.go)       - business logic
              ├── Store   (store.go)     - pgx-backed Postgres
              └── BlobStore              - S3 via aws-sdk-go-v2
```

`Service` is the only type the HTTP layer touches. `Store` and `BlobStore` are
interfaces so tests can swap in the in-memory fakes in `pdfsign_test.go`.

## Security properties

1. **SHA-256 chain.** The client hashes the original PDF (`sha256Before`) and
   the stamped PDF (`sha256After`). The server recomputes `sha256After` on
   receipt and refuses the upload if the claim does not match. Both hashes
   are written to the DB row, the audit JSON, and the certificate page of
   the PDF itself. Any downstream tampering fails `VerifyIntegrity`.
2. **Payload header check.** Server rejects any request whose first five bytes
   are not `%PDF-`. This is not a full parse, but blocks the trivial content
   confusion attack (upload a malicious HTML/SVG disguised as a PDF).
3. **Upload size cap.** `FinalizeInput.PDFMaxBytes` (default 25 MB) prevents
   memory exhaustion attacks. Enforced twice: at `ParseMultipartForm` and at
   the inner `readAllCapped`.
4. **One-shot session.** Tokens are 32 random bytes encoded as base62 (~43
   chars, ~190 bits entropy). After a successful finalize the session status
   is flipped to `completed` and further attempts return HTTP 410 Gone.
5. **IP + UA attribution.** The server — never the client — records the IP
   (via `X-Forwarded-For` when behind a trusted L7 proxy) and `User-Agent`.
   Clients cannot forge these fields in the audit record.
6. **Separate buckets for signed PDFs and audit JSON.** Two IAM policies, two
   blast radii. The audit bucket is written once per signature and never
   mutated; it should be configured with versioning + object lock (see spec
   \u00a7 "S3 Key Strategy + IAM Policy").

### What this does NOT protect against

- **Compromised browser session** — if a signer's device is owned, an
  attacker can sign on their behalf. Outside the threat model of v1.
- **Collusion** — we trust the operator (ComplianceKit) not to fabricate
  signatures. For higher-assurance needs, see the PAdES-B-B upgrade path in
  the spec.

## S3 key layout

All objects live in the single `ck-files` bucket.

| Prefix | Key format | Content-Type | Notes |
|--------|------------|--------------|-------|
| `templates/` | `templates/{provider_id}/{document_id}.pdf` | `application/pdf` | Provider-writable, signer-readable via pre-signed URL only |
| `signed/` | `signed/{provider_id}/{document_id}/{signature_id}.pdf` | `application/pdf` | Write-once (app-enforced); owner-readable |
| `audit/` | `audit/{provider_id}/{signature_id}.json` | `application/json` | Write-once (app-enforced) |

## DB schema (prose)

`document_templates` — one row per blank PDF the provider uploaded to drag
fields onto. Owns the S3 pointer in `ck-templates`.

`document_template_fields` — one row per template, `fields_json` is an array
of `Field` values (see `pdfsign.go`). Updated every time the provider saves
the FieldDesigner UI.

`sign_sessions` — one row per signing invitation. Carries a frozen snapshot of
the fields (so that editing the template later does not retroactively change
a pending invitation). Status transitions: `pending → in_progress → completed`
or `pending → expired` / `pending → revoked`.

`signatures` — one row per completed signature. The canonical location of
`sha256_before`, `sha256_after`, `signed_pdf_s3_key`, `audit_s3_key`, `ip`,
`user_agent`, and `signed_at`. Joined to `sign_sessions` via
`session_token`.

Exact DDL is inline at the top of `store.go` and shipped as a migration.

## Tests

```sh
go test ./backend/internal/pdfsign/...
```

Tests exercise:

- base62 encoding sanity + length
- session create, get, expire
- finalize happy path + bad header + hash mismatch + reuse
- consent requirement
- `VerifyIntegrity` tamper detection

No real S3 or Postgres is touched; both are mocked via in-memory fakes.

## ESIGN / UETA compliance checklist

| Requirement | Where satisfied |
|-------------|-----------------|
| Consumer disclosure + consent | `consent.esignDisclosureVersion`, `consent.signerTypedName` on every audit record |
| Intent to sign | User must type full name and click "Finalize signature" in the React UI |
| Signature attribution | `signer_email` on the invitation; `ip_address`, `user_agent` recorded server-side |
| Record integrity | `sha256_before` + `sha256_after` stored in DB, audit JSON, and certificate page |
| Record retention | S3 buckets versioned; `signatures` row never deleted (soft-delete only) |
| Consumer right to paper copy | Not in this package — handled by the email flow that delivers the signed PDF PDF attachment |

**Disclaimer:** this package implements the technical controls. Legal
validity requires the product to also present the ESIGN disclosure before
first signature and retain records per state rules. See
`legal/esignature-disclosure.md`.
