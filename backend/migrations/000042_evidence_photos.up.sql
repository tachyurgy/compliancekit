-- 000039_evidence_photos.up.sql
-- REQ-038: arbitrary photo evidence attached to drills, postings, and any
-- other subject (generic). Mirrors documents.storage_bucket/storage_key so
-- we can presign downloads via the same s3 client. Soft-delete via
-- deleted_at — never hard-delete (we want the audit trail intact even if
-- an operator yanks the photo).

CREATE TABLE IF NOT EXISTS evidence_photos (
    id                    TEXT PRIMARY KEY,
    provider_id           TEXT NOT NULL,
    subject_kind          TEXT NOT NULL,           -- 'drill' | 'posting' | 'generic'
    subject_id            TEXT NOT NULL,           -- drill_logs.id, posting key, or free-form
    storage_bucket        TEXT NOT NULL,
    storage_key           TEXT NOT NULL,
    mime_type             TEXT NOT NULL,
    caption               TEXT NOT NULL DEFAULT '',
    uploaded_by_user_id   TEXT,
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at            DATETIME
);

CREATE INDEX IF NOT EXISTS evidence_photos_subject_idx
    ON evidence_photos (provider_id, subject_kind, subject_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS evidence_photos_provider_idx
    ON evidence_photos (provider_id)
    WHERE deleted_at IS NULL;
