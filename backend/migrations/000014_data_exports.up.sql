-- 000014_data_exports.up.sql
-- Provider data-export job log. Rows are append-only; each row represents a
-- single "export everything I have" request made from the Settings UI.
--
-- We store the S3 key of the finished ZIP (exports/{provider_id}/{ts}.zip) so
-- GET /api/exports/:id/download can mint a fresh presigned URL on demand
-- rather than leaking a long-lived link in the first success email.
--
-- No soft delete: successful exports remain visible as historical receipts.
-- Failed exports stay visible too (with error_text) so the user can retry.
--
-- SQLite dialect (see ADR-017).

CREATE TABLE data_exports (
    id                    TEXT PRIMARY KEY,
    provider_id           TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    requested_by_user_id  TEXT REFERENCES users(id) ON DELETE SET NULL,
    status                TEXT NOT NULL DEFAULT 'requested',
    s3_key                TEXT,
    error_text            TEXT,
    started_at            TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at           TEXT,
    CONSTRAINT data_exports_status_chk CHECK (
        status IN ('requested','running','completed','failed')
    )
);

CREATE INDEX data_exports_provider_started_idx
    ON data_exports (provider_id, started_at DESC);
