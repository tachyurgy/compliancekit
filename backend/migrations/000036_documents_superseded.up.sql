-- REQ-007: supersede support for the chase engine.
--
-- When a fresh document of the same kind lands for the same subject
-- (staff_id or child_id, represented here as subject_kind+subject_id), the
-- old document is marked superseded and the chase loop must skip it. This
-- is the fastest way to lose operator trust: sending "your CPR is expiring"
-- right after the parent just uploaded a new CPR.
--
-- The chase service joins on `superseded_at IS NULL` so the supersede flip
-- is a single UPDATE in the same transaction as the new document insert.
ALTER TABLE documents ADD COLUMN superseded_at TEXT;
ALTER TABLE documents ADD COLUMN superseded_by_document_id TEXT;

-- Partial indexes to make the supersede-lookup cheap. The chase candidate
-- query also filters on superseded_at IS NULL; the planner uses these
-- when (subject_kind, subject_id, kind) is the natural lookup key, which it
-- is for both the supersede-flip path and the chase candidate scan.
CREATE INDEX IF NOT EXISTS idx_documents_active_by_subject_kind
  ON documents (subject_kind, subject_id, kind)
  WHERE superseded_at IS NULL AND deleted_at IS NULL;
