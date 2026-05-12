DROP INDEX IF EXISTS idx_documents_active_by_subject_kind;
ALTER TABLE documents DROP COLUMN superseded_by_document_id;
ALTER TABLE documents DROP COLUMN superseded_at;
