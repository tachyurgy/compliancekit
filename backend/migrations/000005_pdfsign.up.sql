-- 000005_pdfsign.up.sql
-- Tables backing the pdfsign Go package: blank-form templates, signing
-- sessions tied to magic links, and the final signature audit record.

BEGIN;

-- ---------------------------------------------------------------------------
-- document_templates: pre-built blank forms (e.g. FL CF-FSP 5219 parental
-- consent). fields_json describes the acroform / overlay regions:
--   [{"id":"parent_name","kind":"text","page":1,"x":100,"y":200,"w":300,"h":20},
--    {"id":"parent_signature","kind":"signature","page":2,"x":100,"y":50, ...}]
-- ---------------------------------------------------------------------------
CREATE TABLE document_templates (
    id                 TEXT PRIMARY KEY,
    provider_id        TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    name               TEXT NOT NULL,
    s3_key_blank_pdf   TEXT NOT NULL,
    fields_json        JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER document_templates_set_updated_at
BEFORE UPDATE ON document_templates
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX document_templates_provider_idx ON document_templates (provider_id);

-- ---------------------------------------------------------------------------
-- sign_sessions: one per "send this form to <signer>". Either starts from a
-- template (blank form to fill + sign) or from an existing document (we have
-- the PDF, just need signature boxes filled). Exactly one of
-- document_template_id / document_id will be non-null — enforced below.
--
-- Bound 1:1 to a magic_link_tokens row (which carries expiry + consumption).
-- ---------------------------------------------------------------------------
CREATE TABLE sign_sessions (
    id                    TEXT PRIMARY KEY,
    document_template_id  TEXT REFERENCES document_templates(id) ON DELETE SET NULL,
    document_id           TEXT REFERENCES documents(id) ON DELETE SET NULL,
    signer_role           TEXT NOT NULL,
    signer_name           TEXT NOT NULL,
    signer_email          CITEXT,
    signer_phone          TEXT,
    magic_link_token_id   TEXT NOT NULL REFERENCES magic_link_tokens(id) ON DELETE CASCADE,
    status                TEXT NOT NULL DEFAULT 'pending',
    expires_at            TIMESTAMPTZ NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT sign_sessions_signer_role_chk CHECK (signer_role IN ('provider_admin','staff','parent','other')),
    CONSTRAINT sign_sessions_status_chk      CHECK (status IN ('pending','in_progress','signed','declined','expired')),
    CONSTRAINT sign_sessions_source_chk      CHECK (
        (document_template_id IS NOT NULL AND document_id IS NULL) OR
        (document_template_id IS NULL     AND document_id IS NOT NULL)
    )
);

CREATE TRIGGER sign_sessions_set_updated_at
BEFORE UPDATE ON sign_sessions
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX sign_sessions_status_idx    ON sign_sessions (status);
CREATE INDEX sign_sessions_expiring_idx  ON sign_sessions (expires_at) WHERE status IN ('pending','in_progress');
CREATE INDEX sign_sessions_magic_idx     ON sign_sessions (magic_link_token_id);

-- ---------------------------------------------------------------------------
-- signatures: immutable post-signing record. sha256_before is the hash of
-- the PDF handed to the signer, sha256_after is the hash of the finalized
-- PDF. A separate audit PDF (timeline, IP, UA, consent version) is stored
-- in ck-audit-trail and referenced here. We do NOT allow updates/deletes
-- of rows in this table in application code.
--
-- consent_version_id is a hard FK to policy_versions (ESIGN disclosure
-- version active when the user clicked "I agree") — see 000007.
-- Created here with a forward-declared REFERENCES that is resolved in 000007
-- via ALTER TABLE ADD CONSTRAINT. For migration ordering we make this a
-- TEXT column now and add the FK constraint in 000007.
-- ---------------------------------------------------------------------------
CREATE TABLE signatures (
    id                    TEXT PRIMARY KEY,
    sign_session_id       TEXT NOT NULL REFERENCES sign_sessions(id) ON DELETE CASCADE,
    document_id           TEXT NOT NULL REFERENCES documents(id) ON DELETE RESTRICT,
    signer_user_id        TEXT REFERENCES users(id) ON DELETE SET NULL,
    signer_declared_name  TEXT NOT NULL,
    signed_at             TIMESTAMPTZ NOT NULL,
    sha256_before         BYTEA NOT NULL,
    sha256_after          BYTEA NOT NULL,
    s3_bucket_signed      TEXT NOT NULL,
    s3_key_signed         TEXT NOT NULL,
    s3_bucket_audit       TEXT NOT NULL,
    s3_key_audit          TEXT NOT NULL,
    signer_ip             INET,
    signer_user_agent     TEXT,
    consent_version_id    TEXT NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX signatures_session_idx   ON signatures (sign_session_id);
CREATE INDEX signatures_document_idx  ON signatures (document_id);
CREATE INDEX signatures_signed_at_idx ON signatures (signed_at);

COMMIT;
