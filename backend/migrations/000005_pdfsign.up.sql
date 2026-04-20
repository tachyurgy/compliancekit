-- 000005_pdfsign.up.sql
-- Tables backing the pdfsign Go package: blank-form templates, signing
-- sessions tied to magic links, and the final signature audit record.
-- SQLite dialect (see ADR-017).

-- ---------------------------------------------------------------------------
-- document_templates: pre-built blank forms (e.g. FL CF-FSP 5219 parental
-- consent). fields_json describes the acroform / overlay regions.
-- Stored as TEXT and validated with json_valid() + json_type() = 'array'.
-- ---------------------------------------------------------------------------
CREATE TABLE document_templates (
    id                 TEXT PRIMARY KEY,
    provider_id        TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    name               TEXT NOT NULL,
    s3_key_blank_pdf   TEXT NOT NULL,
    fields_json        TEXT NOT NULL DEFAULT '[]',
    created_at         TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT document_templates_fields_json_chk CHECK (
        json_valid(fields_json) AND json_type(fields_json) = 'array'
    )
);

CREATE TRIGGER document_templates_set_updated_at
AFTER UPDATE ON document_templates
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE document_templates SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE INDEX document_templates_provider_idx ON document_templates (provider_id);

-- ---------------------------------------------------------------------------
-- sign_sessions: one per "send this form to <signer>". Either starts from a
-- template (blank form to fill + sign) or from an existing document. Exactly
-- one of document_template_id / document_id will be non-null.
--
-- Bound 1:1 to a magic_link_tokens row (which carries expiry + consumption).
-- ---------------------------------------------------------------------------
CREATE TABLE sign_sessions (
    id                    TEXT PRIMARY KEY,
    document_template_id  TEXT REFERENCES document_templates(id) ON DELETE SET NULL,
    document_id           TEXT REFERENCES documents(id) ON DELETE SET NULL,
    signer_role           TEXT NOT NULL,
    signer_name           TEXT NOT NULL,
    signer_email          TEXT COLLATE NOCASE,
    signer_phone          TEXT,
    magic_link_token_id   TEXT NOT NULL REFERENCES magic_link_tokens(id) ON DELETE CASCADE,
    status                TEXT NOT NULL DEFAULT 'pending',
    expires_at            TEXT NOT NULL,
    created_at            TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT sign_sessions_signer_role_chk CHECK (signer_role IN ('provider_admin','staff','parent','other')),
    CONSTRAINT sign_sessions_status_chk      CHECK (status IN ('pending','in_progress','signed','declined','expired')),
    CONSTRAINT sign_sessions_source_chk      CHECK (
        (document_template_id IS NOT NULL AND document_id IS NULL) OR
        (document_template_id IS NULL     AND document_id IS NOT NULL)
    )
);

CREATE TRIGGER sign_sessions_set_updated_at
AFTER UPDATE ON sign_sessions
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE sign_sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE INDEX sign_sessions_status_idx    ON sign_sessions (status);
CREATE INDEX sign_sessions_expiring_idx  ON sign_sessions (expires_at) WHERE status IN ('pending','in_progress');
CREATE INDEX sign_sessions_magic_idx     ON sign_sessions (magic_link_token_id);

-- ---------------------------------------------------------------------------
-- signatures: immutable post-signing record. sha256_before is the hash of
-- the PDF handed to the signer, sha256_after is the hash of the finalized
-- PDF. A separate audit JSON (timeline, IP, UA, consent version) is stored
-- in the ck-files bucket under the audit/ prefix and referenced here. We do
-- NOT allow updates/deletes of rows in this table in application code.
--
-- consent_version_id references policy_versions(id) semantically, but SQLite
-- cannot add a foreign key via ALTER TABLE ADD CONSTRAINT after the fact,
-- and policy_versions is created later in 000007. We keep the column as
-- TEXT NOT NULL and enforce integrity at INSERT time in the application
-- layer (pdfsign.InsertSignature). See ADR-017.
-- ---------------------------------------------------------------------------
CREATE TABLE signatures (
    id                    TEXT PRIMARY KEY,
    sign_session_id       TEXT NOT NULL REFERENCES sign_sessions(id) ON DELETE CASCADE,
    document_id           TEXT NOT NULL REFERENCES documents(id) ON DELETE RESTRICT,
    signer_user_id        TEXT REFERENCES users(id) ON DELETE SET NULL,
    signer_declared_name  TEXT NOT NULL,
    signed_at             TEXT NOT NULL,
    sha256_before         BLOB NOT NULL,
    sha256_after          BLOB NOT NULL,
    s3_key_signed         TEXT NOT NULL,
    s3_key_audit          TEXT NOT NULL,
    signer_ip             TEXT,
    signer_user_agent     TEXT,
    consent_version_id    TEXT NOT NULL,
    created_at            TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX signatures_session_idx   ON signatures (sign_session_id);
CREATE INDEX signatures_document_idx  ON signatures (document_id);
CREATE INDEX signatures_signed_at_idx ON signatures (signed_at);
