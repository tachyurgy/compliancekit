-- 000017_reconcile_documents_and_staff
--
-- Third and hopefully final wave of handler-to-schema reconcile migrations.
-- Runtime SQL in handlers/{documents,staff,portal}.go references columns
-- that the canonical schema did not declare. Add them with backfill.
--
-- Documents:
--   subject_kind ← alias for owner_kind (handler wrote this shape)
--   subject_id   ← alias for owner_id
--   kind         ← alias for doc_type
--   title        ← new, nullable string display name
--
-- Staff:
--   background_check_date ← date of last cleared background check
--   (hire_date was added in 000016)
--
-- Once the runtime stabilises we should pick ONE canonical name per
-- concept and migrate callers; for now both spellings coexist.

ALTER TABLE documents ADD COLUMN subject_kind TEXT;
UPDATE documents SET subject_kind = owner_kind WHERE subject_kind IS NULL;

ALTER TABLE documents ADD COLUMN subject_id TEXT;
UPDATE documents SET subject_id = owner_id WHERE subject_id IS NULL;

ALTER TABLE documents ADD COLUMN kind TEXT;
UPDATE documents SET kind = doc_type WHERE kind IS NULL;

ALTER TABLE documents ADD COLUMN title TEXT;

ALTER TABLE staff ADD COLUMN background_check_date TEXT;
