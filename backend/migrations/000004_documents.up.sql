-- 000004_documents.up.sql
-- Core document store + OCR results + unassigned "snap-and-go" parent uploads.
-- SQLite dialect (see ADR-017).

-- ---------------------------------------------------------------------------
-- documents: the authoritative record of every uploaded file. The file bytes
-- themselves live in S3; this table is the index.
--
-- owner_kind / owner_id is intentionally polymorphic because documents may
-- attach to a child, a staff member, or the facility itself. No FK — we
-- validate the reference in application code when the row is created.
--
-- sha256 is indexed UNIQUE-per-provider to dedupe accidental re-uploads of
-- the same vaccine record. Across providers we allow duplicates because
-- different tenants' data lives in different S3 prefixes.
-- ---------------------------------------------------------------------------
CREATE TABLE documents (
    id                     TEXT PRIMARY KEY,
    provider_id            TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    owner_kind             TEXT NOT NULL,
    owner_id               TEXT NOT NULL,
    doc_type               TEXT NOT NULL,
    original_filename      TEXT,
    mime_type              TEXT,
    s3_key                 TEXT NOT NULL,
    sha256                 BLOB,
    byte_size              INTEGER,
    uploaded_by_user_id    TEXT REFERENCES users(id) ON DELETE SET NULL,
    uploaded_via           TEXT NOT NULL,
    ocr_status             TEXT NOT NULL DEFAULT 'pending',
    ocr_confidence         REAL NULL,
    expiration_date        TEXT NULL,
    expiration_source      TEXT NOT NULL DEFAULT 'none',
    expiration_confidence  REAL NULL,
    confirmed_by_user_id   TEXT REFERENCES users(id) ON DELETE SET NULL,
    confirmed_at           TEXT NULL,
    created_at             TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at             TEXT NULL,
    CONSTRAINT documents_owner_kind_chk   CHECK (owner_kind IN ('child','staff','facility')),
    CONSTRAINT documents_uploaded_via_chk CHECK (uploaded_via IN ('provider','parent_portal','staff_portal','bulk_import')),
    CONSTRAINT documents_ocr_status_chk   CHECK (ocr_status IN ('pending','processing','completed','failed','skipped')),
    CONSTRAINT documents_exp_source_chk   CHECK (expiration_source IN ('ocr','user_confirmed','user_entered','none'))
);

CREATE TRIGGER documents_set_updated_at
AFTER UPDATE ON documents
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE documents SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE INDEX documents_provider_expiration_idx ON documents (provider_id, expiration_date)
    WHERE deleted_at IS NULL;
CREATE INDEX documents_provider_doctype_idx    ON documents (provider_id, doc_type)
    WHERE deleted_at IS NULL;
CREATE INDEX documents_owner_idx               ON documents (owner_kind, owner_id)
    WHERE deleted_at IS NULL;
CREATE INDEX documents_ocr_pending_idx         ON documents (created_at)
    WHERE ocr_status IN ('pending','processing');
-- Per-provider dedupe of identical file contents.
CREATE UNIQUE INDEX documents_provider_sha256_uidx ON documents (provider_id, sha256)
    WHERE deleted_at IS NULL AND sha256 IS NOT NULL;

-- ---------------------------------------------------------------------------
-- document_ocr_results: raw model output. We keep BOTH mistral + gemini
-- responses when we dual-run for confidence scoring. The winner is reflected
-- back into documents.expiration_date / ocr_confidence.
-- parsed is TEXT holding JSON; validated with json_valid().
-- ---------------------------------------------------------------------------
CREATE TABLE document_ocr_results (
    id           TEXT PRIMARY KEY,
    document_id  TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    provider     TEXT NOT NULL,
    raw_text     TEXT,
    parsed       TEXT,
    confidence   REAL,
    latency_ms   INTEGER,
    created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT document_ocr_provider_chk CHECK (provider IN ('mistral','gemini')),
    CONSTRAINT document_ocr_parsed_json  CHECK (parsed IS NULL OR json_valid(parsed))
);

CREATE INDEX document_ocr_results_document_idx ON document_ocr_results (document_id);

-- ---------------------------------------------------------------------------
-- document_unassigned_photos: "parent took a photo of a shot record at the
-- pediatrician's office via the magic-link upload form, but we haven't
-- matched it to a child/doc_type yet." Provider triages these in the UI.
--
-- Once assigned, assigned_document_id points to the documents row and we
-- keep this row as an audit trail (soft-deletable).
-- ---------------------------------------------------------------------------
CREATE TABLE document_unassigned_photos (
    id                        TEXT PRIMARY KEY,
    provider_id               TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    uploaded_by_magic_link_id TEXT REFERENCES magic_link_tokens(id) ON DELETE SET NULL,
    s3_key                    TEXT NOT NULL,
    thumbnail_s3_key          TEXT,
    taken_at                  TEXT NULL,
    assigned_document_id      TEXT REFERENCES documents(id) ON DELETE SET NULL,
    created_at                TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at                TEXT NULL
);

CREATE INDEX docunassigned_provider_unassigned_idx ON document_unassigned_photos (provider_id, created_at)
    WHERE assigned_document_id IS NULL AND deleted_at IS NULL;
