-- 000020_reconcile_documents_chase_fields
--
-- handlers/children.go ListDocuments, handlers/staff.go ListDocuments,
-- handlers/portal.go and notify/chase.go all read columns from `documents`
-- that never made it into the canonical 000004 schema:
--
--   issued_at          — when the document was issued (license/cert-facing
--                         data, distinct from created_at/uploaded_at)
--   expires_at         — alias for canonical expiration_date
--   ocr_source         — provider that produced the OCR (mistral/gemini)
--   uploaded_by        — alias for uploaded_by_user_id
--   last_chase_sent_at — bookkeeping for the chase worker dedup logic
--
-- Add them with backfill from canonical columns where applicable.

ALTER TABLE documents ADD COLUMN issued_at TEXT;
ALTER TABLE documents ADD COLUMN expires_at TEXT;
UPDATE documents SET expires_at = expiration_date WHERE expires_at IS NULL;

ALTER TABLE documents ADD COLUMN ocr_source TEXT;
ALTER TABLE documents ADD COLUMN uploaded_by TEXT;
UPDATE documents SET uploaded_by = uploaded_by_user_id WHERE uploaded_by IS NULL;

ALTER TABLE documents ADD COLUMN last_chase_sent_at TEXT;
