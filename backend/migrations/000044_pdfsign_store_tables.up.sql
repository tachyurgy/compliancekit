-- 000044_pdfsign_store_tables.up.sql
-- Why: migration 000005 sketched pdfsign tables (document_templates,
-- sign_sessions, signatures) around magic-link tokens, but the pdfsign Go
-- package (internal/pdfsign/store.go) was written against a different shape:
-- token-keyed sessions, a separate template-fields row, and per-provider
-- signature records. The two never met, so every pdfsign query failed with
-- "no such column". These pdfsign_* tables are the schema the code actually
-- uses. The 000005 tables are left in place (retention purge and data export
-- still reference them) and are empty.
--
-- Timestamps are declared DATETIME so the modernc driver round-trips Go
-- time.Time values without custom scanners. Every row hangs off providers
-- with ON DELETE CASCADE so a tenant purge removes its signing history too.

CREATE TABLE pdfsign_templates (
    id          TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    page_count  INTEGER NOT NULL DEFAULT 1,
    sha256      TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX pdfsign_templates_provider_idx ON pdfsign_templates (provider_id);

CREATE TABLE pdfsign_template_fields (
    template_id TEXT PRIMARY KEY REFERENCES pdfsign_templates(id) ON DELETE CASCADE,
    fields_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(fields_json)),
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE pdfsign_sessions (
    token                    TEXT PRIMARY KEY,
    provider_id              TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    document_id              TEXT NOT NULL REFERENCES pdfsign_templates(id) ON DELETE CASCADE,
    signer_id                TEXT,
    signer_name              TEXT NOT NULL,
    signer_email             TEXT NOT NULL,
    fields_json              TEXT NOT NULL CHECK (json_valid(fields_json)),
    esign_disclosure_version TEXT NOT NULL,
    status                   TEXT NOT NULL CHECK (status IN ('pending','in_progress','completed','expired','revoked')),
    created_at               DATETIME NOT NULL,
    expires_at               DATETIME NOT NULL
);
CREATE INDEX pdfsign_sessions_provider_idx ON pdfsign_sessions (provider_id);

CREATE TABLE pdfsign_signatures (
    signature_id      TEXT PRIMARY KEY,
    session_token     TEXT NOT NULL REFERENCES pdfsign_sessions(token) ON DELETE CASCADE,
    provider_id       TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    document_id       TEXT NOT NULL,
    signed_at         DATETIME NOT NULL,
    sha256_before     TEXT NOT NULL,
    sha256_after      TEXT NOT NULL,
    signed_pdf_s3_key TEXT NOT NULL,
    audit_s3_key      TEXT NOT NULL,
    ip_address        TEXT,
    user_agent        TEXT
);
CREATE INDEX pdfsign_signatures_provider_idx ON pdfsign_signatures (provider_id, signed_at DESC);
