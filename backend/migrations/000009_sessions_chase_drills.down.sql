BEGIN;

ALTER TABLE providers
  DROP COLUMN IF EXISTS postings_checked_at,
  DROP COLUMN IF EXISTS ratio_checked_at,
  DROP COLUMN IF EXISTS postings_complete,
  DROP COLUMN IF EXISTS ratio_ok;

DROP TABLE IF EXISTS drill_logs;
DROP TABLE IF EXISTS document_chase_sends;
DROP TABLE IF EXISTS sessions;

COMMIT;
