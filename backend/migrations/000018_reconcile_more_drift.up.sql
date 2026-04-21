-- 000018_reconcile_more_drift
--
-- Final sweep of handler-vs-schema drift surfaced by integration tests.
-- Documents handler + portal handler reference columns (storage_bucket,
-- storage_key, size_bytes) that never made it into migration 000004 —
-- instead the canonical schema uses (s3_key, byte_size). Children handler
-- writes a `status` column that 000002 replaced with `withdrawal_date`.
--
-- Add alias columns with backfill to keep both spellings working.

ALTER TABLE documents ADD COLUMN storage_bucket TEXT;
ALTER TABLE documents ADD COLUMN storage_key TEXT;
UPDATE documents SET storage_key = s3_key WHERE storage_key IS NULL;

ALTER TABLE documents ADD COLUMN size_bytes INTEGER;
UPDATE documents SET size_bytes = byte_size WHERE size_bytes IS NULL;

ALTER TABLE children ADD COLUMN status TEXT NOT NULL DEFAULT 'enrolled';
-- Back-fill: rows with a withdrawal_date were effectively "withdrawn".
UPDATE children SET status = 'withdrawn' WHERE withdrawal_date IS NOT NULL;
